package navigator

import (
	"fmt"
	"net"
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
func (nb *nearbyPin) DstCost() int {
	return 100 - nb.proximity // TODO: weigh with other costs
}

// FindNearestHubs searches for the nearest Hubs to the given IP address. The returned Hubs must not be modified in any way.
func (m *Map) FindNearestHubs(ip net.IP, opts *Options, maxMatches int) ([]*hub.Hub, error) {
	m.RLock()
	defer m.RUnlock()

	if m.isEmpty() {
		return nil, ErrEmptyMap
	}

	// Find nearest Pins.
	nearby, err := m.findNearestPins(ip, opts.Matcher(DestinationHub), maxMatches)
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

func (m *Map) findNearestPins(ip net.IP, matcher PinMatcher, maxMatches int) (*nearbyPins, error) {
	// Save whether the given IP address is a IPv4 or IPv6 address.
	v4 := ip.To4() != nil

	// Get the location of the given IP address.
	location, err := geoip.GetLocation(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to get IP location: %w", err)
	}

	// Create nearby Pins list.
	nearby := &nearbyPins{
		maxPins: maxMatches,
	}

	// Iterate over all Pins in the Map to find the nearest ones.
	for _, pin := range m.All {
		// Check if the Pin matches the criteria.
		if !matcher(pin) {
			// fmt.Printf("skipping %s\n", pin)
			continue
		}

		// Calculate proximity to the given IP address.
		var proximity int
		if v4 {
			proximity = pin.LocationV4.EstimateNetworkProximity(location)
		} else {
			proximity = pin.LocationV6.EstimateNetworkProximity(location)
		}

		// Add Pin to the list with the calculated proximity.
		nearby.add(pin, proximity)
		// fmt.Printf("added %s with %d proximity\n", pin, proximity)

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
