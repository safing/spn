package docks

import (
	"net"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

const (
	expansionClientTimeout = 2 * time.Minute
	expansionServerTimeout = 5 * time.Minute
)

// CraneTerminal is a terminal started by a crane.
type CraneTerminal struct {
	*terminal.TerminalBase
	*terminal.DuplexFlowQueue

	crane *Crane
}

// NewLocalCraneTerminal returns a new local crane terminal.
func NewLocalCraneTerminal(
	crane *Crane,
	remoteHub *hub.Hub,
	initMsg *terminal.TerminalOpts,
	submitUpstream func(*container.Container),
) (*CraneTerminal, *container.Container, *terminal.Error) {
	// Create Terminal Base.
	t, initData, err := terminal.NewLocalBaseTerminal(
		crane.ctx,
		crane.getNextTerminalID(),
		crane.ID,
		remoteHub,
		initMsg,
	)
	if err != nil {
		return nil, nil, err
	}

	return initCraneTerminal(crane, t, initMsg), initData, nil
}

// NewRemoteCraneTerminal returns a new remote crane terminal.
func NewRemoteCraneTerminal(
	crane *Crane,
	id uint32,
	initData *container.Container,
) (*CraneTerminal, *terminal.TerminalOpts, *terminal.Error) {
	// Create Terminal Base.
	t, initMsg, err := terminal.NewRemoteBaseTerminal(
		crane.ctx,
		id,
		crane.ID,
		crane.identity,
		initData,
	)
	if err != nil {
		return nil, nil, err
	}

	return initCraneTerminal(crane, t, initMsg), initMsg, nil
}

func initCraneTerminal(
	crane *Crane,
	t *terminal.TerminalBase,
	initMsg *terminal.TerminalOpts,
) *CraneTerminal {
	// Create Flow Queue.
	dfq := terminal.NewDuplexFlowQueue(t, initMsg.QueueSize, t.SubmitAsDataMsg(crane.submitTerminalMsg))

	// Create Crane Terminal and assign it as the extended Terminal.
	ct := &CraneTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
		crane:           crane,
	}
	t.SetTerminalExtension(ct)

	// Start workers.
	module.StartWorker("crane terminal handler", ct.Handler)
	module.StartWorker("crane terminal sender", ct.Sender)
	module.StartWorker("crane terminal flow queue", ct.FlowHandler)

	return ct
}

// GrantPermission grants the given permissions.
// Additionally, it will mark the crane as authenticated, if not public.
func (t *CraneTerminal) GrantPermission(grant terminal.Permission) {
	// Forward granted permission to base terminal.
	t.TerminalBase.GrantPermission(grant)

	// Mark crane as authenticated if not public or already authenticated.
	if !t.crane.Public() && !t.crane.Authenticated() {
		t.crane.authenticated.Set()

		// Submit metrics.
		newAuthenticatedCranes.Inc()
	}
}

// Deliver delivers a message to the crane terminal.
func (t *CraneTerminal) Deliver(c *container.Container) *terminal.Error {
	return t.DuplexFlowQueue.Deliver(c)
}

// Flush flushes the terminal and its flow queue.
func (t *CraneTerminal) Flush() {
	t.TerminalBase.Flush()
	t.DuplexFlowQueue.Flush()
}

// LocalAddr returns the crane's local address.
func (t *CraneTerminal) LocalAddr() net.Addr {
	return t.crane.LocalAddr()
}

// RemoteAddr returns the crane's remote address.
func (t *CraneTerminal) RemoteAddr() net.Addr {
	return t.crane.RemoteAddr()
}

// Transport returns the crane's transport.
func (t *CraneTerminal) Transport() *hub.Transport {
	return t.crane.Transport()
}

// IsAbandoned returns whether the crane has been abandoned.
func (t *CraneTerminal) IsAbandoned() bool {
	return t.Abandoned.IsSet()
}

// Abandon abandons the crane terminal.
func (t *CraneTerminal) Abandon(err *terminal.Error) {
	if t.Abandoned.SetToIf(false, true) {
		// Send stop msg and end all operations.
		t.Shutdown(err, err.IsExternal())

		// Abandon terminal.
		t.crane.AbandonTerminal(t.ID(), err)
	}
}
