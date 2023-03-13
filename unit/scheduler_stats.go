package unit

// GetAvgSlotPace returns the current average slot pace.
func (s *Scheduler) GetAvgSlotPace() int64 {
	// This is somewhat racy, as one value might already be updated with the
	// latest slot data, while the other has been not.
	// This is not so much of a problem, as slots are really short and the impact
	// is very low.
	cnt := s.avgPaceCnt.Load()
	sum := s.avgPaceSum.Load()

	return sum / cnt
}

// ResetAvgSlotPace reset average slot pace values.
func (s *Scheduler) ResetAvgSlotPace() {
	// This is somewhat racy, as one value might already be updated with the
	// latest slot data, while the other has been not.
	// This is not so much of a problem, as slots are really short and the impact
	// is very low.
	s.avgPaceCnt.Store(0)
	s.avgPaceSum.Store(0)
}

// GetMaxLeveledSlotPace returns the current maximum leveled slot pace.
func (s *Scheduler) GetMaxLeveledSlotPace() int64 {
	return s.maxLeveledPace.Load()
}

// ResetMaxLeveledSlotPace resets the maximum leveled slot pace value.
func (s *Scheduler) ResetMaxLeveledSlotPace() {
	s.maxLeveledPace.Store(0)
}
