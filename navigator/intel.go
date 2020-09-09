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
	m.Intel = update

	// go through map
	for _, pin := range m.All {
		m.updateIntelStatuses(pin)
	}
}

func (m *Map) updateIntelStatuses(pin *Pin) {
	// Reset all related states.
	pin.unsetStates(StateTrusted | StateUsageDiscouraged | StateUsageAsHomeDiscouraged | StateUsageAsDestinationDiscouraged)

	// Check if Intel data is loaded.
	if m.Intel == nil {
		return
	}

	// Check if Hub is trusted.
	for _, hubID := range m.Intel.TrustedHubs {
		if pin.Hub.ID == hubID {
			pin.setStates(StateTrusted)
			break
		}
	}

	// Check advisories.
	// Check for UsageDiscouraged.
	checkStatusList(
		pin,
		StateUsageDiscouraged,
		m.Intel.AdviseOnlyTrustedHubs,
		m.Intel.Parsed().HubAdvisory,
	)
	// Check for UsageAsHomeDiscouraged.
	checkStatusList(
		pin,
		StateUsageAsHomeDiscouraged,
		m.Intel.AdviseOnlyTrustedHomeHubs,
		m.Intel.Parsed().HomeHubAdvisory,
	)
	// Check for UsageAsDestinationDiscouraged.
	checkStatusList(
		pin,
		StateUsageAsDestinationDiscouraged,
		m.Intel.AdviseOnlyTrustedDestinationHubs,
		m.Intel.Parsed().DestinationHubAdvisory,
	)
}

func checkStatusList(pin *Pin, state PinState, requireTrusted bool, endpointList endpoints.Endpoints) {
	if requireTrusted && !pin.State.hasAllOf(StateTrusted) {
		pin.setStates(state)
		return
	}

	result, _ := endpointList.Match(context.TODO(), pin.EntityV4)
	if result == endpoints.Denied {
		pin.setStates(state)
		return
	}

	result, _ = endpointList.Match(context.TODO(), pin.EntityV6)
	if result == endpoints.Denied {
		pin.setStates(state)
	}
}
