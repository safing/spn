package terminal

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"sync/atomic"
	"testing"
	"time"

	"github.com/safing/spn/hub"

	"github.com/safing/spn/cabin"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
)

const (
	logTestCraneMsgs = false
	testPadding      = 8
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
	module.StartWorker("test terminal", ct.Handler)
	module.StartWorker("test terminal flow queue", ct.FlowHandler)

	return ct
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
		t.StopAll(nil)
	}
}

func TestTerminals(t *testing.T) {
	var testQueueSize uint16 = DefaultQueueSize
	countToQueueSize := uint64(testQueueSize)

	initMsg := &TerminalOpts{
		QueueSize: testQueueSize,
		Padding:   testPadding,
	}

	var term1 *TestTerminal
	var term2 *TestTerminal
	var initData *container.Container
	var err *Error
	term1, initData, err = NewLocalTestTerminal(
		module.Ctx, 127, "c1", nil, initMsg, createTestForwardingFunc(
			t, "c1", "c2", func(c *container.Container) *Error {
				return term2.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if err != nil {
		t.Fatalf("failed to create local terminal: %s", err)
	}
	term2, _, err = NewRemoteTestTerminal(
		module.Ctx, 127, "c2", nil, initData, createTestForwardingFunc(
			t, "c2", "c1", func(c *container.Container) *Error {
				return term1.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if err != nil {
		t.Fatalf("failed to create remote terminal: %s", err)
	}

	// Start testing with counters.

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway-flushing-waiting",
		oneWay:          true,
		flush:           true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway-waiting",
		oneWay:          true,
		countTo:         10,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway-flushing",
		oneWay:          true,
		flush:           true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway",
		oneWay:          true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway-flushing-waiting",
		flush:           true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway-waiting",
		flush:           true,
		countTo:         10,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway-flushing",
		flush:           true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway",
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "stresstest",
		countTo:         1000000,
		waitBetweenMsgs: time.Microsecond,
	})
}

func TestTerminalsWithEncryption(t *testing.T) {
	var testQueueSize uint16 = DefaultQueueSize
	countToQueueSize := uint64(testQueueSize)

	initMsg := &TerminalOpts{
		QueueSize: testQueueSize,
		Padding:   8,
	}

	identity, erro := cabin.CreateIdentity(module.Ctx, hub.ScopeTest)
	if erro != nil {
		t.Fatalf("failed to create identity: %s", erro)
	}
	_, erro = identity.MaintainExchKeys(time.Now())
	if erro != nil {
		t.Fatalf("failed to maintain exchange keys: %s", erro)
	}

	var term1 *TestTerminal
	var term2 *TestTerminal
	var initData *container.Container
	var err *Error
	term1, initData, err = NewLocalTestTerminal(
		module.Ctx, 127, "c1", identity.Hub(), initMsg, createTestForwardingFunc(
			t, "c1", "c2", func(c *container.Container) *Error {
				return term2.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if err != nil {
		t.Fatalf("failed to create local terminal: %s", err)
	}
	term2, _, err = NewRemoteTestTerminal(
		module.Ctx, 127, "c2", identity, initData, createTestForwardingFunc(
			t, "c2", "c1", func(c *container.Container) *Error {
				return term1.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if err != nil {
		t.Fatalf("failed to create remote terminal: %s", err)
	}

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway-encrypting",
		countTo:         countToQueueSize * 20,
		waitBetweenMsgs: time.Millisecond,
	})
}

func createTestForwardingFunc(t *testing.T, srcName, dstName string, deliverFunc func(*container.Container) *Error) func(*container.Container) {
	return func(c *container.Container) {
		// Fast track nil containers.
		if c == nil {
			dErr := deliverFunc(c)
			if dErr != nil {
				t.Errorf("%s>%s: failed to deliver nil msg to terminal: %s", srcName, dstName, dErr)
			}
			return
		}

		// Log messages.
		if logTestCraneMsgs {
			t.Logf("%s>%s: %v\n", srcName, dstName, c.CompileData())
		}

		// Strip message type, as we are not multiplexing in this test.
		_, err := c.GetNextN32()
		if err != nil {
			t.Errorf("%s>%s: failed to strip terminal ID: %s", srcName, dstName, err)
			return
		}

		// Strip terminal ID, as we are not multiplexing in this test.
		_, err = c.GetNextN32()
		if err != nil {
			t.Errorf("%s>%s: failed to strip terminal ID: %s", srcName, dstName, err)
			return
		}

		// Deliver to other terminal.
		dErr := deliverFunc(c)
		if dErr != nil {
			t.Errorf("%s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
		}
	}
}

type testWithCounterOpts struct {
	testName        string
	oneWay          bool
	flush           bool
	countTo         uint64
	waitBetweenMsgs time.Duration
}

func testTerminalWithCounters(t *testing.T, term1, term2 *TestTerminal, opts *testWithCounterOpts) {
	var counter1, counter2 *CounterOp

	// Wait async for test to complete, print stack after timeout.
	finished := make(chan struct{})
	go func() {
		select {
		case <-finished:
		case <-time.After(60 * time.Second):
			fmt.Printf("terminal test %s is taking too long, print stack:\n", opts.testName)
			_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			os.Exit(1)
		}
	}()

	t.Logf("starting terminal counter test %s", opts.testName)
	defer t.Logf("stopping terminal counter test %s", opts.testName)

	// Start counters.
	counter1 = runTerminalCounter(t, term1, opts)
	if !opts.oneWay {
		counter2 = runTerminalCounter(t, term2, opts)
	}

	// Wait until counters are done.
	counter1.Wait()
	if !opts.oneWay {
		counter2.Wait()
	}
	close(finished)

	// Log stats.
	t.Logf("%s: counter1: counter=%d countTo=%d", opts.testName, counter1.Counter, counter1.CountTo)
	if counter1.Counter < counter1.CountTo {
		t.Errorf("%s: did not finish counting", opts.testName)
	}
	if !opts.oneWay {
		t.Logf("%s: counter2: counter=%d countTo=%d", opts.testName, counter2.Counter, counter2.CountTo)
		if counter2.Counter < counter2.CountTo {
			t.Errorf("%s: did not finish counting", opts.testName)
		}
	}
	printCTStats(t, opts.testName, "term1", term1)
	printCTStats(t, opts.testName, "term2", term2)
}

func runTerminalCounter(t *testing.T, term *TestTerminal, opts *testWithCounterOpts) *CounterOp {
	counter, err := NewCounterOp(term, opts.countTo, opts.waitBetweenMsgs)
	if err != nil {
		t.Fatalf("%s: %s: failed to start counter op: %s", opts.testName, term.parentID, err)
		return nil
	}

	go func() {
		var round uint64
		for {
			// Send counter msg.
			err = counter.SendCounter()
			switch err {
			case nil:
				// All good.
			case ErrStopping:
				return // Done!
			default:
				// Something went wrong.
				t.Errorf("%s: %s: failed to send counter: %s", opts.testName, term.parentID, err)
				return
			}

			if opts.flush {
				// Force sending message.
				term.Flush()
			}

			if opts.waitBetweenMsgs > 0 {
				// Wait shortly.
				// In order for the test to succeed, this should be roughly the same as
				// the sendThresholdMaxWait.
				time.Sleep(opts.waitBetweenMsgs)
			}

			// Endless loop check
			round++
			if round > counter.CountTo*2 {
				t.Errorf("%s: %s: looping more than it should", opts.testName, term.parentID)
				return
			}

			// Log progress
			if round%100000 == 0 {
				t.Logf("%s: %s: sent %d messages", opts.testName, term.parentID, round)
			}
		}
	}()

	return counter
}

func printCTStats(t *testing.T, testName, name string, term *TestTerminal) {
	t.Logf(
		"%s: %s: sq=%d rq=%d sends=%d reps=%d opq=%d",
		testName,
		name,
		len(term.DuplexFlowQueue.sendQueue),
		len(term.DuplexFlowQueue.recvQueue),
		atomic.LoadInt32(term.DuplexFlowQueue.sendSpace),
		atomic.LoadInt32(term.DuplexFlowQueue.reportedSpace),
		len(term.opMsgQueue),
	)
}
