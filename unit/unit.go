package unit

import (
	"github.com/tevino/abool"
)

// Unit describes a "work unit" and is meant to be embedded into another struct
// used for passing data moving through multiple processing steps.
type Unit struct {
	id           uint64
	scheduler    *Scheduler
	finished     abool.AtomicBool
	highPriority abool.AtomicBool
	paused       abool.AtomicBool
}

// NewUnit returns a new unit within the scheduler.
func (s *Scheduler) NewUnit() *Unit {
	return &Unit{
		id:        s.currentUnitID.Add(1),
		scheduler: s,
	}
}

// WaitForUnitSlot blocks until the unit may be processed.
func (u *Unit) WaitForUnitSlot() {
	// Unpause.
	u.unpause()

	// High priority units may always process.
	if u.highPriority.IsSet() {
		return
	}

	for {
		// Check if we are allowed to process in the current slot.
		if u.id <= u.scheduler.clearanceUpTo.Load() {
			return
		}

		// Debug logging:
		// fmt.Printf("unit %d waiting for clearance at %d\n", u.id, u.scheduler.clearanceUpTo.Load())

		// Wait for next slot.
		<-u.scheduler.nextSlotSignal()
	}
}

// FinishUnit signals the unit scheduler that this unit has finished processing.
// Will no-op if called on a nil Unit.
func (u *Unit) FinishUnit() {
	if u == nil {
		return
	}
	if u.finished.SetToIf(false, true) {
		u.scheduler.finished.Add(1)
	}
	u.removeHighPriority()
	u.unpause()
}

// PauseUnit signals the unit scheduler that this unit is paused and not being
// processed at the moment.
func (u *Unit) PauseUnit() {
	if u.finished.IsNotSet() && u.paused.SetToIf(false, true) {
		u.scheduler.pausedUnits.Add(1)
	}
}

// unpause signals the unit scheduler that this unit is not paused anymore and
// is now waiting for processing.
func (u *Unit) unpause() {
	if u.paused.SetToIf(true, false) {
		u.scheduler.pausedUnits.Add(-1)
	}
}

// MakeUnitHighPriority marks the unit as high priority.
func (u *Unit) MakeUnitHighPriority() {
	if u.highPriority.SetToIf(false, true) {
		u.scheduler.highPrioUnits.Add(1)
	}
}

// IsHighPriorityUnit returns whether the unit has high priority.
func (u *Unit) IsHighPriorityUnit() bool {
	return u.highPriority.IsSet()
}

// removeHighPriority removes the high priority mark.
func (u *Unit) removeHighPriority() {
	if u.highPriority.SetToIf(true, false) {
		u.scheduler.highPrioUnits.Add(-1)
	}
}
