package docks

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/rng"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/terminal"
)

const (
	LatencyTestOpType = "latency"

	latencyPingRequest  = 1
	latencyPingResponse = 2

	latencyTestNonceSize     = 16
	latencyTestRuns          = 10
	latencyTestPauseDuration = 1 * time.Second
	latencyTestOpTimeout     = latencyTestRuns * latencyTestPauseDuration * 3
)

type LatencyTestOp struct {
	terminal.OpBase
	t terminal.OpTerminal
}

type LatencyTestClientOp struct {
	LatencyTestOp

	lastPingSentAt    time.Time
	lastPingNonce     []byte
	measuredLatencies []time.Duration
	responses         chan *container.Container
	testResult        time.Duration

	result chan *terminal.Error
}

func (op *LatencyTestOp) Type() string {
	return LatencyTestOpType
}

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:     LatencyTestOpType,
		Requires: terminal.IsCraneController,
		RunOp:    runLatencyTestOp,
	})
}

func NewLatencyTestOp(t terminal.OpTerminal) (*LatencyTestClientOp, *terminal.Error) {
	// Create and init.
	op := &LatencyTestClientOp{
		LatencyTestOp: LatencyTestOp{
			t: t,
		},
		responses:         make(chan *container.Container),
		measuredLatencies: make([]time.Duration, 0, latencyTestRuns),
		result:            make(chan *terminal.Error, 1),
	}
	op.LatencyTestOp.OpBase.Init()

	// Make ping request.
	pingRequest, err := op.createPingRequest()
	if err != nil {
		return nil, terminal.ErrInternalError.With("%w", err)
	}

	// Send ping.
	tErr := t.OpInit(op, pingRequest)
	if tErr != nil {
		return nil, tErr
	}

	// Start handler.
	module.StartWorker("op latency handler", op.handler)

	return op, nil
}

func (op *LatencyTestClientOp) handler(ctx context.Context) error {
	returnErr := terminal.ErrStopping
	defer op.t.OpEnd(op, returnErr)

	var nextTest <-chan time.Time
	opTimeout := time.After(latencyTestOpTimeout)

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-opTimeout:
			return nil

		case <-nextTest:
			// Create ping request and send it.
			pingRequest, err := op.createPingRequest()
			if err != nil {
				returnErr = terminal.ErrInternalError.With("%w", err)
				return nil
			}
			tErr := op.t.OpSend(op, pingRequest)
			if tErr != nil {
				returnErr = tErr.Wrap("failed to send ping request")
				return nil
			}
			op.t.Flush()

			nextTest = nil

		case data := <-op.responses:
			// Check if the op ended.
			if data == nil {
				return nil
			}

			// Handle response
			tErr := op.handleResponse(data)
			if tErr != nil {
				returnErr = tErr
				return nil
			}

			// Check if we have enough latency tests.
			if len(op.measuredLatencies) >= latencyTestRuns {
				op.reportMeasuredLatencies()
				return nil
			}

			// Schedule next latency test, if not yet scheduled.
			if nextTest == nil {
				nextTest = time.After(latencyTestPauseDuration)
			}
		}
	}
}

func (op *LatencyTestClientOp) createPingRequest() (*container.Container, error) {
	// Generate nonce.
	nonce, err := rng.Bytes(latencyTestNonceSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create ping nonce")
	}

	// Set client request state.
	op.lastPingSentAt = time.Now()
	op.lastPingNonce = nonce

	return container.New(
		varint.Pack8(latencyPingRequest),
		nonce,
	), nil
}

func (op *LatencyTestClientOp) handleResponse(data *container.Container) *terminal.Error {
	rType, err := data.GetNextN8()
	if err != nil {
		return terminal.ErrMalformedData.With("failed to get response type: %w", err)
	}

	switch rType {
	case latencyPingResponse:
		// Check if the ping nonce matches.
		if !bytes.Equal(op.lastPingNonce, data.CompileData()) {
			return terminal.ErrIntegrity.With("ping nonce mismatch")
		}
		op.lastPingNonce = nil
		// Save latency.
		op.measuredLatencies = append(op.measuredLatencies, time.Since(op.lastPingSentAt))

		return nil
	default:
		return terminal.ErrIncorrectUsage.With("unknown response type")
	}
}

func (op *LatencyTestClientOp) reportMeasuredLatencies() {
	// Find lowest value.
	lowestLatency := time.Hour
	for _, latency := range op.measuredLatencies {
		if latency < lowestLatency {
			lowestLatency = latency
		}
	}
	op.testResult = lowestLatency

	// Save the result to the crane.
	if controller, ok := op.t.(*CraneControllerTerminal); ok {
		controller.Crane.SetLaneLatency(op.testResult)
	} else if !runningTests {
		log.Errorf("docks: latency operation was run on terminal that is not a crane controller, but %T", op.t)
	}
}

func (op *LatencyTestClientOp) Deliver(c *container.Container) *terminal.Error {
	// Optimized delivery with 1s timeout.
	select {
	case op.responses <- c:
	default:
		select {
		case op.responses <- c:
		case <-time.After(1 * time.Second):
			return terminal.ErrTimeout
		}
	}
	return nil
}

func (op *LatencyTestClientOp) End(tErr *terminal.Error) {
	close(op.responses)
	select {
	case op.result <- tErr:
	default:
	}
}

func (op *LatencyTestClientOp) Result() <-chan *terminal.Error {
	return op.result
}

func runLatencyTestOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Create operation.
	op := &LatencyTestOp{
		t: t,
	}
	op.OpBase.Init()
	op.OpBase.SetID(opID)

	// Handle first request.
	tErr := op.Deliver(data)
	if tErr != nil {
		return nil, tErr
	}

	return op, nil
}

func (op *LatencyTestOp) Deliver(c *container.Container) *terminal.Error {
	rType, err := c.GetNextN8()
	if err != nil {
		return terminal.ErrMalformedData.With("failed to get response type: %w", err)
	}

	switch rType {
	case latencyPingRequest:
		// Keep the nonce and just replace the msg type.
		c.PrependNumber(latencyPingResponse)

		// Send response.
		tErr := op.t.OpSend(op, c)
		if tErr != nil {
			return tErr.Wrap("failed to send ping response")
		}
		op.t.Flush()

		return nil

	default:
		return terminal.ErrIncorrectUsage.With("unknown request type")
	}
}

func (op *LatencyTestOp) End(tErr *terminal.Error) {}
