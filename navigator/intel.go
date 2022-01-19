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
		m.updateInfoOverrides(pin)
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

func (m *Map) updateInfoOverrides(pin *Pin) {
	// Check if Intel data is loaded and if there are any overrides.
	if m.intel == nil || m.intel.InfoOverrides == nil {
		return
	}

	// Get overrides for this pin.
	overrides, ok := m.intel.InfoOverrides[pin.Hub.ID]
	if !ok {
		return
	}

	// Apply overrides
	if overrides.ContinentCode != "" {
		if pin.LocationV4 != nil {
			pin.LocationV4.Continent.Code = overrides.ContinentCode
		}
		if pin.LocationV6 != nil {
			pin.LocationV6.Continent.Code = overrides.ContinentCode
		}
	}
	if overrides.CountryCode != "" {
		if pin.LocationV4 != nil {
			pin.LocationV4.Country.ISOCode = overrides.CountryCode
			pin.EntityV4.Country = overrides.CountryCode
		}
		if pin.LocationV6 != nil {
			pin.LocationV6.Country.ISOCode = overrides.CountryCode
			pin.EntityV6.Country = overrides.CountryCode
		}
	}
	if overrides.Coordinates != nil {
		if pin.LocationV4 != nil {
			pin.LocationV4.Coordinates = *overrides.Coordinates
			pin.EntityV4.Coordinates = overrides.Coordinates
		}
		if pin.LocationV6 != nil {
			pin.LocationV6.Coordinates = *overrides.Coordinates
			pin.EntityV6.Coordinates = overrides.Coordinates
		}
	}
	if overrides.ASN != 0 {
		if pin.LocationV4 != nil {
			pin.LocationV4.AutonomousSystemNumber = overrides.ASN
			pin.EntityV4.ASN = overrides.ASN
		}
		if pin.LocationV6 != nil {
			pin.LocationV6.AutonomousSystemNumber = overrides.ASN
			pin.EntityV6.ASN = overrides.ASN
		}
	}
	if overrides.ASOrg != "" {
		if pin.LocationV4 != nil {
			pin.LocationV4.AutonomousSystemOrganization = overrides.ASOrg
			pin.EntityV4.ASOrg = overrides.ASOrg
		}
		if pin.LocationV6 != nil {
			pin.LocationV6.AutonomousSystemOrganization = overrides.ASOrg
			pin.EntityV6.ASOrg = overrides.ASOrg
		}
	}
}
