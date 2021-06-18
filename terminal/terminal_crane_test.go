package terminal

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/safing/portbase/container"
)

func init() {
	addPaddingTo = 0
}

func TestCraneTerminal(t *testing.T) {
	var term1 *CraneTerminal
	term1Submit := func(c *container.Container) {
		// Fast track nil containers.
		if c == nil {
			term1.DuplexFlowQueue.Deliver(c)
			return
		}

		// Log message.
		t.Logf("2>1: %v\n", c.CompileData())

		// Strip terminal ID, as we are not multiplexing in this test.
		_, err := c.GetNextN32()
		if err != nil {
			t.Errorf("failed to strip Terminal ID: %s", err)
		}

		// Deliver to other terminal.
		dErr := term1.DuplexFlowQueue.Deliver(c)
		if dErr != ErrNil {
			t.Errorf("failed to strip Terminal ID: %s", err)
			term1.End("delivery failed", ErrInternalError)
		}
	}

	var term2 *CraneTerminal
	term2Submit := func(c *container.Container) {
		// Fast track nil containers.
		if c == nil {
			term2.DuplexFlowQueue.Deliver(c)
			return
		}

		// Log message.
		t.Logf("1>2: %v\n", c.CompileData())

		// Strip terminal ID, as we are not multiplexing in this test.
		_, err := c.GetNextN32()
		if err != nil {
			t.Errorf("failed to strip Terminal ID: %s", err)
		}

		// Deliver to other terminal.
		dErr := term2.DuplexFlowQueue.Deliver(c)
		if dErr != ErrNil {
			t.Errorf("failed to strip Terminal ID: %s", err)
			term2.End("delivery failed", ErrInternalError)
		}
	}

	term1 = NewCraneTerminal(module.Ctx, "c1", 127, nil, term2Submit)
	term2 = NewCraneTerminal(module.Ctx, "c2", 127, nil, term1Submit)

	/*
		// Part 1
		// Test unidirectional traffic
		var submitted int
		for i := 0; i < 1000; i++ {
			// Submit message for sending.
			err := term1.addToOpMsgSendBuffer(
				64,
				MsgTypeData,
				container.New([]byte{8, 7, 6, 5, 4, 3, 2, 1}),
				// container.New([]byte("The quick brown fox something something something")),
				false,
			)
			if err != ErrNil {
				t.Errorf("failed to addToOpMsgSendBuffer: %s", err)
				break
			}
			// Force sending message.
			term1.Flush()
			// Count.
			submitted++
		}

		t.Logf("submitted: %d", submitted)
		printCTStats(t, "term1", term1)
		printCTStats(t, "term2", term2)
	*/

	term1counter1, err := NewCounterOp(term1, 10)
	if err != ErrNil {
		t.Fatalf("failed to start counter op: %s", err)
	}

	go func() {
		var round uint64
		for {
			// Send counter msg.
			err = term1counter1.SendCounter()
			switch err {
			case ErrNil:
				// All good.
			case ErrOpEnded:
				return // Done!
			default:
				// Something went wrong.
				t.Errorf("failed to send counter: %s", err)
				return
			}

			// Force sending message.
			term1.Flush()

			// Wait shortly.
			// In order for the test to succeed, this should be roughly the same as
			// the sendThresholdMaxWait.
			time.Sleep(sendThresholdMaxWait)

			// Endless loop check
			round++
			if round > term1counter1.CountTo*2 {
				t.Error("looping more than it should")
				return
			}
		}
	}()

	// Wait until done.
	term1counter1.Wait.Wait()

	t.Logf("term1counter1: counter=%d countTo=%d", term1counter1.Counter, term1counter1.CountTo)
	printCTStats(t, "term1", term1)
	printCTStats(t, "term2", term2)

	// Part 2
	// Test concurrent traffic

	// FIXME: continue testing with variations
}

func printCTStats(t *testing.T, name string, term *CraneTerminal) {
	t.Logf(
		"%s: sq=%d rq=%d ss=%d rs=%d opq=%d",
		name,
		len(term.DuplexFlowQueue.sendQueue),
		len(term.DuplexFlowQueue.recvQueue),
		atomic.LoadInt32(term.DuplexFlowQueue.sendSpace),
		atomic.LoadInt32(term.DuplexFlowQueue.reportedSpace),
		len(term.opMsgQueue),
	)
}
