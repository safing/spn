package unit

import (
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

	// Start scheduler for test.
	cancel := startTestScheduler()
	defer cancel()

	// Create units.
	unitQ := make(chan *Unit, size)
	for i := 0; i < size; i++ {
		// Create new unit.
		u := New(nil)

		// Make 1% high priority.
		if rand.Int()%100 == 0 { //nolint:gosec // This is a test.
			u.MakeHighPriority()
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
				u.Start()
				u.Pause()

				time.Sleep(1 * time.Microsecond)

				u.Start()
				u.Finish()
			}
			wg.Done()
		}()
	}

	// Wait for workers to finish.
	wg.Wait()

	// Wait for two slot durations for values to update.
	time.Sleep(slotDuration * 2)

	// Print current state.
	fmt.Printf(`scheduler state:
		currentUnitID = %d
		finished = %d
		clearanceUpTo = %d
		
		slotPace = %d
		pausedUnits = %d
		highPrioUnits = %d
`,
		currentUnitID.Load(),
		finished.Load(),
		clearanceUpTo.Load(),
		slotPace.Load(),
		pausedUnits.Load(),
		highPrioUnits.Load(),
	)

	// Check if everything seems good.
	assert.Equal(t, size, int(currentUnitID.Load()), "currentUnitID must match size")
	assert.GreaterOrEqual(t, int(clearanceUpTo.Load()), size+minSlotPace, "clearanceUpTo must be at least size+minSlotPace")
	assert.Equal(t, 0, int(highPrioUnits.Load()), "high priority units must be zero when finished")
	assert.Equal(t, 0, int(pausedUnits.Load()), "paused units must be zero when finished")
}
