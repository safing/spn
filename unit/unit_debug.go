package unit

import (
	"sync"
	"time"

	"github.com/safing/portbase/log"
)

// UnitDebugger is used to debug unit leaks.
type UnitDebugger struct { //nolint:golint
	units     map[int64]*UnitDebugData
	unitsLock sync.Mutex
}

// UnitDebugData represents a unit that is being debugged.
type UnitDebugData struct { //nolint:golint
	unit       *Unit
	unitSource string
}

// DebugUnit registers the given unit for debug output with the given source.
// Additional calls on the same unit update the unit source.
// StartDebugLog() must be called before calling DebugUnit().
func (s *Scheduler) DebugUnit(u *Unit, unitSource string) {
	// Check if scheduler and unit debugger are created.
	if s == nil || s.unitDebugger == nil {
		return
	}

	s.unitDebugger.unitsLock.Lock()
	defer s.unitDebugger.unitsLock.Unlock()

	s.unitDebugger.units[u.id] = &UnitDebugData{
		unit:       u,
		unitSource: unitSource,
	}
}

// StartDebugLog logs the scheduler state every second.
func (s *Scheduler) StartDebugLog() {
	s.unitDebugger = &UnitDebugger{
		units: make(map[int64]*UnitDebugData),
	}

	go func() {
		for {
			s.debugStep()
			time.Sleep(time.Second)
		}
	}()
}

func (s *Scheduler) debugStep() {
	s.unitDebugger.unitsLock.Lock()
	defer s.unitDebugger.unitsLock.Unlock()

	// Go through debugging units and clear finished ones, count sources.
	sources := make(map[string]int)
	for id, debugUnit := range s.unitDebugger.units {
		if debugUnit.unit.finished.IsSet() {
			delete(s.unitDebugger.units, id)
		} else {
			cnt := sources[debugUnit.unitSource]
			sources[debugUnit.unitSource] = cnt + 1
		}
	}

	// Print current state.
	log.Debugf(
		`scheduler: state: slotPace=%d avgPace=%d maxPace=%d currentUnitID=%d clearanceUpTo=%d`,
		s.slotPace.Load(),
		s.avgPaceSum.Load()/s.avgPaceCnt.Load(),
		s.maxLeveledPace.Load(),
		s.currentUnitID.Load(),
		s.clearanceUpTo.Load(),
	)
	log.Debugf("scheduler: unit sources: %+v", sources)
}
