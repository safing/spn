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
	defaultSlotDuration = 10 * time.Millisecond
	defaultMinSlotPace  = 1000

	defaultAdjustFractionPerStreak        = 1000 // 0.1%
	defaultHighPriorityMaxReserveFraction = 2    // 50%
)

// Scheduler creates and schedules units.
// Must be created using NewScheduler().
type Scheduler struct {
	// Configuration.
	config SchedulerConfig

	// Units IDs Limit / Thresholds.

	// currentUnitID holds the last assigned Unit ID.
	currentUnitID atomic.Int64
	// finished holds the amount of units that were finished.
	// Not necessarily all Unit IDs below this value are actually finished.
	finished atomic.Int64
	// clearanceUpTo holds the current threshold up to which Unit ID Units may be processed.
	clearanceUpTo atomic.Int64

	// Pace and amount of unit states.

	// slotPace holds the current pace. This is the base value for clearance
	// calcuation, not the value of the current cleared Units itself.
	slotPace atomic.Int64
	// pausedUnits holds the amount of unfinished Units which are currently marked as "paused".
	// These Units are waiting for an external condition.
	pausedUnits atomic.Int64
	// highPrioUnits holds the amount of unfinished Units which were marked as high priority.
	highPrioUnits atomic.Int64

	// Slot management.
	slotSignalA      chan struct{}
	slotSignalB      chan struct{}
	slotSignalSwitch bool
	slotSignalsLock  sync.RWMutex

	stopping abool.AtomicBool

	unitDebugger *UnitDebugger
}

// SchedulerConfig holds scheduler configuration.
type SchedulerConfig struct {
	// SlotDuration defines the duration of one slot.
	SlotDuration time.Duration

	// MinSlotPace defines the minimum slot pace.
	// The slot pace will never fall below this value.
	MinSlotPace int64

	// AdjustFractionPerStreak defines the fraction of the pace the pace itself changes in either direction to match the current use and load.
	AdjustFractionPerStreak int64

	// HighPriorityMaxReserveFraction defines the fraction of the pace that may - at maximum - be reserved for high priority units.
	HighPriorityMaxReserveFraction int64
}

// NewScheduler returns a new scheduler.
func NewScheduler(config *SchedulerConfig) *Scheduler {
	// Fallback to empty config if none is given.
	if config == nil {
		config = &SchedulerConfig{}
	}

	// Create new scheduler.
	s := &Scheduler{
		config:      *config,
		slotSignalA: make(chan struct{}),
		slotSignalB: make(chan struct{}),
	}

	// Fill in defaults.
	if s.config.SlotDuration == 0 {
		s.config.SlotDuration = defaultSlotDuration
	}
	if s.config.MinSlotPace == 0 {
		s.config.MinSlotPace = defaultMinSlotPace
	}
	if s.config.AdjustFractionPerStreak == 0 {
		s.config.AdjustFractionPerStreak = defaultAdjustFractionPerStreak
	}
	if s.config.HighPriorityMaxReserveFraction == 0 {
		s.config.HighPriorityMaxReserveFraction = defaultHighPriorityMaxReserveFraction
	}

	// Initialize scheduler fields.
	s.clearanceUpTo.Store(s.config.MinSlotPace)
	s.slotPace.Store(s.config.MinSlotPace)

	return s
}

func (s *Scheduler) nextSlotSignal() chan struct{} {
	s.slotSignalsLock.RLock()
	defer s.slotSignalsLock.RUnlock()

	if s.slotSignalSwitch {
		return s.slotSignalA
	}
	return s.slotSignalB
}

func (s *Scheduler) announceNextSlot() {
	s.slotSignalsLock.Lock()
	defer s.slotSignalsLock.Unlock()

	// Close new slot signal and refresh previous one.
	if s.slotSignalSwitch {
		close(s.slotSignalA)
		s.slotSignalB = make(chan struct{})
	} else {
		close(s.slotSignalB)
		s.slotSignalA = make(chan struct{})
	}

	// Switch to next slot.
	s.slotSignalSwitch = !s.slotSignalSwitch
}

// SlotScheduler manages the slot and schedules units.
// Must only be started once.
func (s *Scheduler) SlotScheduler(ctx context.Context) error {
	// Start slot ticker.
	ticker := time.NewTicker(s.config.SlotDuration)
	defer ticker.Stop()

	// Give clearance to all when stopping.
	defer s.clearanceUpTo.Store(math.MaxInt64 - math.MaxInt32)

	var (
		lastClearanceAmount int64
		finishedAtStart     = s.finished.Load()
		increaseStreak      int64
		decreaseStreak      int64
	)
	for range ticker.C {
		// Calculate how many units were finished in slot.
		// Only load "finished" once, so we don't miss anything.
		finishedAtEnd := s.finished.Load()
		finishedInSlot := finishedAtEnd - finishedAtStart

		// Adapt pace.
		if finishedInSlot >= lastClearanceAmount {
			// Adjust based on streak.
			increaseStreak++
			decreaseStreak = 0
			s.slotPace.Add((s.slotPace.Load() / s.config.AdjustFractionPerStreak) * increaseStreak)

			// Debug logging:
			// fmt.Printf("+++ slot pace: %d (finished in slot: %d, last clearance: %d, increaseStreak: %d)\n", s.slotPace.Load(), finishedInSlot, lastClearanceAmount, increaseStreak)
		} else {
			// Adjust based on streak.
			decreaseStreak++
			increaseStreak = 0
			s.slotPace.Add(-((s.slotPace.Load() / s.config.AdjustFractionPerStreak) * decreaseStreak))

			// Enforce minimum.
			if s.slotPace.Load() < s.config.MinSlotPace {
				s.slotPace.Store(s.config.MinSlotPace)
				decreaseStreak = 0
			}

			// Debug logging:
			// fmt.Printf("--- slot pace: %d (finished in slot: %d, last clearance: %d, decreaseStreak: %d)\n", s.slotPace.Load(), finishedInSlot, lastClearanceAmount, decreaseStreak)
		}

		// Set new slot clearance.
		// First, add current pace and paused units.
		newClearance := s.slotPace.Load() + s.pausedUnits.Load()
		// Second, subtract up to 20% of clearance for high priority units.
		highPrio := s.highPrioUnits.Load()
		if highPrio > newClearance/5 {
			newClearance -= newClearance / 5
		} else {
			newClearance -= highPrio
		}
		// Third, add finished to set new clearance limit.
		s.clearanceUpTo.Store(finishedAtEnd + newClearance)
		// Lastly, save new clearance for comparison for next slot.
		lastClearanceAmount = newClearance

		// Go to next slot.
		finishedAtStart = finishedAtEnd
		s.announceNextSlot()

		// Check if we are stopping.
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if s.stopping.IsSet() {
			return nil
		}
	}

	// We should never get here.
	// If we do, trigger a worker restart via the service worker.
	return errors.New("unexpected end of scheduler")
}

// Stop stops the scheduler and gives clearance to all units.
func (s *Scheduler) Stop() {
	s.stopping.Set()
}
