package navigator

import (
	"sort"
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
)

var (
	Main = NewMap("main")
)

// Map represent a collection of Pins and their relationship and status.
type Map struct {
	sync.RWMutex
	Name string

	all   map[string]*Pin
	intel *hub.Intel

	home         *Pin
	homeTerminal *docks.CraneTerminal
}

// NewMap returns a new and empty Map.
func NewMap(name string) *Map {
	m := &Map{
		Name: name,
		all:  make(map[string]*Pin),
	}
	addMapToAPI(m)
	return m
}

func (m *Map) Close() {
	removeMapFromAPI(m.Name)
}

// GetHome returns the current home and it's accompanying terminal.
// Both may be nil.
func (m *Map) GetHome() (*Pin, *docks.CraneTerminal) {
	m.RLock()
	defer m.RUnlock()

	return m.home, m.homeTerminal
}

// SetHome sets the given hub as the new home. Optionally, a terminal may be
// supplied to accompany the home hub.
func (m *Map) SetHome(id string, t *docks.CraneTerminal) (ok bool) {
	m.Lock()
	defer m.Unlock()

	// Get pin from map.
	newHome, ok := m.all[id]
	if !ok {
		return false
	}

	// Remove home hub state from all pins.
	for _, pin := range m.all {
		pin.removeStates(StateIsHomeHub)
	}

	// Set pin as home.
	m.home = newHome
	m.homeTerminal = t
	m.home.addStates(StateIsHomeHub)

	// Recalculate reachable.
	err := m.recalculateReachableHubs()
	if err != nil {
		log.Warningf("spn/navigator: failed to recalculate reachable hubs: %s", err)
	}

	return true
}

// isEmpty returns whether the Map is regarded as empty.
func (m *Map) isEmpty() bool {
	if m.home != nil {
		// When a home hub is set, we also regard a map with only one entry to be
		// empty, as this will be the case for Hubs, which will have their own
		// entry in the Map.
		return len(m.all) <= 1
	}

	return len(m.all) == 0
}

func (m *Map) sortedPins() []*Pin {
	// Copy into slice.
	sorted := make([]*Pin, 0, len(m.all))
	for _, pin := range m.all {
		sorted = append(sorted, pin)
	}
	// Sort slice.
	sort.Sort(sortByPinID(sorted))
	return sorted
}

type sortByPinID []*Pin

func (a sortByPinID) Len() int           { return len(a) }
func (a sortByPinID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortByPinID) Less(i, j int) bool { return a[i].Hub.ID < a[j].Hub.ID }
