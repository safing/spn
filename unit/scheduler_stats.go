package unit

// SchedulerState holds a snapshot of the current scheduler state.
type SchedulerState struct {
	CurrentUnitID int64
	SlotPace      int64
	ClearanceUpTo int64
	Finished      int64

	Epoch                int32
	FinishedInEpoch      int64
	PausedUnitsInEpoch   int64
	HighPrioUnitsInEpoch int64
}

// State returns the current internal state values of the scheduler.
func (s *Scheduler) State() *SchedulerState {
	return &SchedulerState{
		CurrentUnitID: s.currentUnitID.Load(),
		SlotPace:      s.slotPace.Load(),
		ClearanceUpTo: s.clearanceUpTo.Load(),
		Finished:      s.finishedTotal.Load(),

		Epoch:                s.epoch.Load(),
		FinishedInEpoch:      s.finished.Load(),
		PausedUnitsInEpoch:   s.pausedUnits.Load(),
		HighPrioUnitsInEpoch: s.highPrioUnits.Load(),
	}
}

// GetCurrentUnitID returns the current unit ID.
func (s *Scheduler) GetCurrentUnitID() int64 {
	return s.currentUnitID.Load()
}

// GetSlotPace returns the current slot pace.
func (s *Scheduler) GetSlotPace() int64 {
	return s.slotPace.Load()
}

// GetClearanceUpTo returns the current clearance limit.
func (s *Scheduler) GetClearanceUpTo() int64 {
	return s.clearanceUpTo.Load()
}

// GetFinished returns the current amount of finished units.
func (s *Scheduler) GetFinished() int64 {
	return s.finishedTotal.Load()
}

// GetEpoch returns the current epoch ID.
func (s *Scheduler) GetEpoch() int32 {
	return s.epoch.Load()
}

// GetFinishedInEpoch returns the current finished units within the current epoch.
func (s *Scheduler) GetFinishedInEpoch() int64 {
	return s.finished.Load()
}

// GetPausedUnitsInEpoch returns the current paused units within the current epoch.
func (s *Scheduler) GetPausedUnitsInEpoch() int64 {
	return s.pausedUnits.Load()
}

// GetHighPrioUnitsInEpoch returns the current high priority units within the current epoch.
func (s *Scheduler) GetHighPrioUnitsInEpoch() int64 {
	return s.highPrioUnits.Load()
}
