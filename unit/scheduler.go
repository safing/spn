package unit

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"
)

const (
	slotDuration = 10 * time.Millisecond
	minSlotPace  = 1000

	adjustFractionPerStreak = 1000 // 0.1%
)

var (
	// With just 100B per packet, a uint64 is enough for over 1800 Exabyte. No need for overflow support.
	currentUnitID = atomic.Uint64{}
	finished      = atomic.Uint64{}
	clearanceUpTo = atomic.Uint64{}

	slotPace      = atomic.Int64{}
	pausedUnits   = atomic.Int64{}
	highPrioUnits = atomic.Int64{}

	slotSignalA      = make(chan struct{})
	slotSignalB      = make(chan struct{})
	slotSignalSwitch bool
	slotSignalsLock  sync.RWMutex

	schedulerRunning = abool.New()
)

func init() {
	clearanceUpTo.Store(minSlotPace)
	slotPace.Store(minSlotPace)
}

func nextSlotSignal() chan struct{} {
	slotSignalsLock.RLock()
	defer slotSignalsLock.RUnlock()

	if slotSignalSwitch {
		return slotSignalA
	}
	return slotSignalB
}

func announceNextSlot() {
	slotSignalsLock.Lock()
	defer slotSignalsLock.Unlock()

	// Close new slot signal and refresh previous one.
	if slotSignalSwitch {
		close(slotSignalA)
		slotSignalB = make(chan struct{})
	} else {
		close(slotSignalB)
		slotSignalA = make(chan struct{})
	}

	// Switch to next slot.
	slotSignalSwitch = !slotSignalSwitch
}

func slotScheduler(ctx context.Context) error {
	// Only run one scheduler at once.
	if !schedulerRunning.SetToIf(false, true) {
		return errors.New("scheduler is already running")
	}
	defer schedulerRunning.UnSet()

	// Start slot ticker.
	ticker := time.NewTicker(slotDuration)
	defer ticker.Stop()

	// Give clearance to all when stopping.
	defer clearanceUpTo.Store(math.MaxUint64)

	var (
		lastClearanceAmount int64
		finishedAtStart     = finished.Load()
		increaseStreak      int64
		decreaseStreak      int64
	)
	for range ticker.C {
		// Calculate how many units were finished in slot.
		// Only load "finished" once, so we don't miss anything.
		finishedAtEnd := finished.Load()
		finishedInSlot := int64(finishedAtEnd - finishedAtStart)

		// Adapt pace.
		if finishedInSlot >= lastClearanceAmount {
			// Adjust based on streak.
			increaseStreak++
			decreaseStreak = 0
			slotPace.Add((slotPace.Load() / adjustFractionPerStreak) * increaseStreak)

			// Debug logging:
			// fmt.Printf("+++ slot pace: %d (finished in slot: %d, last clearance: %d, increaseStreak: %d)\n", slotPace.Load(), finishedInSlot, lastClearanceAmount, increaseStreak)
		} else {
			// Adjust based on streak.
			decreaseStreak++
			increaseStreak = 0
			slotPace.Add(-((slotPace.Load() / adjustFractionPerStreak) * decreaseStreak))

			// Enforce minimum.
			if slotPace.Load() < minSlotPace {
				slotPace.Store(minSlotPace)
				decreaseStreak = 0
			}

			// Debug logging:
			// fmt.Printf("--- slot pace: %d (finished in slot: %d, last clearance: %d, decreaseStreak: %d)\n", slotPace.Load(), finishedInSlot, lastClearanceAmount, decreaseStreak)
		}

		// Set new slot clearance.
		// First, add current pace and paused units.
		newClearance := slotPace.Load() + pausedUnits.Load()
		// Second, subtract up to 20% of clearance for high priority units.
		highPrio := highPrioUnits.Load()
		if highPrio > newClearance/5 {
			newClearance -= newClearance / 5
		} else {
			newClearance -= highPrio
		}
		// Third, add finished to set new clearance limit.
		clearanceUpTo.Store(finishedAtEnd + uint64(newClearance))
		// Lastly, save new clearance for comparison for next slot.
		lastClearanceAmount = newClearance

		// Go to next slot.
		finishedAtStart = finishedAtEnd
		announceNextSlot()

		// Check if we are stopping.
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}

	// We should never get here.
	// If we do, trigger a worker restart via the service worker.
	return errors.New("unexpected end of scheduler")
}
