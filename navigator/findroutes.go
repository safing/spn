package navigator

import (
	"net"
)

func (m *Map) FindRoutes(ip net.IP, opts *Options, maxRoutes int) (*Routes, error) {
	m.Lock()
	defer m.Unlock()

	if m.isEmpty() {
		return nil, ErrEmptyMap
	}

	// Find nearest Pins.
	nearby, err := m.findNearestPins(ip, opts.Matcher(DestinationHub), maxRoutes)
	if err != nil {
		return nil, err
	}

	return m.findRoutes(nearby, opts, maxRoutes)
}

func (m *Map) findRoutes(dsts *nearbyPins, opts *Options, maxRoutes int) (*Routes, error) {
	if m.Home == nil {
		return nil, ErrHomeHubUnset
	}

	// Initialize matchers.
	var done bool
	transitMatcher := opts.Matcher(TransitHub)
	destinationMatcher := opts.Matcher(DestinationHub)
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
		Path: make([]*Hop, 1, routingProfile.MinHops*2+routingProfile.MaxExtraHops),
	}
	route.Path[0] = &Hop{
		pin: m.Home,
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
		route.addHop(lane.Pin, lane.Latency)
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
		default:
			// Route is disqualified and we can return without further exploration.
		}
	}

	// Start the hop exploration tree.
	// This will fork into about a gazillion branches and add all the found valid
	// routes to the list.
	exploreLanes(route)

	return routes, nil
}
