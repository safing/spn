package navigator

import (
	"errors"
	"fmt"

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
	if m.Home == nil {
		return nil, ErrHomeHubUnset
	}

	// Create default matcher.
	matcher := opts.Matcher(DestinationHub)

	// Define loop variables.
	var (
		mostDistantPin     *Pin
		highestHopDistance = optimizationHopDistanceTarget
	)

	// Iterate over all Pins to find the most distant Pin.
	var matchedAny bool
	for _, pin := range m.All {
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
		return nil, nil
	}

	// If no pin matched at all, we don't seem connected to anyone.
	// Find the nearest non-connected pin, to bootstrap to the network.
	m.RLock()
	defer m.RUnlock()

	pins, err := m.findNearestPins(m.Home.LocationV4, m.Home.LocationV6, opts.Matcher(HomeHub), 10)
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
