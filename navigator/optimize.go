package navigator

import "github.com/safing/spn/hub"

const (
	optimizationHopDistanceTarget = 3
)

// FindNearestHubs searches for the nearest Hubs to the given IP address. The returned Hubs must not be modified in any way.
func (m *Map) Optimize(opts *Options) (connectTo *hub.Hub, err error) {
	m.RLock()
	defer m.RUnlock()

	// Check if
	if m.isEmpty() {
		return nil, ErrEmptyMap
	}

	// Set default options if unset.
	if opts == nil {
		opts = m.defaultOptions()
	}

	pin, err := m.optimize(opts)
	if err != nil {
		return nil, err
	}

	return pin.Hub, nil
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
	for _, pin := range m.All {
		// Check if the Pin matches the criteria.
		if !matcher(pin) {
			continue
		}

		if pin.HopDistance > highestHopDistance {
			highestHopDistance = pin.HopDistance
			mostDistantPin = pin
		}
	}

	return mostDistantPin, nil
}
