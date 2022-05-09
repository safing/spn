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

	crane *Crane
}

// NewLocalCraneTerminal returns a new local crane terminal.
func NewLocalCraneTerminal(
	crane *Crane,
	remoteHub *hub.Hub,
	initMsg *terminal.TerminalOpts,
	submitUpstream func(*container.Container) *terminal.Error,
) (*CraneTerminal, *container.Container, *terminal.Error) {
	// Create Terminal Base.
	t, initData, err := terminal.NewLocalBaseTerminal(
		crane.ctx,
		crane.getNextTerminalID(),
		crane.ID,
		remoteHub,
		initMsg,
		crane.submitTerminalMsg,
		true,
	)
	if err != nil {
		return nil, nil, err
	}

	return initCraneTerminal(crane, t), initData, nil
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
		crane.submitTerminalMsg,
		true,
	)
	if err != nil {
		return nil, nil, err
	}

	return initCraneTerminal(crane, t), initMsg, nil
}

func initCraneTerminal(
	crane *Crane,
	t *terminal.TerminalBase,
) *CraneTerminal {
	// Create Crane Terminal and assign it as the extended Terminal.
	ct := &CraneTerminal{
		TerminalBase: t,
		crane:        crane,
	}
	t.SetTerminalExtension(ct)

	// Start workers.
	t.StartWorkers(module, "crane terminal")

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

// IsBeingAbandoned returns whether the terminal is being abandoned.
func (t *CraneTerminal) IsBeingAbandoned() bool {
	return t.Abandoning.IsSet()
}

// Abandon abandons the crane terminal.
func (t *CraneTerminal) Abandon(err *terminal.Error) {
	if t.Abandoning.SetToIf(false, true) {
		// Send stop msg and end all operations.
		t.StartAbandonProcedure(err, err.IsExternal(), func() {
			// Abandon terminal.
			t.crane.AbandonTerminal(t.ID(), err)
		})
	}
}
