package crew

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/network/netutils"
	"github.com/safing/portmaster/network/packet"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/terminal"
)

const ConnectOpType string = "connect"

var (
	activeConnectOps = new(int64)
)

type ConnectOp struct {
	terminal.OpBase
	*terminal.DuplexFlowQueue

	// ctx is the context of the Terminal.
	ctx context.Context
	// cancelCtx cancels ctx.
	cancelCtx context.CancelFunc

	incomingTraffic *uint64
	outgoingTraffic *uint64

	t       terminal.OpTerminal
	conn    net.Conn
	entry   bool
	request *ConnectRequest
}

func (op *ConnectOp) Type() string {
	return ConnectOpType
}

func (op *ConnectOp) Ctx() context.Context {
	return op.ctx
}

type ConnectRequest struct {
	Domain    string
	IP        net.IP
	Protocol  packet.IPProtocol
	Port      uint16
	QueueSize uint32
}

func (r *ConnectRequest) Address() string {
	return net.JoinHostPort(r.IP.String(), strconv.Itoa(int(r.Port)))
}

func (r *ConnectRequest) String() string {
	if r.Domain != "" {
		return fmt.Sprintf("%s (%s %s)", r.Domain, r.Protocol, r.Address())
	}
	return fmt.Sprintf("%s %s", r.Protocol, r.Address())
}

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:     ConnectOpType,
		Requires: terminal.MayConnect,
		RunOp:    runConnectOp,
	})
}

func NewConnectOp(t terminal.OpTerminal, request *ConnectRequest, conn net.Conn) (*ConnectOp, *terminal.Error) {
	// Set defaults.
	if request.QueueSize == 0 {
		request.QueueSize = terminal.DefaultQueueSize
	}

	// Create new op.
	op := &ConnectOp{
		t:       t,
		conn:    conn,
		entry:   true,
		request: request,
	}
	op.OpBase.Init()
	op.ctx, op.cancelCtx = context.WithCancel(context.Background())
	op.DuplexFlowQueue = terminal.NewDuplexFlowQueue(op, request.QueueSize, op.submitUpstream)

	// Prepare init msg.
	data, err := dsd.Dump(request, dsd.JSON)
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to pack connect request: %w", err)
	}

	// Initialize.
	tErr := t.OpInit(op, container.New(data))
	if err != nil {
		return nil, tErr
	}

	// Setup metrics.
	op.incomingTraffic = new(uint64)
	op.outgoingTraffic = new(uint64)

	module.StartWorker("connect op conn reader", op.connReader)
	module.StartWorker("connect op conn writer", op.connWriter)
	module.StartWorker("connect op flow handler", op.DuplexFlowQueue.FlowHandler)
	return op, nil
}

func runConnectOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Submit metrics.
	newConnectOp.Inc()

	// Check if we are running a public hub.
	if !conf.PublicHub() {
		return nil, terminal.ErrPermissinDenied.With("connecting is only allowed on public hubs")
	}

	// Parse connect request.
	request := &ConnectRequest{}
	_, err := dsd.Load(data.CompileData(), request)
	if err != nil {
		return nil, terminal.ErrMalformedData.With("failed to parse connect request: %w", err)
	}
	if request.QueueSize == 0 || request.QueueSize > terminal.MaxQueueSize {
		return nil, terminal.ErrInvalidOptions.With("invalid queue size of %d", request.QueueSize)
	}

	// Check if connection target is in global scope.
	ipScope := netutils.GetIPScope(request.IP)
	if ipScope != netutils.Global {
		return nil, terminal.ErrPermissinDenied.With("denied request to connect to non-global IP %s", request.IP)
	}

	// Get protocol net for connecting.
	var dialNet string
	switch request.Protocol {
	case packet.TCP:
		dialNet = "tcp"
	case packet.UDP:
		dialNet = "udp"
	default:
		return nil, terminal.ErrIncorrectUsage.With("protocol %s is not supported", request.Protocol)
	}

	// Check exit policy.
	if tErr := checkExitPolicy(request); tErr != nil {
		return nil, tErr
	}

	// Connect to destination.
	conn, err := net.DialTimeout(dialNet, request.Address(), 3*time.Second)
	if err != nil {
		return nil, terminal.ErrConnectionError.With("failed to connect to %s: %w", request, err)
	}

	// Create and initialize operation.
	op := &ConnectOp{
		t:       t,
		conn:    conn,
		request: request,
	}
	op.OpBase.Init()
	op.OpBase.SetID(opID)
	op.ctx, op.cancelCtx = context.WithCancel(context.Background())
	op.DuplexFlowQueue = terminal.NewDuplexFlowQueue(op, request.QueueSize, op.submitUpstream)

	// Setup metrics.
	op.incomingTraffic = new(uint64)
	op.outgoingTraffic = new(uint64)

	// Start worker.
	module.StartWorker("connect op conn reader", op.connReader)
	module.StartWorker("connect op conn writer", op.connWriter)
	module.StartWorker("connect op flow handler", op.DuplexFlowQueue.FlowHandler)

	log.Infof("spn/crew: connected op %s#%d to %s", op.t.FmtID(), op.ID(), request)
	return op, nil
}

func (op *ConnectOp) submitUpstream(c *container.Container) {
	tErr := op.t.OpSend(op, c)
	if tErr != nil {
		op.t.OpEnd(op, tErr.Wrap("failed to send data (op) read from %s", op.connectedType()))
	}
}

func (op *ConnectOp) connReader(_ context.Context) error {
	// Metrics setup and submitting.
	if !op.entry {
		atomic.AddInt64(activeConnectOps, 1)
		started := time.Now()
		defer func() {
			atomic.AddInt64(activeConnectOps, -1)
			connectOpDurationHistogram.UpdateDuration(started)
			connectOpIncomingDataHistogram.Update(float64(atomic.LoadUint64(op.incomingTraffic)))
			connectOpOutgoingDataHistogram.Update(float64(atomic.LoadUint64(op.outgoingTraffic)))
		}()
	}

	for {
		buf := make([]byte, 1500)
		n, err := op.conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				op.t.OpEnd(op, terminal.ErrStopping.With("connection to %s was closed on read", op.connectedType()))
			} else {
				op.t.OpEnd(op, terminal.ErrConnectionError.With("failed to read from %s: %w", op.connectedType(), err))
			}
			return nil
		}
		if n == 0 {
			log.Tracef("spn/crew: connect op %s>%d read 0 bytes from %s", op.t.FmtID(), op.ID(), op.connectedType())
			continue
		}

		// Submit metrics.
		connectOpIncomingBytes.Add(n)
		atomic.AddUint64(op.incomingTraffic, uint64(n))

		tErr := op.DuplexFlowQueue.Send(container.New(buf[:n]))
		if tErr != nil {
			op.t.OpEnd(op, tErr.Wrap("failed to send data (dfq) read from %s", op.connectedType()))
			return nil
		}
	}
}

func (op *ConnectOp) Deliver(c *container.Container) *terminal.Error {
	return op.DuplexFlowQueue.Deliver(c)
}

func (op *ConnectOp) connWriter(_ context.Context) error {
	defer op.conn.Close()

writing:
	for {
		var c *container.Container
		select {
		case c = <-op.DuplexFlowQueue.Receive():
		default:
			// Handle all data before also listening for the context cancel.
			// This ensures all data is written properly before stopping.
			select {
			case c = <-op.DuplexFlowQueue.Receive():
			case <-op.ctx.Done():
				return nil
			}
		}

		data := c.CompileData()
		if len(data) == 0 {
			continue writing
		}

		// Submit metrics.
		connectOpOutgoingBytes.Add(len(data))
		atomic.AddUint64(op.outgoingTraffic, uint64(len(data)))

		// Send all given data.
		for {
			n, err := op.conn.Write(data)
			switch {
			case err != nil:
				if errors.Is(err, io.EOF) {
					op.t.OpEnd(op, terminal.ErrStopping.With("connection to %s was closed on write", op.connectedType()))
				} else {
					op.t.OpEnd(op, terminal.ErrConnectionError.With("failed to send to %s: %w", op.connectedType(), err))
				}
				return nil
			case n == 0:
				op.t.OpEnd(op, terminal.ErrConnectionError.With("sent 0 bytes to %s", op.connectedType()))
				return nil
			case n < len(data):
				// If not all data was sent, try again.
				log.Debugf("spn/crew: %s#%d only sent %d/%d bytes to %s", op.t.FmtID(), op.ID(), n, len(data), op.connectedType())
				data = data[n:]
			default:
				continue writing
			}
		}
	}
}

func (op *ConnectOp) connectedType() string {
	if op.entry {
		return "origin"
	}
	return "destination"
}

func (op *ConnectOp) End(err *terminal.Error) {
	// Send all data before closing.
	op.DuplexFlowQueue.Flush()

	// Cancel workers.
	op.cancelCtx()
}

func (op *ConnectOp) Abandon(err *terminal.Error) {
	// Proxy for DuplexFlowQueue
	op.t.OpEnd(op, err)
}

func (op *ConnectOp) FmtID() string {
	return fmt.Sprintf("%s>%d", op.t.FmtID(), op.ID())
}
