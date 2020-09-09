package navigator

import (
	"github.com/safing/portbase/log"
)

type RoutingProfile struct {
	ID string

	// MinHops defines how many hops a route must have at minimum. In order to reduce confusion, the Home Hub is also counted.
	MinHops int

	MaxHops int

	// MaxExtraHops sets a limit on how many extra hops are allowed in addition to the amount of Hops in the currently best route. This is an optimization option and should not interfere with finding the best route, but might reduce the amount of routes found.
	MaxExtraHops int

	// MaxExtraCost sets a limit on the extra cost allowed in addition to the cost of the currently best route. This is an optimization option and should not interfere with finding the best route, but might reduce the amount of routes found.
	MaxExtraCost int
}

const (
	RoutingProfileDefaultName  = "default"
	RoutingProfileShortestName = "shortest"
)

var (
	RoutingProfileDefault = &RoutingProfile{
		ID:           RoutingProfileDefaultName,
		MinHops:      3,
		MaxHops:      5,
		MaxExtraHops: 2,
		MaxExtraCost: 100, // TODO: implement costs
	}

	RoutingProfileShortest = &RoutingProfile{
		ID:           RoutingProfileShortestName,
		MinHops:      1,
		MaxHops:      5,
		MaxExtraHops: 1,
		MaxExtraCost: 100, // TODO: implement costs
	}
)

func getRoutingProfile(name string) *RoutingProfile {
	switch name {
	case RoutingProfileDefaultName:
		return RoutingProfileDefault
	case RoutingProfileShortestName:
		return RoutingProfileShortest
	default:
		log.Warningf("spn/navigator: routing profile %q does not exist, falling back to default", name)
		return RoutingProfileDefault
	}
}

type routeCompliance uint8

const (
	routeOk           routeCompliance = iota // Route is fully compliant and can be used.
	routeNonCompliant                        // Route is not compliant, but this might change if more hops are added.
	routeDisqualified                        // Route is disqualified and won't be able to become compliant.
)

func (rp *RoutingProfile) checkRouteCompliance(route *Route, foundRoutes *Routes) routeCompliance {
	switch {
	case len(route.Path) < rp.MinHops:
		// Route is shorter than the defined minimum.
		return routeNonCompliant
	case len(route.Path) > rp.MaxHops:
		// Route is longer than the defined maximum.
		return routeDisqualified
	}

	// Abort route exploration when we are outside the optimization boundaries.
	if len(foundRoutes.All) > 0 {
		// Get the best found route.
		best := foundRoutes.All[0]
		// Abort if current route exceeds max extra costs.
		if route.TotalCost > best.TotalCost+rp.MaxExtraCost {
			return routeDisqualified
		}
		// Abort if current route exceeds max extra hops.
		if len(route.Path) > len(best.Path)+rp.MaxExtraHops {
			return routeDisqualified
		}
	}

	return routeOk
}
