package navigator

import (
	"context"

	"github.com/safing/portmaster/intel"
	"github.com/safing/portmaster/profile/endpoints"
)

type HubType uint8

const (
	HomeHub HubType = iota
	TransitHub
	DestinationHub
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

	// HomeHubPolicy is an endpoint list that Home Hubs must pass in order to be taken into account for the operation.
	HomeHubPolicy endpoints.Endpoints

	// DestinationHubPolicy is an endpoint list that Destination Hubs must pass in order to be taken into account for the operation.
	DestinationHubPolicy endpoints.Endpoints

	// FIXME
	CheckHubEntryPolicy bool

	// FIXME
	CheckHubExitPolicy bool

	// NoDefaults declares whether default and recommended Regard and Disregard states should not be used.
	NoDefaults bool

	// RequireTrustedDestinationHubs declares whether only Destination Hubs that have the Trusted state should be used.
	RequireTrustedDestinationHubs bool

	// RoutingProfile defines the algorithm to use to find a route.
	RoutingProfile string
}

func (o *Options) Copy() *Options {
	return &Options{
		Regard:                        o.Regard,
		Disregard:                     o.Disregard,
		HubPolicy:                     o.HubPolicy,
		HomeHubPolicy:                 o.HomeHubPolicy,
		DestinationHubPolicy:          o.DestinationHubPolicy,
		CheckHubEntryPolicy:           o.CheckHubEntryPolicy,
		CheckHubExitPolicy:            o.CheckHubExitPolicy,
		NoDefaults:                    o.NoDefaults,
		RequireTrustedDestinationHubs: o.RequireTrustedDestinationHubs,
		RoutingProfile:                o.RoutingProfile,
	}
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

	if m.intel != nil && m.intel.Parsed() != nil {
		opts.HubPolicy = m.intel.Parsed().HubAdvisory
		opts.HomeHubPolicy = m.intel.Parsed().HomeHubAdvisory
		opts.DestinationHubPolicy = m.intel.Parsed().DestinationHubAdvisory
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
		regard = regard.add(StateSummaryRegard)
		disregard = disregard.add(StateSummaryDisregard)

		// Add type based Advisories.
		switch hubType {
		case HomeHub:
			// Home Hubs don't need to be reachable and don't need keys ready to be used.
			regard = regard.remove(StateReachable)
			regard = regard.remove(StateActive)
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
