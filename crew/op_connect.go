package crew

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
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

type ConnectOp struct {
	// FIXME: add flow queue

	terminal.OpBase

	t       terminal.OpTerminal
	conn    net.Conn
	entry   bool
	request *ConnectRequest
}

func (op *ConnectOp) Type() string {
	return ConnectOpType
}

type ConnectRequest struct {
	Domain   string
	IP       net.IP
	Protocol packet.IPProtocol
	Port     uint16
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
	// Create new op.
	op := &ConnectOp{
		t:       t,
		conn:    conn,
		entry:   true,
		request: request,
	}

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

	module.StartWorker("connect op conn reader", op.connReader)
	return op, nil
}

func runConnectOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
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
	op.SetID(opID)

	// Start worker.
	module.StartWorker("connect op conn reader", op.connReader)

	log.Infof("spn/crew: connected op %s#%d to %s", op.t.FmtID(), op.ID(), request)
	return op, nil
}

func (op *ConnectOp) connReader(_ context.Context) error {
	for {
		buf := make([]byte, 1500)
		n, err := op.conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				op.t.OpEnd(op, nil)
			} else {
				op.t.OpEnd(op, terminal.ErrConnectionError.With("failed to read from %s: %w", op.connectedType(), err))
			}
			return nil
		}

		tErr := op.t.OpSend(op, container.New(buf[:n]))
		if tErr != nil {
			op.t.OpEnd(op, tErr.Wrap("failed to send data read from %s", op.connectedType()))
			return nil
		}
	}
}

func (op *ConnectOp) Deliver(c *container.Container) *terminal.Error {
	data := c.CompileData()

	for {
		// Send all given data.
		n, err := op.conn.Write(data)
		switch {
		case err != nil:
			return terminal.ErrConnectionError.With("failed to send to %s: %w", op.connectedType(), err)
		case n == 0:
			return terminal.ErrConnectionError.With("sent 0 bytes to %s", op.connectedType())
		case n < len(data):
			// If not all data was sent, try again.
			log.Debugf("spn/crew: %s#%d only sent %d/%d bytes to %s", op.t.FmtID(), op.ID(), n, len(data), op.connectedType())
			data = data[n:]
		default:
			return nil
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
	if op.conn != nil {
		_ = op.conn.Close()
	}
}
