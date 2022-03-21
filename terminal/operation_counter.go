package terminal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
)

// CounterOpType is the type ID for the Counter Operation.
const CounterOpType string = "debug/count"

// CounterOp sends increasing numbers on both sides.
type CounterOp struct { //nolint:maligned
	t      OpTerminal
	id     uint32
	wg     sync.WaitGroup
	server bool
	opts   *CounterOpts

	counterLock   sync.Mutex
	ClientCounter uint64
	ServerCounter uint64
	ended         *abool.AtomicBool
	Error         error
}

// CounterOpts holds the options for CounterOp.
type CounterOpts struct {
	ClientCountTo uint64
	ServerCountTo uint64
	Wait          time.Duration
	Flush         bool

	suppressWorker bool
}

func init() {
	RegisterOpType(OpParams{
		Type:  CounterOpType,
		RunOp: runCounterOp,
	})
}

// NewCounterOp returns a new CounterOp.
func NewCounterOp(t OpTerminal, opts CounterOpts) (*CounterOp, *Error) {
	// Create operation.
	op := &CounterOp{
		t:     t,
		opts:  &opts,
		ended: abool.New(),
	}
	op.wg.Add(1)

	// Create argument container.
	data, err := dsd.Dump(op.opts, dsd.JSON)
	if err != nil {
		return nil, ErrInternalError.With("failed to pack options: %w", err)
	}

	// Initialize operation.
	tErr := t.OpInit(op, container.New(data))
	if tErr != nil {
		return nil, tErr
	}

	// Start worker if needed.
	if op.getRemoteCounterTarget() > 0 && !op.opts.suppressWorker {
		module.StartWorker("counter sender", op.CounterWorker)
	}
	return op, nil
}

func runCounterOp(t OpTerminal, opID uint32, data *container.Container) (Operation, *Error) {
	// Create operation.
	op := &CounterOp{
		t:      t,
		id:     opID,
		server: true,
		ended:  abool.New(),
	}
	op.wg.Add(1)

	// Parse arguments.
	opts := &CounterOpts{}
	_, err := dsd.Load(data.CompileData(), opts)
	if err != nil {
		return nil, ErrInternalError.With("failed to unpack options: %w", err)
	}
	op.opts = opts

	// Start worker if needed.
	if op.getRemoteCounterTarget() > 0 {
		module.StartWorker("counter sender", op.CounterWorker)
	}

	return op, nil
}

// ID returns the ID of the operation.
func (op *CounterOp) ID() uint32 {
	return op.id
}

// SetID sets the ID of the operation.
func (op *CounterOp) SetID(id uint32) {
	op.id = id
}

// Type returns the type ID.
func (op *CounterOp) Type() string {
	return CounterOpType
}

// HasEnded returns whether the operation has ended.
func (op *CounterOp) HasEnded(end bool) bool {
	if end {
		// Return false if we just only it to ended.
		return !op.ended.SetToIf(false, true)
	}
	return op.ended.IsSet()
}

func (op *CounterOp) getCounter(sending, increase bool) uint64 {
	op.counterLock.Lock()
	defer op.counterLock.Unlock()

	// Use server counter, when op is server or for sending, but not when both.
	if op.server != sending {
		if increase {
			op.ServerCounter++
		}
		return op.ServerCounter
	}

	if increase {
		op.ClientCounter++
	}
	return op.ClientCounter
}

func (op *CounterOp) getRemoteCounterTarget() uint64 {
	if op.server {
		return op.opts.ClientCountTo
	}
	return op.opts.ServerCountTo
}

func (op *CounterOp) isDone() bool {
	op.counterLock.Lock()
	defer op.counterLock.Unlock()

	return op.ClientCounter >= op.opts.ClientCountTo &&
		op.ServerCounter >= op.opts.ServerCountTo
}

// Deliver delivers data to the operation.
func (op *CounterOp) Deliver(data *container.Container) *Error {
	nextStep, err := data.GetNextN64()
	if err != nil {
		op.t.OpEnd(op, ErrMalformedData.With("failed to parse next number: %w", err))
		return nil
	}

	// Count and compare.
	counter := op.getCounter(false, true)

	// Debugging:
	// if counter < 100 ||
	// 	counter < 1000 && counter%100 == 0 ||
	// 	counter < 10000 && counter%1000 == 0 ||
	// 	counter < 100000 && counter%10000 == 0 ||
	// 	counter < 1000000 && counter%100000 == 0 {
	// 	log.Errorf("terminal: counter %s>%d recvd, now at %d", op.t.FmtID(), op.id, counter)
	// }

	if counter != nextStep {
		log.Warningf(
			"terminal: integrity of counter op violated: received %d, expected %d",
			nextStep,
			counter,
		)
		op.t.OpEnd(op, ErrIntegrity.With("counters mismatched"))
		return nil
	}

	// Check if we are done.
	if op.isDone() {
		op.t.OpEnd(op, nil)
	}

	return nil
}

// End ends the operation.
func (op *CounterOp) End(err *Error) {
	// Check if counting finished.
	if !op.isDone() {
		err := fmt.Errorf(
			"counter op %d: did not finish counting (%d<-%d %d->%d)",
			op.id,
			op.opts.ClientCountTo, op.ClientCounter,
			op.ServerCounter, op.opts.ServerCountTo,
		)
		op.Error = err
	}

	op.wg.Done()
}

// SendCounter sends the next counter.
func (op *CounterOp) SendCounter() *Error {
	if op.ended.IsSet() {
		return ErrStopping
	}

	// Increase sending counter.
	counter := op.getCounter(true, true)

	// Debugging:
	// if counter < 100 ||
	// 	counter < 1000 && counter%100 == 0 ||
	// 	counter < 10000 && counter%1000 == 0 ||
	// 	counter < 100000 && counter%10000 == 0 ||
	// 	counter < 1000000 && counter%100000 == 0 {
	// 	defer log.Errorf("terminal: counter %s>%d sent, now at %d", op.t.FmtID(), op.id, counter)
	// }

	return op.t.OpSend(op, container.New(varint.Pack64(counter)))
}

// Wait waits for the Counter Op to finish.
func (op *CounterOp) Wait() {
	op.wg.Wait()
}

// CounterWorker is a worker that sends counters.
func (op *CounterOp) CounterWorker(ctx context.Context) error {
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
			err := fmt.Errorf("counter op %d: failed to send counter: %w", op.id, err)
			op.Error = err
			op.t.OpEnd(op, ErrInternalError.With(err.Error()))
			return nil
		}

		// Maybe flush message.
		if op.opts.Flush {
			op.t.Flush()
		}

		// Check if we are done with sending.
		if op.getCounter(true, false) >= op.getRemoteCounterTarget() {
			return nil
		}

		// Maybe wait a little.
		if op.opts.Wait > 0 {
			time.Sleep(op.opts.Wait)
		}
	}
}
