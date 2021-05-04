package navigator

import (
	"sync"
	"testing"

	"github.com/brianvoe/gofakeit"

	"github.com/safing/spn/hub"
)

var (
	optimizedDefaultMapCreate sync.Once
	optimizedDefaultMap       *Map
)

func getOptimizedDefaultTestMap(t *testing.T) *Map {
	optimizedDefaultMapCreate.Do(func() {
		optimizedDefaultMap = createRandomTestMap(2, 100)
		optimizedDefaultMap.optimizeTestMap(t)
	})
	return optimizedDefaultMap
}

func (m *Map) optimizeTestMap(t *testing.T) {
	if t != nil {
		t.Logf("optimizing test map %s with %d pins", m.Name, len(m.All))
	}

	// Save original Home, as we will be switching around the home for the
	// optimization.
	run := 0
	newLanes := 0
	newLanesInRun := 0
	lastRun := false
	originalHome := m.Home

	for {
		run++
		newLanesInRun = 0
		// Let's check if we have a run without any map changes.
		lastRun = true

		for _, pin := range m.All {

			// Set Home to this Pin for this iteration.
			m.Home = pin
			err := m.recalculateReachableHubs()
			if err != nil {
				panic(err)
			}

			connectTo, err := m.optimize(m.defaultOptions())
			if err != nil {
				panic(err)
			}
			if connectTo != nil {
				// Add lanes to the Hub status.
				m.Home.Hub.AddLane(&hub.Lane{
					ID:       connectTo.Hub.ID,
					Capacity: gofakeit.Number(10, 100),
					Latency:  gofakeit.Number(10, 100),
				})
				connectTo.Hub.AddLane(&hub.Lane{
					ID:       m.Home.Hub.ID,
					Capacity: gofakeit.Number(10, 100),
					Latency:  gofakeit.Number(10, 100),
				})
				// Update Hubs in map.
				m.updateHub(m.Home.Hub)
				m.updateHub(connectTo.Hub)
				newLanes++
				newLanesInRun++

				// We are changing the map in this run, so this is not the last.
				lastRun = false
			}
		}

		// Log progress.
		if t != nil {
			t.Logf(
				"optimizing: added %d lanes in run #%d (%d Hubs) - %d new lanes in total",
				newLanesInRun,
				run,
				len(m.All),
				newLanes,
			)
		}

		// End optimization after last run.
		if lastRun {
			break
		}
	}

	// Log what was done and set home back to the original value.
	if t != nil {
		t.Logf("finished optimizing test map %s: added %d lanes in %d runs", m.Name, newLanes, run)
	}
	m.Home = originalHome
}

func TestOptimize(t *testing.T) {
	m := getOptimizedDefaultTestMap(t)
	matcher := m.defaultOptions().Matcher(DestinationHub)
	originalHome := m.Home

	for _, pin := range m.All {
		// Set Home to this Pin for this iteration.
		m.Home = pin
		err := m.recalculateReachableHubs()
		if err != nil {
			panic(err)
		}

		for _, peer := range m.All {
			// Check if the Pin matches the criteria.
			if !matcher(peer) {
				continue
			}

			if peer.HopDistance > optimizationHopDistanceTarget {
				t.Errorf("Optimization error: %s is %d hops away from %s", peer, peer.HopDistance, pin)
			}
		}
	}

	// Print stats
	t.Logf("optimized map:\n%s\n", m.Stats())

	m.Home = originalHome
}
