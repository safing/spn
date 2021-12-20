package docks

import (
	"testing"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/spn/terminal"
)

func TestEffectiveBandwidth(t *testing.T) {
	var (
		bwTestDelay            = 100 * time.Millisecond
		bwTestQueueSize uint16 = 1000
		bwTestVolume           = 10000000 // 10MB
	)

	// Create test terminal pair.
	a, b, err := terminal.NewSimpleTestTerminalPair(
		bwTestDelay,
		&terminal.TerminalOpts{
			QueueSize: bwTestQueueSize,
		},
	)
	if err != nil {
		t.Fatalf("failed to create test terminal pair: %s", err)
	}

	// Grant permission for op on remote terminal and start op.
	b.GrantPermission(terminal.IsCraneController)

	// Re-use the capacity test for the bandwidth test.
	op := &CapacityTestOp{
		t: a,
		opts: &CapacityTestOptions{
			TestVolume: bwTestVolume,
		},
		result:  make(chan *terminal.Error, 1),
		started: true,
	}
	op.OpBase.Init()
	// Fake starting of sender in order to only transfer data in one direction.
	op.started = true
	op.startTime = time.Now()
	op.dataSent = bwTestVolume
	op.dataSentAck = true
	// Make capacity test request.
	request, err := dsd.Dump(op.opts, dsd.CBOR)
	if err != nil {
		t.Fatal(terminal.ErrInternalError.With("failed to serialize capactity test options: %w", err))
	}
	// Send test request.
	tErr := a.OpInit(op, container.New(request))
	if tErr != nil {
		t.Fatal(tErr)
	}

	// Wait for result and check error.
	tErr = <-op.Result()
	if tErr.IsError() {
		t.Fatalf("op failed: %s", tErr)
	}
	t.Logf("measured capacity: %d bit/s", op.testResult)

	// Calculate expected bandwidth.
	expectedBitsPerSecond := (float64(capacityTestMsgSize*8*int64(bwTestQueueSize)) / float64(bwTestDelay)) * float64(time.Second)
	t.Logf("expected capacity: %f bit/s", expectedBitsPerSecond)

	// Check if measured bandwidth is within parameters.
	if float64(op.testResult) > expectedBitsPerSecond*1.1 {
		t.Fatal("measured capacity too high")
	}
	// TODO: Check if we can raise this to at least 90%.
	if float64(op.testResult) < expectedBitsPerSecond*0.2 {
		t.Fatal("measured capacity too low")
	}
}