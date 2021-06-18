package terminal

import (
	"context"
	"sync/atomic"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
)

type CraneTerminal struct {
	*TerminalBase
	*DuplexFlowQueue
}

func NewCraneTerminal(
	ctx context.Context,
	craneID string,
	id uint32,
	initialData *container.Container,
	submitUpstream func(*container.Container),
) *CraneTerminal {
	// Create Terminal Base.
	t := NewTerminalBase(ctx, id, craneID, initialData)
	atomic.StoreUint32(t.nextOpID, 1)

	// Create Flow Queue.
	dfq := NewDuplexFlowQueue(t, defaultQueueSize, submitUpstream)

	// Create Crane Terminal and assign it as the extended Terminal.
	ct := &CraneTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
	}
	t.ext = ct

	module.StartWorker("crane terminal", t.handler)

	return ct
}

func (t *CraneTerminal) Abandon(action string, err Error) {
	if t.abandoned.SetToIf(false, true) {
		switch err {
		case ErrNil:
			// ErrNil means that the Terminal is being shutdown by the owner.
			log.Tracef("terminal: %s#%d is closing", t.parentID, t.id)
		default:
			// All other errors are faults.
			log.Warningf("terminal: %s#%d %s: %s", t.parentID, t.id, action, err)
		}

		// Report back.
		if err != ErrNil {
			if err := t.sendTerminalMsg(
				MsgTypeAbandon,
				container.New([]byte(err)),
			); err != ErrNil {
				log.Warningf("terminal: %s#%d failed to send terminal error: %s", t.parentID, t.id, err)
			}
		}

		// End all operations.
		t.lock.Lock()
		defer t.lock.Unlock()
		for _, op := range t.operations {
			op.End("", ErrNil)
		}

		// Stop all connected workers.
		t.cancelCtx()
	}
}
