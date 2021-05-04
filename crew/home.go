package crew

import (
	"fmt"
	"net"
)

// SetHome sets the h
func (m *Map) EstablishHome(deviceIP net.IP) error {
	m.Lock()
	defer m.Unlock()

	nbPins, err := m.findNearestPins(deviceIP, m.DefaultOptions().Matcher(HomeHub), 10)
	if err != nil {
		return fmt.Errorf("failed to find nearby Hubs: %w", err)
	}

	for _, nbPin := range nbPins.pins {
		// TODO: set sail
		// Set Home Hub.
		// m.Home = home
	}

	return m.recalculateReachableHubs()
}
