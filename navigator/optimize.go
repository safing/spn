package navigator

import (
	"errors"
	"fmt"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
)

const (
	optimizationHopDistanceTarget = 3
)

// FindNearestHubs searches for the nearest Hubs to the given IP address. The returned Hubs must not be modified in any way.
func (m *Map) Optimize(opts *Options) (connectTo *hub.Hub, err error) {
	m.RLock()
	defer m.RUnlock()

	// Check if the map is empty.
	if m.isEmpty() {
		return nil, ErrEmptyMap
	}

	// Set default options if unset.
	if opts == nil {
		opts = m.defaultOptions()
	}

	pin, err := m.optimize(opts)
	switch {
	case err != nil:
		return nil, err
	case pin == nil:
		return nil, nil
	default:
		return pin.Hub, nil
	}
}

func (m *Map) optimize(opts *Options) (connectTo *Pin, err error) {
	if m.home == nil {
		return nil, ErrHomeHubUnset
	}

	// Create default matcher.
	matcher := opts.Matcher(TransitHub)

	// Define loop variables.
	var (
		mostDistantPin     *Pin
		highestHopDistance = optimizationHopDistanceTarget
	)

	// Iterate over all Pins to find the most distant Pin.
	var matchedAny bool
	for _, pin := range m.all {
		// Check if the Pin matches the criteria.
		if !matcher(pin) {
			// Debugging:
			// log.Tracef("spn/navigator: skipping %s with states %s for optimizing", pin, pin.State)
			continue
		}
		matchedAny = true

		if pin.HopDistance > highestHopDistance {
			highestHopDistance = pin.HopDistance
			mostDistantPin = pin
		}
	}
	// Return the most distant pin, if set.
	if mostDistantPin != nil {
		return mostDistantPin, nil
	}
	// If anything matched, we seem to be connected to the network.
	if matchedAny {
		// We are connected, but the network may be segregated.
		return m.optimizeDesegregate(opts)
	}

	// If no pin matched at all, we don't seem connected to anyone.
	// Find the nearest non-connected pin, to bootstrap to the network.

	pins, err := m.findNearestPins(m.home.LocationV4, m.home.LocationV6, opts.Matcher(HomeHub), 10)
	if err != nil {
		return nil, fmt.Errorf("failed to find nearest hubs for bootstrapping: %w", err)
	}
	switch pins.Len() {
	case 0:
		return nil, errors.New("failed to find nearest hubs for bootstrapping: no hubs nearby")
	case 1:
		return pins.pins[0].pin, nil
	default:
		// Return a pseudo random pin.
		for _, nearby := range pins.pins {
			return nearby.pin, nil
		}
	}

	return nil, nil
}

func (m *Map) optimizeDesegregate(opts *Options) (connectTo *Pin, err error) {
	// Check if the network we belog to is less than 50% of the network.
	var reachable int
	for _, pin := range m.all {
		if pin.State.has(StateReachable) {
			reachable++
		}
	}
	// If we are part of the bigger network, everything is good.
	if reachable > len(m.all)/2 {
		return nil, nil
	}

	// If we are part of the smaller network, try to connect to the bigger network.

	// Copy opts as we are going to make changes.
	opts = opts.Copy()
	if !opts.NoDefaults {
		opts.NoDefaults = true
		opts.Regard = opts.Regard.add(StateSummaryRegard)
		opts.Disregard = opts.Disregard.add(StateSummaryDisregard)
	}
	// Move reachable from regard to disregard.
	opts.Regard = opts.Regard.remove(StateReachable)
	opts.Disregard = opts.Disregard.add(StateReachable)

	// Iterate over all Pins to find any matching Pin.
	matcher := opts.Matcher(TransitHub)
	for _, pin := range m.all {
		if matcher(pin) {
			log.Warningf("spn/navigator: recommending to connect to %s to desegregate map %s", pin.Hub, m.Name)
			return pin, nil
		}
	}

	return nil, nil
}
