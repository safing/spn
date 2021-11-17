package docks

import (
	"net"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

const (
	sendThresholdMaxWait = 100 * time.Microsecond

	expansionClientTimeout = 2 * time.Minute
	expansionServerTimeout = 5 * time.Minute
)

type CraneTerminal struct {
	*terminal.TerminalBase
	*terminal.DuplexFlowQueue

	crane *Crane
}

func NewLocalCraneTerminal(
	crane *Crane,
	remoteHub *hub.Hub,
	initMsg *terminal.TerminalOpts,
	submitUpstream func(*container.Container),
) (*CraneTerminal, *container.Container, *terminal.Error) {
	// Default to terminal msg submit function.
	if submitUpstream == nil {
		submitUpstream = crane.submitTerminalMsg
	}

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

func (t *CraneTerminal) Deliver(c *container.Container) *terminal.Error {
	return t.DuplexFlowQueue.Deliver(c)
}

func (t *CraneTerminal) LocalAddr() net.Addr {
	return t.crane.LocalAddr()
}

func (t *CraneTerminal) RemoteAddr() net.Addr {
	return t.crane.RemoteAddr()
}

func (t *CraneTerminal) Transport() *hub.Transport {
	return t.crane.Transport()
}

func (t *CraneTerminal) IsAbandoned() bool {
	return t.Abandoned.IsSet()
}

func (t *CraneTerminal) Abandon(err *terminal.Error) {
	if t.Abandoned.SetToIf(false, true) {
		// Send stop msg and end all operations.
		t.Shutdown(err, err.IsExternal())

		// Abandon terminal.
		t.crane.AbandonTerminal(t.ID(), err)
	}
}
