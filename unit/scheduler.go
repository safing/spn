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
	defaultSlotDuration  = 10 * time.Millisecond
	defaultMinSlotPace   = 100 // 10 000 pps
	defaultEpochDuration = 1 * time.Minute

	defaultAdjustFractionPerStreak        = 100 // 1%
	defaultHighPriorityMaxReserveFraction = 4   //  25%
)

// Scheduler creates and schedules units.
// Must be created using NewScheduler().
type Scheduler struct {
	// Configuration.
	config SchedulerConfig

	// Units IDs Limit / Thresholds.

	// currentUnitID holds the last assigned Unit ID.
	currentUnitID atomic.Int64
	// slotPace holds the current pace. This is the base value for clearance
	// calcuation, not the value of the current cleared Units itself.
	slotPace atomic.Int64
	// clearanceUpTo holds the current threshold up to which Unit ID Units may be processed.
	clearanceUpTo atomic.Int64
	// finishedTotal holds the amount of units that were finished across all epochs.
	finishedTotal atomic.Int64

	// Epoch amounts.

	// epoch is the slot epoch counter for resetting special values.
	epoch atomic.Int32
	// finished holds the amount of units that were finished within the current epoch.
	// Not necessarily all Unit IDs below this value are actually finished.
	finished atomic.Int64
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

	// EpochDuration defines the duration of one epoch.
	// Set to 0 to disable epochs.
	EpochDuration time.Duration

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
	if s.config.EpochDuration == 0 {
		s.config.EpochDuration = defaultEpochDuration
	}
	if s.config.AdjustFractionPerStreak == 0 {
		s.config.AdjustFractionPerStreak = defaultAdjustFractionPerStreak
	}
	if s.config.HighPriorityMaxReserveFraction == 0 {
		s.config.HighPriorityMaxReserveFraction = defaultHighPriorityMaxReserveFraction
	}

	// The adjust fraction may not be bigger than the min slot pace.
	if s.config.AdjustFractionPerStreak > s.config.MinSlotPace {
		s.config.AdjustFractionPerStreak = s.config.MinSlotPace

		// Debug logging:
		// fmt.Printf("--- reduced AdjustFractionPerStreak to %d\n", s.config.AdjustFractionPerStreak)
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

	// Calculate how many slots per epoch
	var slotCnt int64
	slotsPerEpoch := s.config.EpochDuration / s.config.SlotDuration

	// Give clearance to all when stopping.
	defer s.clearanceUpTo.Store(math.MaxInt64 - math.MaxInt32)

	var (
		lastClearanceAmount int64
		finishedAtStart     int64
		increaseStreak      int64
		decreaseStreak      int64
		epochBase           int64
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
			// fmt.Printf("+++ slot pace: %d (finished in slot: %d, last clearance: %d, increaseStreak: %d, high: %d)\n", s.slotPace.Load(), finishedInSlot, lastClearanceAmount, increaseStreak, s.highPrioUnits.Load())
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
			// fmt.Printf("--- slot pace: %d (finished in slot: %d, last clearance: %d, decreaseStreak: %d, high: %d)\n", s.slotPace.Load(), finishedInSlot, lastClearanceAmount, decreaseStreak, s.highPrioUnits.Load())
		}

		// Advance epoch if needed.
		slotCnt++
		if slotCnt%int64(slotsPerEpoch) == 0 {
			slotCnt = 0

			// Switch to new epoch.
			s.epoch.Add(1)

			// Add the finished amount of the current epoch to the total counter.
			s.finishedTotal.Add(finishedAtEnd)

			// Only reduce by amount we have seen, for correct metrics.
			s.finished.Add(-finishedAtEnd)
			finishedAtEnd = 0

			// Raise the epoch base to the current unit ID.
			epochBase = s.currentUnitID.Load()

			// Reset counters.
			s.highPrioUnits.Store(0)
			s.pausedUnits.Store(0)

			// Debug logging:
			// fmt.Printf("--- new epoch\n")
		}

		// Set new slot clearance.
		// First, add current pace and paused units.
		newClearance := s.slotPace.Load() + s.pausedUnits.Load()
		// Second, subtract a fraction of the clearance for high priority units.
		highPrio := s.highPrioUnits.Load()
		if highPrio > newClearance/s.config.HighPriorityMaxReserveFraction {
			newClearance -= newClearance / s.config.HighPriorityMaxReserveFraction
		} else {
			newClearance -= highPrio
		}
		// Third, add finished to set new clearance limit.
		s.clearanceUpTo.Store(epochBase + finishedAtEnd + newClearance)
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
