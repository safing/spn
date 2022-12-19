package unit

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUnit(t *testing.T) { //nolint:paralleltest
	size := 10000000
	workers := 100

	// Create and start scheduler.
	s := NewScheduler(&SchedulerConfig{
		EpochDuration: defaultSlotDuration * 10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := s.SlotScheduler(ctx)
		if err != nil {
			panic(err)
		}
	}()
	defer cancel()

	// Create unit creation worker.
	unitQ := make(chan *Unit, size/workers)
	go func() {
		for i := 0; i < size; i++ {
			// Create new unit.
			u := s.NewUnit()

			// Make 1% high priority.
			if rand.Int()%100 == 0 { //nolint:gosec // This is a test.
				u.MakeUnitHighPriority()
			}

			// Add to queue.
			unitQ <- u
		}
		close(unitQ)
	}()

	// Create 10 workers.
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			for u := range unitQ {
				u.WaitForUnitSlot()
				u.PauseUnit()

				time.Sleep(1 * time.Microsecond)

				u.WaitForUnitSlot()
				u.FinishUnit()
			}
			wg.Done()
		}()
	}

	// Wait for workers to finish.
	wg.Wait()

	// Wait for two slot durations for values to update.
	time.Sleep(s.config.SlotDuration * 10)

	// Print current state.
	fmt.Printf(`scheduler state:
		currentUnitID = %d
		slotPace = %d
		clearanceUpTo = %d
		finishedTotal = %d

		epoch = %d
		finished = %d
		pausedUnits = %d
		highPrioUnits = %d
`,
		s.currentUnitID.Load(),
		s.slotPace.Load(),
		s.clearanceUpTo.Load(),
		s.finishedTotal.Load(),

		s.epoch.Load(),
		s.finished.Load(),
		s.pausedUnits.Load(),
		s.highPrioUnits.Load(),
	)

	// Check if everything seems good.
	assert.Equal(t, size, int(s.currentUnitID.Load()), "currentUnitID must match size")
	assert.Equal(t, size, int(s.finishedTotal.Load()), "finishedTotal must match size")
	assert.GreaterOrEqual(t, int(s.clearanceUpTo.Load()), size+int(s.config.MinSlotPace), "clearanceUpTo must be at least size+minSlotPace")
	assert.Equal(t, 0, int(s.highPrioUnits.Load()), "high priority units must be zero when finished")
	assert.Equal(t, 0, int(s.pausedUnits.Load()), "paused units must be zero when finished")

	// Shutdown
	cancel()
	time.Sleep(s.config.SlotDuration * 10)

	// Check if scheduler shut down correctly.
	assert.Equal(t, math.MaxInt64-math.MaxInt32, int(s.clearanceUpTo.Load()), "clearance must be near MaxInt64")
}
