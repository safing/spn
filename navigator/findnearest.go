package navigator

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/spn/hub"
)

// DefaultMaxFindMatches defines a default value of how many matches a find
// operation in a map should return.
const DefaultMaxFindMatches = 100

// nearbyPins is a list of nearby Pins to a certain location.
type nearbyPins struct {
	pins        []*nearbyPin
	minPins     int
	maxPins     int
	maxCost     float32
	cutOffLimit float32
}

// nearbyPin represents a Pin and the proximity to a certain location.
type nearbyPin struct {
	pin  *Pin
	cost float32
}

// Len is the number of elements in the collection.
func (nb *nearbyPins) Len() int {
	return len(nb.pins)
}

// Less reports whether the element with index i should sort before the element
// with index j.
func (nb *nearbyPins) Less(i, j int) bool {
	return nb.pins[i].cost < nb.pins[j].cost
}

// Swap swaps the elements with indexes i and j.
func (nb *nearbyPins) Swap(i, j int) {
	nb.pins[i], nb.pins[j] = nb.pins[j], nb.pins[i]
}

// add potentially adds a Pin to the list of nearby Pins.
func (nb *nearbyPins) add(pin *Pin, cost float32) {
	if len(nb.pins) > nb.minPins && nb.maxCost > 0 && cost > nb.maxCost {
		return
	}

	nb.pins = append(nb.pins, &nearbyPin{
		pin:  pin,
		cost: cost,
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

	// Set maximum cost based on max difference, if we have enough pins.
	if len(nb.pins) >= nb.minPins {
		nb.maxCost = nb.pins[0].cost + nb.cutOffLimit
	}

	// Remove superfluous Pins from the list.
	if len(nb.pins) > nb.maxPins {
		nb.pins = nb.pins[:nb.maxPins]
	}
	// Remove Pins that are too costly.
	if len(nb.pins) > nb.minPins {
		// Search for first pin that is too costly.
		okUntil := nb.minPins
		for ; okUntil < len(nb.pins); okUntil++ {
			if nb.pins[okUntil].cost > nb.maxCost {
				break
			}
		}
		// Cut off the list at that point.
		nb.pins = nb.pins[:okUntil]
	}
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
	nearby, err := m.findNearestPins(locationV4, locationV6, opts, matchFor, maxMatches)
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

func (m *Map) findNearestPins(locationV4, locationV6 *geoip.Location, opts *Options, matchFor HubType, maxMatches int) (*nearbyPins, error) {
	// Fail if no location is provided.
	if locationV4 == nil && locationV6 == nil {
		return nil, errors.New("no location provided")
	}

	// Raise maxMatches to nearestPinsMinimum.
	if maxMatches < nearestPinsMinimum {
		maxMatches = nearestPinsMinimum
	}

	// Create nearby Pins list.
	nearby := &nearbyPins{
		minPins:     nearestPinsMinimum,
		maxPins:     maxMatches,
		cutOffLimit: nearestPinsMaxCostDifference,
	}

	// Create pin matcher.
	matcher := opts.Matcher(matchFor, m.intel)

	// Iterate over all Pins in the Map to find the nearest ones.
	for _, pin := range m.all {
		var cost float32

		// Check if the Pin matches the criteria.
		if !matcher(pin) {
			// Debugging:
			// log.Tracef("spn/navigator: skipping %s with states %s for finding nearest", pin, pin.State)
			continue
		}

		// If finding a home hub, check if there is a common IP version.
		if matchFor == HomeHub {
			switch {
			case locationV4 != nil && pin.LocationV4 != nil:
				// Both have IPv4!
			case locationV6 != nil && pin.LocationV6 != nil:
				// Both have IPv6!
			default:
				// No shared IP stack.
				continue
			}
		}

		// 1. Calculate cost based on distance

		if locationV4 != nil && pin.LocationV4 != nil {
			if locationV4.IsAnycast && m.home != nil {
				// If the destination is anycast, calculate cost though proximity to home hub instead, if possible.
				cost = lessButPositive(cost, CalculateDestinationCost(
					proximityBetweenPins(pin, m.home),
				))
			} else {
				// Regular cost calculation through proximity.
				cost = lessButPositive(cost, CalculateDestinationCost(
					locationV4.EstimateNetworkProximity(pin.LocationV4),
				))
			}
		}

		if locationV6 != nil && pin.LocationV6 != nil {
			if locationV6.IsAnycast && m.home != nil {
				// If the destination is anycast, calculate cost though proximity to home hub instead, if possible.
				cost = lessButPositive(cost, CalculateDestinationCost(
					proximityBetweenPins(pin, m.home),
				))
			} else {
				// Regular cost calculation through proximity.
				cost = lessButPositive(cost, CalculateDestinationCost(
					locationV6.EstimateNetworkProximity(pin.LocationV6),
				))
			}
		}

		// If no cost could be calculated, fall back to a default value.
		if cost == 0 {
			cost = CalculateDestinationCost(50) // proximity out of 0-100
		}

		// Debugging:
		// if matchFor == HomeHub {
		// 	log.Tracef("spn/navigator: adding %.2f proximity cost to home hub %s", cost, pin.Hub)
		// }

		// 2. Add cost based on Hub status

		cost += CalculateHubCost(pin.Hub.Status.Load)

		// Debugging:
		// if matchFor == HomeHub {
		// 	log.Tracef("spn/navigator: adding %.2f hub cost to home hub %s", CalculateHubCost(pin.Hub.Status.Load), pin.Hub)
		// }

		// 3. If matching a home hub, add cost based on capacity/latency performance.

		if matchFor == HomeHub {
			// Find best capacity/latency values.
			var (
				bestCapacity int
				bestLatency  time.Duration
			)
			for _, lane := range pin.Hub.Status.Lanes {
				if lane.Capacity > bestCapacity {
					bestCapacity = lane.Capacity
				}
				if bestLatency == 0 || lane.Latency < bestLatency {
					bestLatency = lane.Latency
				}
			}
			// Add cost of best capacity/latency values.
			cost += CalculateLaneCost(bestLatency, bestCapacity)

			// Debugging:
			// log.Tracef("spn/navigator: adding %.2f lane cost to home hub %s", CalculateLaneCost(bestLatency, bestCapacity), pin.Hub)
			// log.Debugf("spn/navigator: total cost of %.2f to home hub %s", cost, pin.Hub)
		}

		nearby.add(pin, cost)

		// Clean the nearby list if have collected more than two times the max amount.
		if len(nearby.pins) >= nearby.maxPins*2 {
			nearby.clean()
		}
	}

	// Check if we found any nearby pins
	if nearby.Len() == 0 {
		return nil, errors.New("no pins found near destination")
	}

	// Clean one last time and return the list.
	nearby.clean()

	// Debugging:
	// if matchFor == HomeHub {
	// 	log.Debug("spn/navigator: nearby pins:")
	// 	for _, nbPin := range nearby.pins {
	// 		log.Debugf("spn/navigator: nearby pin %s", nbPin)
	// 	}
	// }

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
	return fmt.Sprintf("%s at %.2fc", nb.pin, nb.cost)
}

func proximityBetweenPins(a, b *Pin) float32 {
	var x, y float32

	// Get IPv4 network proximity.
	if a.LocationV4 != nil && b.LocationV4 != nil {
		x = a.LocationV4.EstimateNetworkProximity(b.LocationV4)
	}

	// Get IPv6 network proximity.
	if a.LocationV6 != nil && b.LocationV6 != nil {
		y = a.LocationV6.EstimateNetworkProximity(b.LocationV6)
	}

	// Return higher proximity.
	if x > y {
		return x
	}
	return y
}

func lessButPositive(a, b float32) float32 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}
