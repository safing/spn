package unit

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUnit(t *testing.T) { //nolint:paralleltest
	size := 1000000
	workers := 10

	// Create and start scheduler.
	s := NewScheduler(nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := s.SlotScheduler(ctx)
		if err != nil {
			panic(err)
		}
	}()
	defer cancel()

	// Create units.
	unitQ := make(chan *Unit, size)
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
	time.Sleep(s.config.SlotDuration * 2)

	// Print current state.
	fmt.Printf(`scheduler state:
		currentUnitID = %d
		finished = %d
		clearanceUpTo = %d
		
		slotPace = %d
		pausedUnits = %d
		highPrioUnits = %d
`,
		s.currentUnitID.Load(),
		s.finished.Load(),
		s.clearanceUpTo.Load(),
		s.slotPace.Load(),
		s.pausedUnits.Load(),
		s.highPrioUnits.Load(),
	)

	// Check if everything seems good.
	assert.Equal(t, size, int(s.currentUnitID.Load()), "currentUnitID must match size")
	assert.GreaterOrEqual(t, int(s.clearanceUpTo.Load()), size+int(s.config.MinSlotPace), "clearanceUpTo must be at least size+minSlotPace")
	assert.Equal(t, 0, int(s.highPrioUnits.Load()), "high priority units must be zero when finished")
	assert.Equal(t, 0, int(s.pausedUnits.Load()), "paused units must be zero when finished")
}
