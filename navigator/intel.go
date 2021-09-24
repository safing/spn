package navigator

import (
	"context"
	"errors"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/profile/endpoints"
	"github.com/safing/spn/hub"
)

// UpdateIntel supplies the map with new intel data. The data is not copied, so
// it must not be modified after being supplied. If the map is empty, the
// bootstrap hubs will be added to the map.
func (m *Map) UpdateIntel(update *hub.Intel) error {
	// Check if intel data is already parsed.
	if update.Parsed() == nil {
		return errors.New("intel data is not parsed")
	}

	m.Lock()
	defer m.Unlock()

	// Update the map's reference to the intel data.
	m.intel = update

	// go through map
	for _, pin := range m.all {
		m.updateIntelStatuses(pin)
	}

	log.Infof("spn/navigator: updated intel on map %s", m.Name)

	// Add bootstrap hubs if map is empty.
	if m.isEmpty() {
		return m.addBootstrapHubs(m.intel.BootstrapHubs)
	}
	return nil
}

func (m *Map) updateIntelStatuses(pin *Pin) {
	// Reset all related states.
	pin.removeStates(StateTrusted | StateUsageDiscouraged | StateUsageAsHomeDiscouraged | StateUsageAsDestinationDiscouraged)

	// Check if Intel data is loaded.
	if m.intel == nil {
		return
	}

	// Check if Hub is trusted.
	for _, hubID := range m.intel.TrustedHubs {
		if pin.Hub.ID == hubID {
			pin.addStates(StateTrusted)
			break
		}
	}

	// Check advisories.
	// Check for UsageDiscouraged.
	checkStatusList(
		pin,
		StateUsageDiscouraged,
		m.intel.AdviseOnlyTrustedHubs,
		m.intel.Parsed().HubAdvisory,
	)
	// Check for UsageAsHomeDiscouraged.
	checkStatusList(
		pin,
		StateUsageAsHomeDiscouraged,
		m.intel.AdviseOnlyTrustedHomeHubs,
		m.intel.Parsed().HomeHubAdvisory,
	)
	// Check for UsageAsDestinationDiscouraged.
	checkStatusList(
		pin,
		StateUsageAsDestinationDiscouraged,
		m.intel.AdviseOnlyTrustedDestinationHubs,
		m.intel.Parsed().DestinationHubAdvisory,
	)
}

func checkStatusList(pin *Pin, state PinState, requireTrusted bool, endpointList endpoints.Endpoints) {
	if requireTrusted && !pin.State.has(StateTrusted) {
		pin.addStates(state)
		return
	}

	result, _ := endpointList.Match(context.TODO(), pin.EntityV4)
	if result == endpoints.Denied {
		pin.addStates(state)
		return
	}

	result, _ = endpointList.Match(context.TODO(), pin.EntityV6)
	if result == endpoints.Denied {
		pin.addStates(state)
	}
}
