package unit

import (
	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
)

// Unit describes a collection of data (containers) moving through the SPN
// network stack.
type Unit struct {
	id           uint64
	finished     abool.AtomicBool
	highPriority abool.AtomicBool
	paused       abool.AtomicBool

	Data *container.Container
}

// New returns a new unit holding a reference to the given container.
func New(c *container.Container) *Unit {
	return &Unit{
		id:   currentUnitID.Add(1),
		Data: c,
	}
}

// Start blocks until the unit may be processed.
func (u *Unit) Start() {
	// Unpause.
	u.unpause()

	// High priority units may always process.
	if u.highPriority.IsSet() {
		return
	}

	for {
		// Check if we are allowed to process in the current slot.
		if u.id <= clearanceUpTo.Load() {
			return
		}

		// Debug logging:
		// fmt.Printf("unit %d waiting for clearance at %d\n", u.id, clearanceUpTo.Load())

		// Wait for next slot.
		<-nextSlotSignal()
	}
}

// Finish signals the unit scheduler that this unit has finished processing.
func (u *Unit) Finish() {
	if u.finished.SetToIf(false, true) {
		finished.Add(1)
	}
	u.removeHighPriority()
	u.unpause()
}

// Pause signals the unit scheduler that this unit is paused and not being
// processed at the moment.
func (u *Unit) Pause() {
	if u.finished.IsNotSet() && u.paused.SetToIf(false, true) {
		pausedUnits.Add(1)
	}
}

// unpause signals the unit scheduler that this unit is not paused anymore and
// is now waiting for processing.
func (u *Unit) unpause() {
	if u.paused.SetToIf(true, false) {
		pausedUnits.Add(-1)
	}
}

// MakeHighPriority marks the unit as high priority.
func (u *Unit) MakeHighPriority() {
	if u.highPriority.SetToIf(false, true) {
		highPrioUnits.Add(1)
	}
}

// HighPriority returns whether the unit has high priority.
func (u *Unit) HighPriority() bool {
	return u.highPriority.IsSet()
}

// removeHighPriority removes the high priority mark.
func (u *Unit) removeHighPriority() {
	if u.highPriority.SetToIf(true, false) {
		highPrioUnits.Add(-1)
	}
}
