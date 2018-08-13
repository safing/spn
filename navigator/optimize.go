package navigator

import (
	"errors"
	"net"
	"sync"

	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/bottle"
)

var (
	minProximity = 2
	ErrIAmLonely = errors.New("could not find any ports")
)

func Optimize(portName string) (*bottle.Bottle, error) {
	log.Infof("navigator: optimizing network for %s", portName)
	return optimizeNetwork(portName, publicPorts, publicPortsLock.RLocker())
}

func optimizeNetwork(portName string, collection map[string]*Port, lock sync.Locker) (*bottle.Bottle, error) {

	// get port
	lock.Lock()
	port, ok := collection[portName]
	lock.Unlock()
	if !ok {
		return nil, errors.New("could not find starting port")
	}

	// check for any connections
	if len(port.Routes) == 0 {
		log.Tracef("port17/navigator: port has no routes, finding nearest port")
		m := NewMap(port, collection, lock)
		var ips []net.IP
		if port.Bottle.IPv4 != nil {
			ips = append(ips, port.Bottle.IPv4)
		}
		if port.Bottle.IPv6 != nil {
			ips = append(ips, port.Bottle.IPv6)
		}
		if len(ips) == 0 {
			return nil, errors.New("primary port does not have any IPs")
		}
		col, err := m.FindNearestPorts(ips)
		if err != nil {
			return nil, err
		}
		if col.Len() == 0 {
			return nil, ErrIAmLonely
		}
		return col.All[0].Port.Bottle, nil
	}

	// search for furthest node

	// lock.Lock()
	// defer lock.Unlock()

	hops := make(map[*Port]int)
	next := []*Port{port}

	// iterate through network
	for distance := 1; distance < 100; distance++ {
		var nextHops []*Port
		// go through ports of next distance
		for _, port := range next {
			// if port not yet in list, add with distance
			if _, ok := hops[port]; !ok {
				hops[port] = distance
				for _, route := range port.Routes {
					// save hops for next iteration
					nextHops = append(nextHops, route.Port)
				}
			}
		}
		next = nextHops
	}

	// return furthest away node not in proximity
	var candidate *bottle.Bottle
	candidateDistance := 0
	for port, distance := range hops {
		if distance > minProximity && distance > candidateDistance {
			candidateDistance = distance
			candidate = port.Bottle
		}
	}

	// debugging
	// for key, val := range hops {
	// 	fmt.Printf("%s: %d\n", key.Name(), val)
	// }

	return candidate, nil

}
