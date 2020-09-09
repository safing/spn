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
		optimizedDefaultMap = createRandomTestMap(1, 1000)
		optimizedDefaultMap.optimizeTestMap(t)
	})
	return optimizedDefaultMap
}

func (m *Map) optimizeTestMap(t *testing.T) {
	t.Logf("optimizing test map %s with %d pins", m.Name, len(m.All))

	// Save original Home, as we will be switching around the home for the
	// optimization.
	progress := 0
	newLanes := 0
	originalHome := m.Home

	for _, pin := range m.All {
		// Set Home to this Pin for this iteration.
		m.Home = pin
		err := m.recalculateReachableHubs()
		if err != nil {
			panic(err)
		}

		for {
			connectTo, err := m.optimize(m.defaultOptions())
			if err != nil {
				panic(err)
			}
			if connectTo == nil {
				break
			}

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

			if newLanes%100 == 0 {
				t.Logf(
					"optimizing progress %d/%d (lanes created: %d)",
					progress,
					len(m.All),
					newLanes,
				)
			}

			// Set other Hub as new Home for next test net optimization.
			m.Home = connectTo
			err = m.recalculateReachableHubs()
			if err != nil {
				panic(err)
			}
		}

		// Log progress.
		progress++
		if progress%10 == 0 || progress == len(m.All) {
			t.Logf(
				"optimizing progress %d/%d (lanes created: %d)",
				progress,
				len(m.All),
				newLanes,
			)
		}
	}

	// Log what was done and set home back to the original value.
	t.Logf("finishid optimizing test map %s: added %d lanes", m.Name, newLanes)
	m.Home = originalHome
}

func TestOptimize(t *testing.T) {
	m := getOptimizedDefaultTestMap(t)
	originalHome := m.Home

	for _, pin := range m.All {
		// Set Home to this Pin for this iteration.
		m.Home = pin
		err := m.recalculateReachableHubs()
		if err != nil {
			panic(err)
		}

		for _, peer := range m.All {
			if peer.HopDistance > 3 {
				t.Errorf("%s is %d hops away from %s", peer, peer.HopDistance, pin)
			}
		}
	}

	// Print stats
	t.Logf("optimized map:\n%s\n", m.Stats())

	m.Home = originalHome
}
