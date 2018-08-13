package navigator

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/network/geoip"
	"github.com/Safing/safing-core/port17"
	"github.com/Safing/safing-core/port17/bottle"
)

// Port represents a node in the Port17 network.
type Port struct {
	sync.RWMutex

	Bottle         *bottle.Bottle
	IgnoreUntil    int64 // e.g. if an error occurred with this port
	Trusted        bool
	Location4      *geoip.Location
	Location6      *geoip.Location
	Routes         []*Route
	ActiveAPI      *port17.API // Api to active Port
	ActiveRoute    []*Port     // list of Ports this connection runs through
	DependingPorts []*Port     // list of Ports that use this Port for a connection
	Load           int         // estimated in microseconds this port adds to latency
}

func NewPort(newBottle *bottle.Bottle) *Port {
	new := &Port{
		Bottle: newBottle,
		Load:   newBottle.Load,
	}
	new.CheckLocation()
	new.CheckTrustStatus()
	return new
}

func (p *Port) String() string {
	return fmt.Sprintf("<Port %s L=%d C=%d R=%v>", p.Name(), p.Load, p.Cost(), p.Routes)
}

func (p *Port) Name() string {
	return p.Bottle.PortName
}

func (p *Port) CheckLocation() {
	// get IPv4 location
	if len(p.Bottle.IPv4) > 0 {
		loc, err := geoip.GetLocation(p.Bottle.IPv4)
		if err != nil {
			log.Warningf("port17/navigator: failed to get IPv4 location of %s for Port %s: %s", p.Bottle.IPv4, p.Name(), err)
		} else {
			p.Location4 = loc
		}
	}

	// get IPv6 location
	if len(p.Bottle.IPv6) > 0 {
		loc, err := geoip.GetLocation(p.Bottle.IPv6)
		if err != nil {
			log.Warningf("port17/navigator: failed to get IPv6 location of %s for Port %s: %s", p.Bottle.IPv6, p.Name(), err)
		} else {
			p.Location6 = loc
		}
	}
}

func (p *Port) HasActiveRoute() bool {
	p.RLock()
	defer p.RUnlock()
	return p.ActiveAPI != nil && !p.ActiveAPI.IsAbandoned()
}

func (p *Port) ActiveRouteCost() int {
	totalCost := p.ActiveCost()
	var route *Route
	lastPort := p

	for i := len(p.ActiveRoute) - 2; i >= 0; i-- {
		port := p.ActiveRoute[i]
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
		totalCost += port.ActiveCost()
		// set new lastPort
		lastPort = port
	}
	return totalCost
}

func (p *Port) AddPortDependent(dependentPort *Port) {
	p.Lock()
	defer p.Unlock()
	p.DependingPorts = append(p.DependingPorts, dependentPort)
}

func (p *Port) RemovePortDependent(dependentPort *Port) {
	p.Lock()
	defer p.Unlock()
	for i, dPort := range p.DependingPorts {
		if dPort == dependentPort {
			p.DependingPorts = append(p.DependingPorts[:i], p.DependingPorts[i+1:]...)
			return
		}
	}
}

func (p *Port) Ignored() bool {
	p.RLock()
	defer p.RUnlock()
	return p.IgnoreUntil < time.Now().Unix()
}

func (p *Port) CheckTrustStatus() {
	p.Lock()
	defer p.Unlock()
	p.Trusted = IsPortTrusted(p)
}

func (p *Port) UpdateLoad(load int) {
	p.Lock()
	defer p.Unlock()
	p.Load = load
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

func (p *Port) Cost() int {
	p.RLock()
	defer p.RUnlock()
	return sanityCheckCost(int(costAt0 * math.Pow(growthRate, float64(p.Load))))
}

func (p *Port) ActiveCost() int {
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

func (p *Port) AddRoute(newPort *Port, cost int) {
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

func (p *Port) RemoveRoute(newPort *Port) {
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

func (p *Port) Equal(other *Port) bool {
	return p.Bottle.PortName == other.Bottle.PortName
}
