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
	submitUpstream func(*container.Container) *Error,
) (*TestTerminal, *container.Container, *Error) {
	// Create Terminal Base.
	t, initData, err := NewLocalBaseTerminal(ctx, id, parentID, remoteHub, initMsg, submitUpstream, false)
	if err != nil {
		return nil, nil, err
	}
	// Disable adding ID/Type to messages, as test terminals are connected directly.
	t.addTerminalIDType = false
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
	submitUpstream func(*container.Container) *Error,
) (*TestTerminal, *TerminalOpts, *Error) {
	// Create Terminal Base.
	t, initMsg, err := NewRemoteBaseTerminal(ctx, id, parentID, identity, initData, submitUpstream, false)
	if err != nil {
		return nil, nil, err
	}
	t.StartWorkers(module, "test terminal")

	return &TestTerminal{t}, initMsg, nil
}

type delayedMsg struct {
	data       *container.Container
	delayUntil time.Time
}

func createDelayingTestForwardingFunc(
	srcName,
	dstName string,
	delay time.Duration,
	delayQueueSize int,
	deliverFunc func(*container.Container) *Error,
) func(*container.Container) *Error {
	// Return simple forward func if no delay is given.
	if delay == 0 {
		return func(c *container.Container) *Error {
			// Deliver to other terminal.
			dErr := deliverFunc(c)
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
			dErr := deliverFunc(msg.data)
			if dErr != nil {
				log.Errorf("spn/testing: %s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
			}
		}
	}()

	return func(c *container.Container) *Error {
		// Add msg to delaying msg channel.
		delayedMsgs <- &delayedMsg{
			data:       c,
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
			"a", "b", delay, delayQueueSize, func(c *container.Container) *Error {
				return b.Deliver(c)
			},
		),
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create local test terminal")
	}
	b, _, tErr = NewRemoteTestTerminal(
		module.Ctx, 127, "b", nil, initData, createDelayingTestForwardingFunc(
			"b", "a", delay, delayQueueSize, func(c *container.Container) *Error {
				return a.Deliver(c)
			},
		),
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create remote test terminal")
	}

	return a, b, nil
}
