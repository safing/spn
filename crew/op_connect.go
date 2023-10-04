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

	// Flow Control
	dfq *terminal.DuplexFlowQueue

	// Context and shutdown handling
	// ctx is the context of the Terminal.
	ctx context.Context
	// cancelCtx cancels ctx.
	cancelCtx context.CancelFunc
	// doneWriting signals that the writer has finished writing.
	doneWriting chan struct{}

	// Metrics
	incomingTraffic *uint64
	outgoingTraffic *uint64
	started         time.Time

	// Connection
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
func (r *ConnectRequest) DialNetwork() string {
	if ip4 := r.IP.To4(); ip4 != nil {
		switch r.Protocol {
		case packet.TCP:
			return "tcp4"
		case packet.UDP:
			return "udp4"
		}
	} else {
		switch r.Protocol {
		case packet.TCP:
			return "tcp6"
		case packet.UDP:
			return "udp6"
		}
	}

	return ""
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
	// Submit metrics.
	newConnectOp.Inc()

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
		doneWriting: make(chan struct{}),
		t:           tunnel.dstTerminal,
		conn:        tunnel.conn,
		request:     request,
		entry:       true,
		tunnel:      tunnel,
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
	op.started = time.Now()

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
		return nil, terminal.ErrPermissionDenied.With("connecting is only allowed on public hubs")
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

	// Check if IP seems valid.
	if len(request.IP) != net.IPv4len && len(request.IP) != net.IPv6len {
		return nil, terminal.ErrInvalidOptions.With("ip address is not valid")
	}

	// Check if connection target is in global scope.
	ipScope := netutils.GetIPScope(request.IP)
	if ipScope != netutils.Global {
		return nil, terminal.ErrPermissionDenied.With("denied request to connect to non-global IP %s", request.IP)
	}

	// Check exit policy.
	if tErr := checkExitPolicy(request); tErr != nil {
		return nil, tErr
	}

	// Connect to destination.
	dialNet := request.DialNetwork()
	if dialNet == "" {
		return nil, terminal.ErrIncorrectUsage.With("protocol %s is not supported", request.Protocol)
	}
	dialer := &net.Dialer{
		Timeout:       5 * time.Second,
		LocalAddr:     conf.GetConnectAddr(dialNet),
		FallbackDelay: -1, // Disables Fast Fallback from IPv6 to IPv4.
		KeepAlive:     -1, // Disable keep-alive.
	}
	conn, err := dialer.Dial(dialNet, request.Address())
	if err != nil {
		return nil, terminal.ErrConnectionError.With("failed to connect to %s: %w", request, err)
	}

	// Create and initialize operation.
	op := &ConnectOp{
		doneWriting: make(chan struct{}),
		t:           t,
		conn:        conn,
		request:     request,
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
	if err != nil {
		msg.Finish()
		op.Stop(op, err.Wrap("failed to send data (op) read from %s", op.connectedType()))
	}
}

const (
	readBufSize = 1500

	// High priority up to first 10MB.
	highPrioThreshold = 10_000_000

	// Rate limit to 128 Mbit/s after 1GB traffic.
	// Do NOT use time.Sleep per packet, as it is very inaccurate and will sleep a lot longer than desired.
	rateLimitThreshold = 1_000_000_000
	rateLimitMaxMbit   = 128
)

func (op *ConnectOp) connReader(_ context.Context) error {
	// Metrics setup and submitting.
	atomic.AddInt64(activeConnectOps, 1)
	defer func() {
		atomic.AddInt64(activeConnectOps, -1)
		connectOpDurationHistogram.UpdateDuration(op.started)
		connectOpIncomingDataHistogram.Update(float64(atomic.LoadUint64(op.incomingTraffic)))
	}()

	rateLimiter := terminal.NewRateLimiter(rateLimitMaxMbit)

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

		// Rate limit if over threshold.
		if inBytes > rateLimitThreshold {
			rateLimiter.Limit(uint64(n))
		}

		// Create message from data.
		msg := op.NewMsg(buf[:n])

		// Define priority and possibly wait for slot.
		switch {
		case inBytes > highPrioThreshold:
			msg.Unit.WaitForSlot()
		case op.request.UsePriorityDataMsgs:
			msg.Unit.MakeHighPriority()
		}

		// Send packet.
		tErr := op.dfq.Send(
			msg,
			30*time.Second,
		)
		if tErr != nil {
			msg.Finish()
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
	// Metrics submitting.
	defer func() {
		connectOpOutgoingDataHistogram.Update(float64(atomic.LoadUint64(op.outgoingTraffic)))
	}()

	defer func() {
		// Close connection.
		_ = op.conn.Close()
	}()

	var msg *terminal.Msg
	defer msg.Finish()

	rateLimiter := terminal.NewRateLimiter(rateLimitMaxMbit)

writing:
	for {
		msg.Finish()

		select {
		case msg = <-op.dfq.Receive():
		case <-op.ctx.Done():
			op.Stop(op, terminal.ErrCanceled)
			return nil
		default:
			// Handle all data before also listening for the context cancel.
			// This ensures all data is written properly before stopping.
			select {
			case msg = <-op.dfq.Receive():
			case op.doneWriting <- struct{}{}:
				op.Stop(op, terminal.ErrStopping)
				return nil
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

		// Rate limit if over threshold.
		if out > rateLimitThreshold {
			rateLimiter.Limit(uint64(len(data)))
		}

		// Special handling after first data was received on client.
		if op.entry &&
			out == uint64(len(data)) {
			// Report time taken to receive first byte.
			connectOpTTFBDurationHistogram.UpdateDuration(op.started)

			// If not stickied yet, stick destination to Hub.
			if !op.tunnel.stickied {
				op.tunnel.stickDestinationToHub()
			}
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

	// If the op was ended locally, send all data before closing.
	// If the op was ended remotely, don't bother sending remaining data.
	if !err.IsExternal() {
		// Flushing could mean sending a full buffer of 50000 packets.
		op.dfq.Flush(5 * time.Minute)
	}

	// If the op was ended remotely, write all remaining received data.
	// If the op was ended locally, don't bother writing remaining data.
	if err.IsExternal() {
		<-op.doneWriting
	}

	// Cancel workers.
	op.cancelCtx()

	// Avoid connecting to destination via this Hub if the was a connection
	// error and no data was received.
	if op.entry && // On clients only.
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
