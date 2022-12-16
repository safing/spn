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

// ConnectOpType is the type ID for the connection operation.
const ConnectOpType string = "connect"

var activeConnectOps = new(int64)

// ConnectOp is used to connect data tunnels to servers on the Internet.
type ConnectOp struct {
	terminal.OperationBase

	dfq *terminal.DuplexFlowQueue

	// ctx is the context of the Terminal.
	ctx context.Context
	// cancelCtx cancels ctx.
	cancelCtx context.CancelFunc

	incomingTraffic *uint64
	outgoingTraffic *uint64

	t       terminal.Terminal
	conn    net.Conn
	request *ConnectRequest
	entry   bool
	tunnel  *Tunnel
}

// Type returns the type ID.
func (op *ConnectOp) Type() string {
	return ConnectOpType
}

// Ctx returns the operation context.
func (op *ConnectOp) Ctx() context.Context {
	return op.ctx
}

// ConnectRequest holds all the information necessary for a connect operation.
type ConnectRequest struct {
	Domain              string            `json:"d,omitempty"`
	IP                  net.IP            `json:"ip,omitempty"`
	UsePriorityDataMsgs bool              `json:"pr,omitempty"`
	Protocol            packet.IPProtocol `json:"p,omitempty"`
	Port                uint16            `json:"po,omitempty"`
	QueueSize           uint32            `json:"qs,omitempty"`
}

// Address returns the address of the connext request.
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
	terminal.RegisterOpType(terminal.OperationFactory{
		Type:     ConnectOpType,
		Requires: terminal.MayConnect,
		Start:    startConnectOp,
	})
}

// NewConnectOp starts a new connect operation.
func NewConnectOp(tunnel *Tunnel) (*ConnectOp, *terminal.Error) {
	// Create request.
	request := &ConnectRequest{
		Domain:              tunnel.connInfo.Entity.Domain,
		IP:                  tunnel.connInfo.Entity.IP,
		Protocol:            packet.IPProtocol(tunnel.connInfo.Entity.Protocol),
		Port:                tunnel.connInfo.Entity.Port,
		UsePriorityDataMsgs: terminal.UsePriorityDataMsgs,
	}

	// Set defaults.
	if request.QueueSize == 0 {
		request.QueueSize = terminal.DefaultQueueSize
	}

	// Create new op.
	op := &ConnectOp{
		t:       tunnel.dstTerminal,
		conn:    tunnel.conn,
		request: request,
		entry:   true,
		tunnel:  tunnel,
	}
	op.ctx, op.cancelCtx = context.WithCancel(module.Ctx)
	op.dfq = terminal.NewDuplexFlowQueue(op.Ctx(), request.QueueSize, op.submitUpstream)

	// Prepare init msg.
	data, err := dsd.Dump(request, dsd.CBOR)
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to pack connect request: %w", err)
	}

	// Initialize.
	tErr := op.t.StartOperation(op, container.New(data), 5*time.Second)
	if err != nil {
		return nil, tErr
	}

	// Setup metrics.
	op.incomingTraffic = new(uint64)
	op.outgoingTraffic = new(uint64)

	module.StartWorker("connect op conn reader", op.connReader)
	module.StartWorker("connect op conn writer", op.connWriter)
	module.StartWorker("connect op flow handler", op.dfq.FlowHandler)

	log.Infof("spn/crew: connected to %s via %s", request, tunnel.dstPin.Hub)
	return op, nil
}

func startConnectOp(t terminal.Terminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
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
	switch request.Protocol { //nolint:exhaustive // Only looking at specific values.
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
	op.InitOperationBase(t, opID)
	op.ctx, op.cancelCtx = context.WithCancel(t.Ctx())
	op.dfq = terminal.NewDuplexFlowQueue(op.Ctx(), request.QueueSize, op.submitUpstream)

	// Setup metrics.
	op.incomingTraffic = new(uint64)
	op.outgoingTraffic = new(uint64)

	// Start worker.
	module.StartWorker("connect op conn reader", op.connReader)
	module.StartWorker("connect op conn writer", op.connWriter)
	module.StartWorker("connect op flow handler", op.dfq.FlowHandler)

	log.Infof("spn/crew: connected op %s#%d to %s", op.t.FmtID(), op.ID(), request)
	return op, nil
}

func (op *ConnectOp) submitUpstream(msg *terminal.Msg, timeout time.Duration) {
	err := op.Send(msg, timeout)
	if err.IsError() {
		msg.FinishUnit()
		op.Stop(op, err.Wrap("failed to send data (op) read from %s", op.connectedType()))
	}
}

const (
	readBufSize = 1500

	// High priority up to first 10MB.
	highPrioThreshold = 10_000_000

	// Rate limit to 100 Mbit/s (with 1500B packets) after 1GB traffic.
	rateLimitThreshold   = 1_000_000_000
	rateLimitMaxMbit     = 100
	rateLimitPacketDelay = time.Second / ((rateLimitMaxMbit / 8) * 1_000_000 / readBufSize)
)

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
		// Read from connection.
		buf := make([]byte, readBufSize)
		n, err := op.conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				op.Stop(op, terminal.ErrStopping.With("connection to %s was closed on read", op.connectedType()))
			} else {
				op.Stop(op, terminal.ErrConnectionError.With("failed to read from %s: %w", op.connectedType(), err))
			}
			return nil
		}
		if n == 0 {
			log.Tracef("spn/crew: connect op %s>%d read 0 bytes from %s", op.t.FmtID(), op.ID(), op.connectedType())
			continue
		}

		// Submit metrics.
		connectOpIncomingBytes.Add(n)
		inBytes := atomic.AddUint64(op.incomingTraffic, uint64(n))

		// Create message from data.
		msg := op.NewMsg(buf[:n])

		// Define priority and possibly wait for slot.
		switch {
		case inBytes > rateLimitThreshold:
			time.Sleep(rateLimitPacketDelay)
			fallthrough
		case inBytes > highPrioThreshold:
			msg.WaitForUnitSlot()
		case op.request.UsePriorityDataMsgs:
			msg.MakeUnitHighPriority()
		}

		// Send packet.
		tErr := op.dfq.Send(
			msg,
			30*time.Second,
		)
		if tErr.IsError() {
			msg.FinishUnit()
			op.Stop(op, tErr.Wrap("failed to send data (dfq) from %s", op.connectedType()))
			return nil
		}
	}
}

// Deliver delivers a messages to the operation.
func (op *ConnectOp) Deliver(msg *terminal.Msg) *terminal.Error {
	return op.dfq.Deliver(msg)
}

func (op *ConnectOp) connWriter(_ context.Context) error {
	defer func() {
		// Close connection.
		_ = op.conn.Close()
	}()

	var msg *terminal.Msg
	defer msg.FinishUnit()

writing:
	for {
		msg.FinishUnit()

		select {
		case msg = <-op.dfq.Receive():
		default:
			// Handle all data before also listening for the context cancel.
			// This ensures all data is written properly before stopping.
			select {
			case msg = <-op.dfq.Receive():
			case <-op.ctx.Done():
				op.Stop(op, terminal.ErrCanceled)
				return nil
			}
		}

		// TODO: Instead of compiling data here again, can we send it as in the container?
		data := msg.Data.CompileData()
		if len(data) == 0 {
			continue writing
		}

		// Submit metrics.
		connectOpOutgoingBytes.Add(len(data))
		out := atomic.AddUint64(op.outgoingTraffic, uint64(len(data)))

		// If on client and the first data was received, sticky the destination to the Hub.
		if op.entry && // On clients only.
			out == uint64(len(data)) && // Only on first packet received.
			!op.tunnel.stickied {
			op.tunnel.stickDestinationToHub()
		}

		// Send all given data.
		for {
			n, err := op.conn.Write(data)
			switch {
			case err != nil:
				if errors.Is(err, io.EOF) {
					op.Stop(op, terminal.ErrStopping.With("connection to %s was closed on write", op.connectedType()))
				} else {
					op.Stop(op, terminal.ErrConnectionError.With("failed to send to %s: %w", op.connectedType(), err))
				}
				return nil
			case n == 0:
				op.Stop(op, terminal.ErrConnectionError.With("sent 0 bytes to %s", op.connectedType()))
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

// HandleStop gives the operation the ability to cleanly shut down.
// The returned error is the error to send to the other side.
// Should never be called directly. Call Stop() instead.
func (op *ConnectOp) HandleStop(err *terminal.Error) (errorToSend *terminal.Error) {
	if err.IsError() {
		reportConnectError(err)
	}

	// Send all data before closing.
	op.dfq.Flush()

	// Cancel workers.
	op.cancelCtx()

	// Avoid connecting to destination via this Hub if the was a connection
	// error and no data was received.
	if op.entry && // On clients only.
		err.IsError() &&
		err.Is(terminal.ErrConnectionError) &&
		atomic.LoadUint64(op.outgoingTraffic) == 0 {
		// Only if no data was received (ie. sent to local application).
		op.tunnel.avoidDestinationHub()
	}

	// If we are on the client, don't leak local errors to the server.
	if op.entry && !err.IsExternal() {
		return terminal.ErrStopping
	}
	return err
}
