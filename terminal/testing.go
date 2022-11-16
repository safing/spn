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
	upstream Upstream,
) (*TestTerminal, *container.Container, *Error) {
	// Create Terminal Base.
	t, initData, err := NewLocalBaseTerminal(ctx, id, parentID, remoteHub, initMsg, upstream)
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
	upstream Upstream,
) (*TestTerminal, *TerminalOpts, *Error) {
	// Create Terminal Base.
	t, initMsg, err := NewRemoteBaseTerminal(ctx, id, parentID, identity, initData, upstream)
	if err != nil {
		return nil, nil, err
	}
	t.StartWorkers(module, "test terminal")

	return &TestTerminal{t}, initMsg, nil
}

type delayedMsg struct {
	msg        *Msg
	timeout    time.Duration
	delayUntil time.Time
}

func createDelayingTestForwardingFunc(
	srcName,
	dstName string,
	delay time.Duration,
	delayQueueSize int,
	deliverFunc func(msg *Msg, timeout time.Duration) *Error,
) func(msg *Msg, timeout time.Duration) *Error {
	// Return simple forward func if no delay is given.
	if delay == 0 {
		return func(msg *Msg, timeout time.Duration) *Error {
			// Deliver to other terminal.
			dErr := deliverFunc(msg, timeout)
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
			dErr := deliverFunc(msg.msg, msg.timeout)
			if dErr != nil {
				log.Errorf("spn/testing: %s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
			}
		}
	}()

	return func(msg *Msg, timeout time.Duration) *Error {
		// Add msg to delaying msg channel.
		delayedMsgs <- &delayedMsg{
			msg:        msg,
			timeout:    timeout,
			delayUntil: time.Now().Add(delay),
		}
		return nil
	}
}

// HandleAbandon gives the terminal the ability to cleanly shut down.
// The returned error is the error to send to the other side.
// Should never be called directly. Call Abandon() instead.
func (t *TestTerminal) HandleAbandon(err *Error) (errorToSend *Error) {
	switch err {
	case nil:
		// nil means that the Terminal is being shutdown by the owner.
		log.Tracef("spn/terminal: %s is closing", fmtTerminalID(t.parentID, t.id))
	default:
		// All other errors are faults.
		log.Warningf("spn/terminal: %s: %s", fmtTerminalID(t.parentID, t.id), err)
	}

	return
}

type upstreamProxy struct {
	send func(msg *Msg, timeout time.Duration) *Error
}

func (up *upstreamProxy) Send(msg *Msg, timeout time.Duration) *Error {
	return up.send(msg, timeout)
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
		module.Ctx, 127, "a", nil, opts, &upstreamProxy{
			send: createDelayingTestForwardingFunc(
				"a", "b", delay, delayQueueSize, func(msg *Msg, timeout time.Duration) *Error {
					return b.Deliver(msg)
				},
			),
		},
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create local test terminal")
	}
	b, _, tErr = NewRemoteTestTerminal(
		module.Ctx, 127, "b", nil, initData, &upstreamProxy{
			send: createDelayingTestForwardingFunc(
				"b", "a", delay, delayQueueSize, func(msg *Msg, timeout time.Duration) *Error {
					return a.Deliver(msg)
				},
			),
		},
	)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to create remote test terminal")
	}

	return a, b, nil
}
