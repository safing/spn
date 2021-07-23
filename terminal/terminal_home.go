package terminal

import (
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/tevino/abool"
)

type HomeTerminal struct {
	*TerminalBase

	// HubID is the ID of the Hub that this Terminal is connected to.
	HubID string
	// Path holds the path through the network.
	Path []string
}

type HomeCraneTerminal struct {
	HomeTerminal
	*DuplexFlowQueue
}

/*
func NewHomeTerminal(hubID string) *HomeTerminal {
	// Create Terminal Base.
	t := NewTerminalBase(id, initData)

	// Create Home Terminal and assign it as the extended Terminal.
	ht := &HomeTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
	}
	t.ext = ht

	return ht
}

func NewHomeCraneTerminal(
	ctx context.Context,
	id uint32,
	initData *container.Container,
	submitUpstream func(*container.Container),
) *HomeCraneTerminal {
	// Create Terminal Base.
	t := NewTerminalBase(id, initData)
	atomic.StoreUint32(t.nextOpID, 0)

	// Create Flow Queue.
	dfq := NewDuplexFlowQueue(id, t.ctx, defaultQueueSize, submitUpstream)

	return &HomeCraneTerminal{
		HomeTerminal: HomeTerminal{
			TerminalBase: t,
		},
		DuplexFlowQueue: dfq,
	}
}
*/

var (
	notifyAbandoned        func(t *HomeTerminal, err Error)
	notifyAbandonedEnabled = abool.NewBool(false)
	notifyAbandonedReady   = abool.NewBool(false)
)

// SetNotifyAbandonedFunc sets a notify function that is called whenever a HomeTerminal is abandoned.
func SetNotifyAbandonedFunc(fn func(t *HomeTerminal, err Error)) {
	if notifyAbandonedEnabled.SetToIf(false, true) {
		notifyAbandoned = fn
		notifyAbandonedReady.Set()
	}
}

func (t *HomeTerminal) abandonNotify(err Error) {
	if notifyAbandonedReady.IsSet() {
		notifyAbandoned(t, err)
	}
}

func (t *HomeTerminal) Abandon(action string, err Error) {
	if t.Abandoned.SetToIf(false, true) {
		switch err {
		case ErrNil:
			// ErrNil means that the Terminal is being shutdown by the owner.
			log.Tracef("spn/terminal: %s is closing", fmtTerminalID(t.parentID, t.id))
		default:
			// All other errors are faults.
			log.Warningf("spn/terminal: %s %s: %s", fmtTerminalID(t.parentID, t.id), action, err)
		}

		// Notify other components of failure.
		t.abandonNotify(err)

		// Report back.
		if err != ErrNil {
			if err := t.sendTerminalMsg(
				MsgTypeAbandon,
				container.New([]byte(err)),
			); err != ErrNil {
				log.Warningf("spn/terminal: %s failed to send terminal error: %s", fmtTerminalID(t.parentID, t.id), err)
			}
		}

		// End all operations and stop all connected workers.
		t.StopAll("received a", ErrCascading)
	}
}
