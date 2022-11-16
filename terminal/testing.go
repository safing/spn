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
	logTestCraneMsgs     = true
)

// TestTerminal is a terminal for running tests.
type TestTerminal struct {
	*TerminalBase
}

// NewLocalTestTerminal returns a new local test terminal.
func NewLocalTestTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	remoteHub *hub.Hub,
	initMsg *TerminalOpts,
	submitUpstream func(msg *Msg) *Error,
) (*TestTerminal, *container.Container, *Error) {
	// Create Terminal Base.
	t, initData, err := NewLocalBaseTerminal(ctx, id, parentID, remoteHub, initMsg, submitUpstream)
	if err != nil {
		return nil, nil, err
	}
	t.StartWorkers(module, "test terminal")

	return &TestTerminal{t}, initData, nil
}

// NewRemoteTestTerminal returns a new remote test terminal.
func NewRemoteTestTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	identity *cabin.Identity,
	initData *container.Container,
	submitUpstream func(msg *Msg) *Error,
) (*TestTerminal, *TerminalOpts, *Error) {
	// Create Terminal Base.
	t, initMsg, err := NewRemoteBaseTerminal(ctx, id, parentID, identity, initData, submitUpstream)
	if err != nil {
		return nil, nil, err
	}
	t.StartWorkers(module, "test terminal")

	return &TestTerminal{t}, initMsg, nil
}

type delayedMsg struct {
	msg        *Msg
	delayUntil time.Time
}

func createDelayingTestForwardingFunc(
	srcName,
	dstName string,
	delay time.Duration,
	delayQueueSize int,
	deliverFunc func(msg *Msg) *Error,
) func(msg *Msg) *Error {
	// Return simple forward func if no delay is given.
	if delay == 0 {
		return func(msg *Msg) *Error {
			// Deliver to other terminal.
			dErr := deliverFunc(msg)
			if dErr != nil {
				log.Errorf("spn/testing: %s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
				return dErr
			}
			return nil
		}
	}

	// If there is delay, create a delaying channel and handler.
	delayedMsgs := make(chan *delayedMsg, delayQueueSize)
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
			dErr := deliverFunc(msg.msg)
			if dErr != nil {
				log.Errorf("spn/testing: %s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
			}
		}
	}()

	return func(msg *Msg) *Error {
		// Add msg to delaying msg channel.
		delayedMsgs <- &delayedMsg{
			msg:        msg,
			delayUntil: time.Now().Add(delay),
		}
		return nil
	}
}

// Stop stops the terminal.
func (t *TestTerminal) Stop(err *Error) {
	if t.Abandoning.SetToIf(false, true) {
		switch err {
		case nil:
			// nil means that the Terminal is being shutdown by the owner.
			log.Tracef("spn/terminal: %s is closing", fmtTerminalID(t.parentID, t.id))
		default:
			// All other errors are faults.
			log.Warningf("spn/terminal: %s: %s", fmtTerminalID(t.parentID, t.id), err)
		}

		// End all operations and stop all connected workers.
		t.StartAbandonProcedure(err, true, nil)
	}
}

// NewSimpleTestTerminalPair provides a simple conntected terminal pair for tests.
func NewSimpleTestTerminalPair(delay time.Duration, delayQueueSize int, opts *TerminalOpts) (a, b *TestTerminal, err error) {
	if opts == nil {
		opts = &TerminalOpts{
			Padding:         defaultTestPadding,
			FlowControl:     FlowControlDFQ,
			FlowControlSize: defaultTestQueueSize,
		}
	}

	var initData *container.Container
	var tErr *Error
	a, initData, tErr = NewLocalTestTerminal(
		module.Ctx, 127, "a", nil, opts, createDelayingTestForwardingFunc(
			"a", "b", delay, delayQueueSize, func(msg *Msg) *Error {
				return b.Deliver(msg)
			},
		),
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create local test terminal")
	}
	b, _, tErr = NewRemoteTestTerminal(
		module.Ctx, 127, "b", nil, initData, createDelayingTestForwardingFunc(
			"b", "a", delay, delayQueueSize, func(msg *Msg) *Error {
				return a.Deliver(msg)
			},
		),
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create remote test terminal")
	}

	return a, b, nil
}
