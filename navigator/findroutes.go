package navigator

import (
	"errors"
	"fmt"
	"net"

	"github.com/safing/portmaster/intel/geoip"
)

// FindRoutes finds possible routes to the given IP, with the given options.
func (m *Map) FindRoutes(ip net.IP, opts *Options, maxRoutes int) (*Routes, error) {
	m.Lock()
	defer m.Unlock()

	// Check if map is populated.
	if m.isEmpty() {
		return nil, ErrEmptyMap
	}

	// Check if home hub is set.
	if m.home == nil {
		return nil, ErrHomeHubUnset
	}

	// Set default options if unset.
	if opts == nil {
		opts = m.defaultOptions()
	}

	// Handle special home routing profile.
	if opts.RoutingProfile == RoutingProfileHomeID {
		return &Routes{
			All: []*Route{{
				Path: []*Hop{{
					pin:   m.home,
					HubID: m.home.Hub.ID,
				}},
				Algorithm: RoutingProfileHomeID,
			}},
		}, nil
	}

	// Get the location of the given IP address.
	var locationV4, locationV6 *geoip.Location
	var err error
	// Save whether the given IP address is a IPv4 or IPv6 address.
	if v4 := ip.To4(); v4 != nil {
		locationV4, err = geoip.GetLocation(ip)
	} else {
		locationV6, err = geoip.GetLocation(ip)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get IP location: %w", err)
	}

	// Find nearest Pins.
	nearby, err := m.findNearestPins(locationV4, locationV6, opts.Matcher(DestinationHub, m.intel), maxRoutes)
	if err != nil {
		return nil, err
	}

	return m.findRoutes(nearby, opts, maxRoutes)
}

// FindRouteToHub finds possible routes to the given Hub, with the given options.
func (m *Map) FindRouteToHub(hubID string, opts *Options, maxRoutes int) (*Routes, error) {
	m.Lock()
	defer m.Unlock()

	// Get Pin.
	pin, ok := m.all[hubID]
	if !ok {
		return nil, ErrHubNotFound
	}

	// Create a nearby with a single Pin.
	nearby := &nearbyPins{
		pins: []*nearbyPin{
			{
				pin: pin,
			},
		},
	}

	// Find a route to the given Hub.
	return m.findRoutes(nearby, opts, maxRoutes)
}

func (m *Map) findRoutes(dsts *nearbyPins, opts *Options, maxRoutes int) (*Routes, error) {
	if m.home == nil {
		return nil, ErrHomeHubUnset
	}

	// Initialize matchers.
	var done bool
	transitMatcher := opts.Matcher(TransitHub, m.intel)
	destinationMatcher := opts.Matcher(DestinationHub, m.intel)
	routingProfile := getRoutingProfile(opts.RoutingProfile)

	// Create routes collector.
	routes := &Routes{
		maxRoutes: maxRoutes,
	}

	// TODO: Start from the destination and use HopDistance to prioritize
	// exploring routes that are in the right direction.

	// Create initial route.
	route := &Route{
		// Estimate how much space we will need, else it'll just expand.
		Path: make([]*Hop, 1, routingProfile.MinHops+routingProfile.MaxExtraHops),
	}
	route.Path[0] = &Hop{
		pin: m.home,
		// TODO: add initial cost
	}

	// exploreHop explores a hop (Lane) to a connected Pin.
	var exploreHop func(route *Route, lane *Lane)

	// exploreLanes explores all Lanes of a Pin.
	exploreLanes := func(route *Route) {
		for _, lane := range route.Path[len(route.Path)-1].pin.ConnectedTo {
			// Check if we are done and can skip the rest.
			if done {
				return
			}

			// Explore!
			exploreHop(route, lane)
		}
	}

	exploreHop = func(route *Route, lane *Lane) {
		// Check if the Pin should be regarded as Transit Hub.
		if !transitMatcher(lane.Pin) {
			return
		}

		// Add Pin to the current path and remove when done.
		route.addHop(lane.Pin, lane.Cost+lane.Pin.Cost)
		defer route.removeHop()

		// Check if the route would even make it into the list.
		if !routes.isGoodEnough(route) {
			return
		}

		// Check route compliance.
		// This also includes some algorithm-based optimizations.
		switch routingProfile.checkRouteCompliance(route, routes) {
		case routeOk:
			// Route would be compliant.
			// Now, check if the last hop qualifies as a Destination Hub.
			if destinationMatcher(lane.Pin) {
				// Get Pin as nearby Pin.
				nbPin := dsts.get(lane.Pin.Hub.ID)
				if nbPin != nil {
					// Pin is listed as selected Destination Hub!
					// Complete route to add destination ("last mile") cost.
					route.completeRoute(nbPin.DstCost())
					routes.add(route)

					// We have found a route and have come to an end here.
					return
				}
			}

			// The Route is compliant, but we haven't found a Destination Hub yet.
			fallthrough
		case routeNonCompliant:
			// Continue exploration.
			exploreLanes(route)
		case routeDisqualified:
			fallthrough
		default:
			// Route is disqualified and we can return without further exploration.
		}
	}

	// Start the hop exploration tree.
	// This will fork into about a gazillion branches and add all the found valid
	// routes to the list.
	exploreLanes(route)

	// Check if we found anything.
	if len(routes.All) == 0 {
		return nil, errors.New("failed to find any routes")
	}

	routes.makeExportReady(opts.RoutingProfile)
	return routes, nil
}
