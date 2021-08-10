package terminal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
)

const CounterOpType string = "test/count"

type CounterOp struct {
	t    OpTerminal
	id   uint32
	wg   sync.WaitGroup
	wait time.Duration

	Counter uint64
	CountTo uint64
	Ended   *abool.AtomicBool
	Error   error
}

func init() {
	RegisterOpType(OpParams{
		Type:  CounterOpType,
		RunOp: runCounterOp,
	})
}

func NewCounterOp(t OpTerminal, countTo uint64, wait time.Duration) (*CounterOp, *Error) {
	// Create operation.
	op := &CounterOp{
		t:       t,
		wait:    wait,
		CountTo: countTo,
		Ended:   abool.New(),
	}
	op.wg.Add(1)

	// Create argument container.
	data := container.New(varint.Pack64(countTo))

	return op, t.OpInit(op, data)
}

func runCounterOp(t OpTerminal, opID uint32, data *container.Container) (Operation, *Error) {
	// Create operation.
	op := &CounterOp{
		t:     t,
		id:    opID,
		Ended: abool.New(),
	}
	op.wg.Add(1)

	// Parse arguments.
	countTo, err := data.GetNextN64()
	if err != nil {
		return nil, ErrMalformedData.With("failed to set up counter op: %w", err)
	}
	op.CountTo = countTo

	return op, nil
}

func (op *CounterOp) ID() uint32 {
	return op.id
}

func (op *CounterOp) SetID(id uint32) {
	op.id = id
}

func (op *CounterOp) Type() string {
	return CounterOpType
}

func (op *CounterOp) Deliver(data *container.Container) *Error {
	nextStep, err := data.GetNextN64()
	if err != nil {
		op.t.OpEnd(op, ErrMalformedData.With("failed to parse next number: %w", err))
		return nil
	}

	// Count and compare.
	op.Counter += 1
	if op.Counter != nextStep {
		log.Warningf(
			"terminal: integrity of counter op violated: have %d, expected %d",
			op.Counter,
			nextStep,
		)
		op.t.OpEnd(op, ErrIntegrity.With("counters mismatched"))
		return nil
	}

	// Check if we are done.
	if op.Counter >= op.CountTo {
		op.t.OpEnd(op, nil)
	}

	return nil
}

func (op *CounterOp) End(err *Error) {
	if op.Ended.SetToIf(false, true) {
		// Check if counting finished.
		if op.Counter < op.CountTo {
			err := fmt.Errorf("counter op %d: did not finish counting", op.id)
			op.Error = err
		}

		op.wg.Done()
	}
}

func (op *CounterOp) SendCounter() *Error {
	if op.Ended.IsSet() {
		return ErrStopping
	}

	op.Counter += 1
	return op.t.OpSend(op, container.New(varint.Pack64(op.Counter)))
}

func (op *CounterOp) Wait() {
	op.wg.Wait()
}

func (op *CounterOp) CounterWorker(ctx context.Context) error {
	var round uint64

	for {
		// Send counter msg.
		err := op.SendCounter()
		switch err {
		case nil:
			// All good, continue.
		case ErrStopping:
			// Done!
			return nil
		default:
			// Something went wrong.
			err := fmt.Errorf("counter op %d: failed to send counter: %s", op.id, err)
			op.Error = err
			return err
		}

		// Endless loop check
		round++
		if round > op.CountTo*2 {
			err := fmt.Errorf("counter op %d: looping more than it should", op.id)
			op.Error = err
			return err
		}

		// Maybe wait a little.
		if op.wait > 0 {
			time.Sleep(op.wait)
		}
	}
}
