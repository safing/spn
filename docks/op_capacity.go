package docks

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/terminal"
)

const (
	CapacityTestOpType = "capacity"

	defaultCapacityTestVolume      = 10000000  // 10MB with 100s timeout
	maxCapacityTestVolume          = 100000000 // 100MB with 1000s timeout
	capacityTestMsgSize            = 1000
	capacityTestTimeoutBasePerByte = 10 * time.Microsecond
	capacityTestSendTimeout        = 1 * time.Second
)

var (
	capacityTestSendData           = make([]byte, capacityTestMsgSize)
	capacityTestDataReceivedSignal = []byte("ACK")
)

type CapacityTestOp struct {
	terminal.OpBase
	t terminal.OpTerminal

	opts *CapacityTestOptions

	measureLock         sync.Mutex
	started             bool
	startTime           time.Time
	dataReceived        int
	dataReceivedAckSent bool
	dataSent            int
	dataSentAck         bool
	testResult          int

	result chan *terminal.Error
}

type CapacityTestOptions struct {
	TestVolume int
}

func (op *CapacityTestOp) Type() string {
	return CapacityTestOpType
}

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:     CapacityTestOpType,
		Requires: terminal.IsCraneController,
		RunOp:    runCapacityTestOp,
	})
}

func NewCapacityTestOp(t terminal.OpTerminal) (*CapacityTestOp, *terminal.Error) {
	// Create and init.
	op := &CapacityTestOp{
		t: t,
		opts: &CapacityTestOptions{
			TestVolume: defaultCapacityTestVolume,
		},
		result: make(chan *terminal.Error, 1),
	}
	op.OpBase.Init()

	// Make capacity test request.
	request, err := dsd.Dump(op.opts, dsd.CBOR)
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to serialize capactity test options: %w", err)
	}

	// Send test request.
	tErr := t.OpInit(op, container.New(request))
	if tErr != nil {
		return nil, tErr
	}
	return op, nil
}

func runCapacityTestOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Parse options.
	opts := &CapacityTestOptions{}
	_, err := dsd.Load(data.CompileData(), opts)
	if err != nil {
		return nil, terminal.ErrMalformedData.With("failed to parse options: %w", err)
	}

	// Create operation.
	op := &CapacityTestOp{
		t:      t,
		opts:   opts,
		result: make(chan *terminal.Error, 1),
	}
	op.OpBase.Init()
	op.OpBase.SetID(opID)

	// Start test.
	op.Deliver(nil)

	return op, nil
}

func (op *CapacityTestOp) testWorker(ctx context.Context) error {
	for {
		// Send next chunk.
		tErr := op.t.OpSendWithTimeout(op, container.New(capacityTestSendData), capacityTestSendTimeout)
		if tErr.IsError() {
			op.t.OpEnd(op, tErr.Wrap("failed to send capacity test data"))
		}

		// Check if op has ended.
		if op.HasEnded(false) {
			return nil
		}

		// Add to sent data counter and stop sending if sending is complete.
		if op.countSentData(len(capacityTestSendData)) {
			return nil
		}
	}
	return nil
}

func (op *CapacityTestOp) countSentData(amount int) (done bool) {
	op.measureLock.Lock()
	defer op.measureLock.Unlock()

	op.dataSent += len(capacityTestSendData)
	if op.dataSent >= op.opts.TestVolume {
		return true
	}
	return false
}

func (op *CapacityTestOp) Deliver(c *container.Container) *terminal.Error {
	op.measureLock.Lock()
	defer op.measureLock.Unlock()

	// Record start time if not started.
	if !op.started {
		op.started = true
		op.startTime = time.Now()

		// Start sender.
		module.StartWorker("op capacity sender", op.testWorker)

		// Check if only called for initialization.
		if c == nil {
			return nil
		}
	}

	// Add to received data counter.
	op.dataReceived += c.Length()

	// Check if we received the data received signal.
	if c.Length() == len(capacityTestDataReceivedSignal) &&
		bytes.Equal(c.CompileData(), capacityTestDataReceivedSignal) {
		op.dataSentAck = true
	}

	// Send the data received signal when we received the full test volume.
	if op.dataReceived >= op.opts.TestVolume && !op.dataReceivedAckSent {
		tErr := op.t.OpSendWithTimeout(op, container.New(capacityTestDataReceivedSignal), capacityTestSendTimeout)
		if tErr.IsError() {
			op.t.OpEnd(op, tErr.Wrap("failed to send data received signal"))
			return nil
		}
		op.dataReceivedAckSent = true
		return nil
	}

	// Check if we can complete the test.
	if op.dataReceived >= op.opts.TestVolume &&
		op.dataSent >= op.opts.TestVolume &&
		op.dataSentAck {

		// Calculate lane capacity and set it.
		timeNeeded := int(time.Since(op.startTime) / time.Second)
		if timeNeeded <= 0 {
			timeNeeded = 1
		}
		duplexBitRate := ((op.dataReceived + op.dataSent) * 8) / timeNeeded
		op.testResult = duplexBitRate / 2

		// Save the result to the crane.
		if controller, ok := op.t.(*CraneControllerTerminal); ok {
			controller.Crane.SetLaneCapacity(op.testResult)
		} else if !runningTests {
			log.Errorf("docks: capacity operation was run on terminal that is not a crane controller, but %T", op.t)
		}

		// Stop operation.
		op.t.OpEnd(op, nil)
	}

	return nil
}

func (op *CapacityTestOp) End(tErr *terminal.Error) {
	select {
	case op.result <- tErr:
	default:
	}
}

func (op *CapacityTestOp) Result() <-chan *terminal.Error {
	return op.result
}
