package navigator

import (
	"context"

	"github.com/safing/portmaster/intel"
	"github.com/safing/portmaster/profile/endpoints"
)

type HubType uint8

const (
	HomeHub        HubType = iota
	TransitHub     HubType = iota
	DestinationHub HubType = iota
)

// Options holds configuration options for operations with the Map.
type Options struct {
	// Regard holds required States. Only Hubs where all of these are present
	// will taken into account for the operation. If NoDefaults is not set, a
	// basic set of desirable states is added automatically.
	Regard PinState

	// Disregard holds disqualifying States. Only Hubs where none of these are
	// present will be taken into account for the operation. If NoDefaults is not
	// set, a basic set of undesireable states is added automatically.
	Disregard PinState

	// HubPolicy is an endpoint list that all Hubs must pass in order to be taken into account for the operation.
	HubPolicy endpoints.Endpoints

	// HomeHubPolicy is an enpoint list that Home Hubs must pass in order to be taken into account for the operation.
	HomeHubPolicy endpoints.Endpoints

	// DestinationHubPolicy is an enpoint list that Destination Hubs must pass in order to be taken into account for the operation.
	DestinationHubPolicy endpoints.Endpoints

	// NoDefaults declares whether default and recommended Regard and Disregard states should not be used.
	NoDefaults bool

	// RequireTrustedDestinationHubs declares whether only Destination Hubs that have the Trusted state should be used.
	RequireTrustedDestinationHubs bool

	// RoutingProfile defines the algorithm to use to find a route.
	RoutingProfile string
}

type PinMatcher func(pin *Pin) bool

// DefaultOptions returns the default options for this Map.
func (m *Map) DefaultOptions() *Options {
	m.Lock()
	defer m.Unlock()

	return m.defaultOptions()
}

func (m *Map) defaultOptions() *Options {
	opts := &Options{
		RoutingProfile: RoutingProfileDefaultName,
	}

	if m.Intel != nil && m.Intel.Parsed() != nil {
		opts.HubPolicy = m.Intel.Parsed().HubAdvisory
		opts.HomeHubPolicy = m.Intel.Parsed().HomeHubAdvisory
		opts.DestinationHubPolicy = m.Intel.Parsed().DestinationHubAdvisory
	}

	return opts
}

func (o *Options) Matcher(hubType HubType) PinMatcher {
	// Compile states to regard and disregard.
	regard := o.Regard
	disregard := o.Disregard

	// Add default states.
	if !o.NoDefaults {
		// Add default States.
		regard |= StateSummaryRegard
		disregard |= StateSummaryDisregard

		// Add type based Advisories.
		switch hubType {
		case HomeHub:
			regard = regard.remove(StateReachable)
			disregard = disregard.add(StateUsageAsHomeDiscouraged)
		case DestinationHub:
			disregard = disregard.add(StateUsageAsDestinationDiscouraged)
		}
	}

	// Add Trusted requirement for Destination Hubs.
	if o.RequireTrustedDestinationHubs && hubType == DestinationHub {
		regard |= StateTrusted
	}

	// Copy and activate applicable policies.
	hubPolicy := o.HubPolicy
	var homeHubPolicy endpoints.Endpoints
	var destinationHubPolicy endpoints.Endpoints
	switch hubType {
	case HomeHub:
		homeHubPolicy = o.HomeHubPolicy
	case DestinationHub:
		destinationHubPolicy = o.DestinationHubPolicy
	}

	return func(pin *Pin) bool {
		// Check required Pin States.
		if !pin.State.has(regard) || pin.State.hasAnyOf(disregard) {
			return false
		}

		// Check main policy.
		if hubPolicy != nil {
			if endpointListMatch(hubPolicy, pin.EntityV4) == endpoints.Denied ||
				endpointListMatch(hubPolicy, pin.EntityV6) == endpoints.Denied {
				return false
			}
		}

		// Check type based policy.
		switch {
		case hubType == HomeHub && homeHubPolicy != nil:
			if endpointListMatch(homeHubPolicy, pin.EntityV4) == endpoints.Denied ||
				endpointListMatch(homeHubPolicy, pin.EntityV6) == endpoints.Denied {
				return false
			}
		case hubType == DestinationHub && destinationHubPolicy != nil:
			if endpointListMatch(destinationHubPolicy, pin.EntityV4) == endpoints.Denied ||
				endpointListMatch(destinationHubPolicy, pin.EntityV6) == endpoints.Denied {
				return false
			}
		}

		return true // All checks have passed.
	}
}

func endpointListMatch(list endpoints.Endpoints, entity *intel.Entity) endpoints.EPResult {
	result, _ := list.Match(context.TODO(), entity)
	return result
}
