package navigator

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/spn/hub"
)

// nearbyPins is a list of nearby Pins to a certain location.
type nearbyPins struct {
	pins         []*nearbyPin
	minProximity int
	maxPins      int
}

// nearbyPin represents a Pin and the proximity to a certain location.
type nearbyPin struct {
	pin       *Pin
	proximity int
}

// Len is the number of elements in the collection.
func (nb *nearbyPins) Len() int {
	return len(nb.pins)
}

// Less reports whether the element with index i should sort before the element
// with index j.
func (nb *nearbyPins) Less(i, j int) bool {
	return nb.pins[i].proximity > nb.pins[j].proximity
}

// Swap swaps the elements with indexes i and j.
func (nb *nearbyPins) Swap(i, j int) {
	nb.pins[i], nb.pins[j] = nb.pins[j], nb.pins[i]
}

// add potentially adds a Pin to the list of nearby Pins.
func (nb *nearbyPins) add(pin *Pin, proximity int) {
	if proximity < nb.minProximity {
		return
	}

	nb.pins = append(nb.pins, &nearbyPin{
		pin:       pin,
		proximity: proximity,
	})
}

// contains checks if the collection contains a Pin.
func (nb *nearbyPins) get(id string) *nearbyPin {
	for _, nbPin := range nb.pins {
		if nbPin.pin.Hub.ID == id {
			return nbPin
		}
	}

	return nil
}

// clean sort and shortens the list to the configured maximum.
func (nb *nearbyPins) clean() {
	// Sort nearby Pins so that the closest one is on top.
	sort.Sort(nb)
	// Remove all remaining from the list.
	if len(nb.pins) > nb.maxPins {
		nb.pins = nb.pins[:nb.maxPins]
	}
	// Set new minimum proximity.
	if len(nb.pins) > 0 {
		nb.minProximity = nb.pins[len(nb.pins)-1].proximity
	}
}

// nearbyPin represents a Pin and the proximity to a certain location.
func (nb *nearbyPin) DstCost() float32 {
	return CalculateDestinationCost(nb.proximity)
}

// FindNearestHubs searches for the nearest Hubs to the given IP address. The returned Hubs must not be modified in any way.
func (m *Map) FindNearestHubs(locationV4, locationV6 *geoip.Location, opts *Options, matchFor HubType, maxMatches int) ([]*hub.Hub, error) {
	m.RLock()
	defer m.RUnlock()

	// Check if map is populated.
	if m.isEmpty() {
		return nil, ErrEmptyMap
	}

	// Set default options if unset.
	if opts == nil {
		opts = m.defaultOptions()
	}

	// Find nearest Pins.
	nearby, err := m.findNearestPins(locationV4, locationV6, opts.Matcher(matchFor), maxMatches)
	if err != nil {
		return nil, err
	}

	// Convert to Hub list and return.
	hubs := make([]*hub.Hub, 0, len(nearby.pins))
	for _, nbPin := range nearby.pins {
		hubs = append(hubs, nbPin.pin.Hub)
	}
	return hubs, nil
}

func (m *Map) findNearestPins(locationV4, locationV6 *geoip.Location, matcher PinMatcher, maxMatches int) (*nearbyPins, error) {
	if locationV4 == nil && locationV6 == nil {
		return nil, errors.New("no location provided")
	}

	// Create nearby Pins list.
	nearby := &nearbyPins{
		maxPins: maxMatches,
	}

	// Iterate over all Pins in the Map to find the nearest ones.
	for _, pin := range m.all {
		// Check if the Pin matches the criteria.
		if !matcher(pin) {
			// Debugging:
			// log.Tracef("spn/navigator: skipping %s with states %s for finding nearest", pin, pin.State)
			continue
		}

		// Calculate IPv4 proximity and add Pin to the list.
		if locationV4 != nil && pin.LocationV4 != nil {
			proximity := pin.LocationV4.EstimateNetworkProximity(locationV4)
			nearby.add(pin, proximity)
		}

		// Calculate IPv6 proximity and add Pin to the list.
		if locationV6 != nil && pin.LocationV6 != nil {
			// Calculate proximity and add Pin to the list.
			proximity := pin.LocationV6.EstimateNetworkProximity(locationV6)
			nearby.add(pin, proximity)
		}

		// Clean the nearby list if have collected more than two times the max amount.
		if len(nearby.pins) >= nearby.maxPins*2 {
			nearby.clean()
		}
	}

	// Clean one last time and return the list.
	nearby.clean()
	return nearby, nil
}

func (nb *nearbyPins) String() string {
	s := make([]string, 0, len(nb.pins))
	for _, nbPin := range nb.pins {
		s = append(s, nbPin.String())
	}
	return strings.Join(s, ", ")
}

func (nb *nearbyPin) String() string {
	return fmt.Sprintf("%s at %d prox", nb.pin, nb.proximity)
}
