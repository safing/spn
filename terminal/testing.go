package terminal

import (
	"context"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/hub"
)

const (
	defaultTestQueueSize = 16
	defaultTestPadding   = 8
	logTestCraneMsgs     = false
)

type TestTerminal struct {
	*TerminalBase
	*DuplexFlowQueue
}

func NewLocalTestTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	remoteHub *hub.Hub,
	initMsg *TerminalOpts,
	submitUpstream func(*container.Container),
) (*TestTerminal, *container.Container, *Error) {
	// Create Terminal Base.
	t, initData, err := NewLocalBaseTerminal(ctx, id, parentID, remoteHub, initMsg)
	if err != nil {
		return nil, nil, err
	}

	return initTestTerminal(t, initMsg, submitUpstream), initData, nil
}

func NewRemoteTestTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	identity *cabin.Identity,
	initData *container.Container,
	submitUpstream func(*container.Container),
) (*TestTerminal, *TerminalOpts, *Error) {
	// Create Terminal Base.
	t, initMsg, err := NewRemoteBaseTerminal(ctx, id, parentID, identity, initData)
	if err != nil {
		return nil, nil, err
	}

	return initTestTerminal(t, initMsg, submitUpstream), initMsg, nil
}

func initTestTerminal(
	t *TerminalBase,
	initMsg *TerminalOpts,
	submitUpstream func(*container.Container),
) *TestTerminal {
	// Create Flow Queue.
	dfq := NewDuplexFlowQueue(t, initMsg.QueueSize, submitUpstream)

	// Create Crane Terminal and assign it as the extended Terminal.
	ct := &TestTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
	}
	t.SetTerminalExtension(ct)

	// Start workers.
	module.StartWorker("test terminal handler", ct.Handler)
	module.StartWorker("test terminal sender", ct.Sender)
	module.StartWorker("test terminal flow queue", ct.FlowHandler)

	return ct
}

type delayedMsg struct {
	data       *container.Container
	delayUntil time.Time
}

func createDelayingTestForwardingFunc(
	srcName,
	dstName string,
	delay time.Duration,
	deliverFunc func(*container.Container) *Error,
) func(*container.Container) {
	// Return simple forward func if no delay is given.
	if delay == 0 {
		return func(c *container.Container) {
			// Deliver to other terminal.
			dErr := deliverFunc(c)
			if dErr != nil {
				log.Errorf("%s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
			}
		}
	}

	// If there is delay, create a delaying channel and handler.
	delayedMsgs := make(chan *delayedMsg, 1000)
	go func() {
		for {
			// Read from chan
			msg := <-delayedMsgs
			if msg == nil {
				return
			}

			// Check if we need to wait.
			waitFor := time.Until(msg.delayUntil)
			if waitFor > 0 {
				time.Sleep(waitFor)
			}

			// Deliver to other terminal.
			dErr := deliverFunc(msg.data)
			if dErr != nil {
				log.Errorf("%s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
			}
		}
	}()

	return func(c *container.Container) {
		// Add msg to delaying msg channel.
		delayedMsgs <- &delayedMsg{
			data:       c,
			delayUntil: time.Now().Add(delay),
		}
	}
}

func (t *TestTerminal) Flush() {
	t.TerminalBase.Flush()
	t.DuplexFlowQueue.Flush()
}

func (t *TestTerminal) Abandon(err *Error) {
	if t.Abandoned.SetToIf(false, true) {
		switch err {
		case nil:
			// nil means that the Terminal is being shutdown by the owner.
			log.Tracef("spn/terminal: %s is closing", fmtTerminalID(t.parentID, t.id))
		default:
			// All other errors are faults.
			log.Warningf("spn/terminal: %s: %s", fmtTerminalID(t.parentID, t.id), err)
		}

		// End all operations and stop all connected workers.
		t.Shutdown(nil, true)
	}
}

func NewSimpleTestTerminalPair(delay time.Duration, opts *TerminalOpts) (a, b *TestTerminal, err error) {
	if opts == nil {
		opts = &TerminalOpts{
			QueueSize: defaultTestQueueSize,
			Padding:   defaultTestPadding,
		}
	}

	var initData *container.Container
	var tErr *Error
	a, initData, tErr = NewLocalTestTerminal(
		module.Ctx, 127, "a", nil, opts, createDelayingTestForwardingFunc(
			"a", "b", delay, func(c *container.Container) *Error {
				return b.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create local test terminal")
	}
	b, _, tErr = NewRemoteTestTerminal(
		module.Ctx, 127, "b", nil, initData, createDelayingTestForwardingFunc(
			"b", "a", delay, func(c *container.Container) *Error {
				return a.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create remote test terminal")
	}

	return a, b, nil
}
