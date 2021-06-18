package terminal

import (
	"sync"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
)

const CounterOpType string = "test/count"

type CounterOp struct {
	t       OpTerminal
	id      uint32
	Counter uint64
	CountTo uint64
	Wait    sync.WaitGroup
	Ended   *abool.AtomicBool
}

func NewCounterOp(t OpTerminal, countTo uint64) (*CounterOp, Error) {
	// Create operation.
	op := &CounterOp{
		t:       t,
		CountTo: countTo,
		Ended:   abool.New(),
	}
	op.Wait.Add(1)

	// Create argument container.
	data := container.New(varint.Pack64(countTo))

	return op, t.OpInit(op, data)
}

func init() {
	RegisterOpType(OpParams{
		Type:  CounterOpType,
		RunOp: runCounterOp,
	})
}

func runCounterOp(t OpTerminal, opID uint32, data *container.Container) Operation {
	// Create operation.
	op := &CounterOp{
		t:     t,
		id:    opID,
		Ended: abool.New(),
	}
	op.Wait.Add(1)

	// Parse arguments.
	countTo, err := data.GetNextN64()
	if err != nil {
		log.Warningf("terminal: failed to set up counter op: %s", err)
		t.OpEnd(op, "run op", ErrMalformedData)
		return nil
	}
	op.CountTo = countTo

	return op
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

func (op *CounterOp) Deliver(data *container.Container) Error {
	nextStep, err := data.GetNextN64()
	if err != nil {
		log.Warningf("terminal: failed to handle counter op data: %s", err)
		op.t.OpEnd(op, "failed to parse nextStep", ErrMalformedData)
		return ErrNil
	}

	// Count and compare.
	op.Counter += 1
	if op.Counter != nextStep {
		log.Warningf(
			"terminal: integrity of counter op violated: have %d, expected %d",
			op.Counter,
			nextStep,
		)
		op.t.OpEnd(op, "counters mismatched", ErrIntegrity)
	}

	// Check if we are done.
	if op.Counter >= op.CountTo {
		op.t.OpEnd(op, "", ErrNil)
	}

	return ErrNil
}

func (op *CounterOp) End(action string, err Error) {
	op.Ended.Set()
	op.Wait.Done()
}

func (op *CounterOp) SendCounter() Error {
	if op.Ended.IsSet() {
		return ErrOpEnded
	}

	op.Counter += 1
	return op.t.OpSend(op, container.New(varint.Pack64(op.Counter)))
}
