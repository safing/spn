package docks

import (
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

const (
	sendThresholdMaxWait = 100 * time.Microsecond
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
	dfq := terminal.NewDuplexFlowQueue(t, initMsg.QueueSize, crane.submitTerminalMsg)

	// Create Crane Terminal and assign it as the extended Terminal.
	ct := &CraneTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
		crane:           crane,
	}
	t.SetTerminalExtension(ct)

	// Start workers.
	module.StartWorker("crane terminal", ct.Handler)
	module.StartWorker("crane terminal flow queue", ct.FlowHandler)

	return ct
}

func (t *CraneTerminal) Deliver(c *container.Container) *terminal.Error {
	return t.DuplexFlowQueue.Deliver(c)
}

func (t *CraneTerminal) Abandon(err *terminal.Error) {
	if t.Abandoned.SetToIf(false, true) {
		// End all operations and stop all connected workers.
		t.StopAll(nil)

		// Abandon terminal.
		t.crane.AbandonTerminal(t.ID(), err)
	}
}
