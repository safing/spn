package navigator

import (
	"context"
	"time"

	"github.com/safing/portmaster/intel"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
)

// Pin represents a Hub on a Map.
type Pin struct {
	// Hub Information
	Hub        *hub.Hub
	EntityV4   *intel.Entity
	EntityV6   *intel.Entity
	LocationV4 *geoip.Location
	LocationV6 *geoip.Location

	// Hub Status
	State       PinState
	HopDistance int
	Load        int // estimated in microseconds this port adds to latency
	ConnectedTo map[string]*Lane

	// FailingUntil specifies until when this Hub should be regarded as failing.
	// This is connected to StateFailing.
	FailingUntil time.Time

	// API Status
	ActiveAPI        *docks.API // API to active Pin
	ConnectedThrough []*Pin     // list of Pins the connection to this Hub runs through
	Dependants       []*Pin     // list of Pins that use this Hub for a connection

	// Internal

	// pushChanges is set to true if something noteworthy on the Pin changed and
	// an update needs to be pushed by the database storage interface to whoever
	// is listening.
	pushChanges bool
}

// Lane is a connection to another Hub.
type Lane struct {
	// Pin is the Pin/Hub this Lane connects to.
	Pin *Pin

	// Capacity designates the available bandwidth between these Hubs.
	// It is specified in MBit/s.
	Capacity int

	// Lateny designates the latency between these Hubs.
	// It is specified in Milliseconds.
	Latency int

	// active is a helper flag in order help remove abandoned Lanes.
	active bool
}

// String returns a human-readable representation of the Pin.
func (pin *Pin) String() string {
	return "<Pin " + pin.Hub.Name() + ">"
}

// updateLocationData fetches the necessary location data in order to correctly map out the Pin.
func (pin *Pin) updateLocationData() {
	if pin.Hub.Info.IPv4 != nil {
		pin.EntityV4 = &intel.Entity{
			IP: pin.Hub.Info.IPv4,
		}
		var ok bool
		pin.LocationV4, ok = pin.EntityV4.GetLocation(context.TODO())
		if !ok {
			log.Warningf("navigator: failed to get location of %s of %s", pin.Hub.Info.IPv4, pin)
			return
		}
	} else {
		pin.EntityV4 = nil
		pin.LocationV4 = nil
	}

	if pin.Hub.Info.IPv6 != nil {
		pin.EntityV6 = &intel.Entity{
			IP: pin.Hub.Info.IPv6,
		}
		var ok bool
		pin.LocationV6, ok = pin.EntityV6.GetLocation(context.TODO())
		if !ok {
			log.Warningf("navigator: failed to get location of %s of %s", pin.Hub.Info.IPv6, pin)
			return
		}
	} else {
		pin.EntityV6 = nil
		pin.LocationV6 = nil
	}
}

/*
func (pin *Pin) hasActiveRoute() bool {
	return pin.ActiveAPI != nil && !pin.ActiveAPI.IsAbandoned()
}

func (pin *Pin) ActiveRouteCost() int {
	totalCost := p.ActiveCost()
	var route *Route
	lastPort := p

	for i := len(pin.ActiveRoute) - 2; i >= 0; i-- {
		port := pin.ActiveRoute[i]
		// find correct route
		route = nil
		for _, route = range port.Routes {
			if route.Port.Equal(lastPort) {
				break
			}
		}
		// return max value if not found
		if route == nil {
			return math.MaxInt32
		}
		// add route cost to total cost
		totalCost += route.Cost
		// add port cost to total cost
		totalCost += pin.ActiveCost()
		// set new lastPort
		lastPort = port
	}
	return totalCost
}

func (p *Pin) AddPortDependent(dependentPort *Pin) {
	p.Lock()
	defer p.Unlock()
	p.DependingPorts = append(p.DependingPorts, dependentPort)
}

func (p *Pin) RemovePortDependent(dependentPort *Pin) {
	p.Lock()
	defer p.Unlock()
	for i, dPort := range p.DependingPorts {
		if dPort == dependentPort {
			p.DependingPorts = append(p.DependingPorts[:i], p.DependingPorts[i+1:]...)
			return
		}
	}
}

const (
	costAt0  float64 = 3000
	costAt90 float64 = 20000

	activeCostAt0  float64 = 1000
	activeCostAt95 float64 = 20000
)

var (
	growthRate       float64 = math.Pow(costAt90/costAt0, float64(1)/float64(90))
	activeGrowthRate float64 = math.Pow(activeCostAt95/activeCostAt0, float64(1)/float64(95))
)

func (p *Pin) Cost() int {
	p.RLock()
	defer p.RUnlock()
	return sanityCheckCost(int(costAt0 * math.Pow(growthRate, float64(p.Load))))
}

func (p *Pin) ActiveCost() int {
	p.RLock()
	defer p.RUnlock()
	return sanityCheckCost(int(activeCostAt0 * math.Pow(activeGrowthRate, float64(p.Load))))
}

func sanityCheckCost(cost int) int {
	if cost < 0 || cost > math.MaxInt32 {
		return math.MaxInt32
	}
	return cost
}

func (p *Pin) AddRoute(newPort *Pin, cost int) {
	p.Lock()
	newPort.Lock()
	// check if route is already here
	found := false
	for _, route := range p.Routes {
		if route.Port.Equal(newPort) {
			found = true
			break
		}
	}
	// add if not found
	if !found {
		p.Routes = append(p.Routes, &Route{
			Port: newPort,
			Cost: cost,
		})
	}
	p.Unlock()
	newPort.Unlock()
	// also add reverse if not found
	if !found {
		newPort.AddRoute(p, cost)
	}
}

func (p *Pin) RemoveRoute(newPort *Pin) {
	p.Lock()
	newPort.Lock()
	// check if route is already here
	var route *Route
	removeIndex := -1
	for removeIndex, route = range p.Routes {
		if route.Port.Equal(newPort) {
			break
		}
	}
	// remove if found
	if removeIndex >= 0 {
		p.Routes = append(p.Routes[:removeIndex], p.Routes[removeIndex+1:]...)
	}
	p.Unlock()
	newPort.Unlock()
	// also remove reverse route
	if removeIndex >= 0 {
		newPort.RemoveRoute(p)
	}
}

func (p *Pin) Equal(other *Pin) bool {
	return p.Hub.ID == other.Hub.ID
}
*/
