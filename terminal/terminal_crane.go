package terminal

import (
	"context"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/hub"
)

type CraneTerminal struct {
	*TerminalBase
	*DuplexFlowQueue
}

func NewLocalCraneTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	remoteHub *hub.Hub,
	initMsg *TerminalOpts,
	submitUpstream func(*container.Container),
) (*CraneTerminal, *container.Container, Error) {
	// Create Terminal Base.
	t, initData, err := NewLocalBaseTerminal(ctx, id, parentID, remoteHub, initMsg)
	if err != ErrNil {
		return nil, nil, err
	}

	return initCraneTerminal(t, initMsg, submitUpstream), initData, ErrNil
}

func NewRemoteCraneTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	identity *cabin.Identity,
	initData *container.Container,
	submitUpstream func(*container.Container),
) (*CraneTerminal, *TerminalOpts, Error) {
	// Create Terminal Base.
	t, initMsg, err := NewRemoteBaseTerminal(ctx, id, parentID, identity, initData)
	if err != ErrNil {
		return nil, nil, err
	}

	return initCraneTerminal(t, initMsg, submitUpstream), initMsg, ErrNil
}

func initCraneTerminal(
	t *TerminalBase,
	initMsg *TerminalOpts,
	submitUpstream func(*container.Container),
) *CraneTerminal {
	// Create Flow Queue.
	dfq := NewDuplexFlowQueue(t, initMsg.QueueSize, submitUpstream)

	// Create Crane Terminal and assign it as the extended Terminal.
	ct := &CraneTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
	}
	t.SetTerminalExtension(ct)

	// Start workers.
	module.StartWorker("crane terminal", ct.Handler)
	module.StartWorker("crane terminal flow queue", ct.FlowHandler)

	return ct
}

func (t *CraneTerminal) Abandon(action string, err Error) {
	if t.Abandoned.SetToIf(false, true) {
		switch err {
		case ErrNil:
			// ErrNil means that the Terminal is being shutdown by the owner.
			log.Tracef("spn/terminal: %s is closing", fmtTerminalID(t.parentID, t.id))
		default:
			// All other errors are faults.
			log.Warningf("spn/terminal: %s %s: %s", fmtTerminalID(t.parentID, t.id), action, err)
			// Report back.
			t.SendAbandonMsg(err)
		}

		// End all operations and stop all connected workers.
		t.StopAll("", ErrNil)
	}
}
