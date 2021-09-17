package navigator

import (
	"context"

	"github.com/safing/portmaster/profile/endpoints"
	"github.com/safing/spn/hub"
)

func (m *Map) UpdateIntel(update *hub.Intel) {
	m.Lock()
	defer m.Unlock()

	// update reference
	m.intel = update

	// go through map
	for _, pin := range m.all {
		m.updateIntelStatuses(pin)
	}
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
