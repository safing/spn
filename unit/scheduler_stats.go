package unit

import (
	"time"

	"github.com/safing/portbase/log"
)

// SchedulerState holds a snapshot of the current scheduler state.
type SchedulerState struct {
	currentUnitID int64
	finished      int64
	clearanceUpTo int64
	slotPace      int64
	pausedUnits   int64
	highPrioUnits int64
}

// State returns the current internal state values of the scheduler.
func (s *Scheduler) State() *SchedulerState {
	return &SchedulerState{
		currentUnitID: s.currentUnitID.Load(),
		finished:      s.finished.Load(),
		clearanceUpTo: s.clearanceUpTo.Load(),
		slotPace:      s.slotPace.Load(),
		pausedUnits:   s.pausedUnits.Load(),
		highPrioUnits: s.highPrioUnits.Load(),
	}
}

// StartDebugLog logs the scheduler state every second.
func (s *Scheduler) StartDebugLog() {
	go func() {
		for {
			log.Debugf("scheduler state: %+v", s.State())
			time.Sleep(time.Second)
		}
	}()
}
