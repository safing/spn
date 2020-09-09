package navigator

import (
	"fmt"
	"strings"
)

type MapStats struct {
	Name   string
	States map[PinState]int
	Lanes  map[int]int
}

// Stats collects and returns statistics from the map.
func (m *Map) Stats() *MapStats {
	m.Lock()
	defer m.Unlock()

	// Create stats struct.
	stats := &MapStats{
		Name:   m.Name,
		States: make(map[PinState]int),
		Lanes:  make(map[int]int),
	}
	for _, state := range allStates {
		stats.States[state] = 0
	}

	// Iterate over all Pins to collect data.
	for _, pin := range m.All {
		// Check all states.
		for _, state := range allStates {
			if pin.State.hasAllOf(state) {
				stats.States[state] += 1
			}
		}

		// Count lanes.
		laneCnt, ok := stats.Lanes[len(pin.ConnectedTo)]
		if ok {
			stats.Lanes[len(pin.ConnectedTo)] = laneCnt + 1
		} else {
			stats.Lanes[len(pin.ConnectedTo)] = 1
		}
	}

	return stats
}

func (ms *MapStats) String() string {
	var builder strings.Builder

	// Write header.
	fmt.Fprintf(&builder, "Stats for Map %s:\n", ms.Name)

	// Write State Stats
	for state, cnt := range ms.States {
		fmt.Fprintf(&builder, "State %s: %d\n", state, cnt)
	}

	// Write Lane Stats
	for laneCnt, pinCnt := range ms.Lanes {
		fmt.Fprintf(&builder, "%d Lanes: %d\n", laneCnt, pinCnt)
	}

	return builder.String()
}
