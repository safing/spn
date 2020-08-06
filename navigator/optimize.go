package navigator

import (
	"errors"
	"sync"

	"github.com/safing/spn/docks"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
)

var (
	minProximity = 2
	ErrIAmLonely = errors.New("could not find any ports")
)

func Optimize(self *hub.Hub) (connectTo *hub.Hub, err error) {
	log.Infof("navigator: optimizing network for %s", self)
	return optimizeNetwork(self, publicPorts, publicPortsLock.RLocker())
}

func optimizeNetwork(self *hub.Hub, collection map[string]*Port, lock sync.Locker) (connectTo *hub.Hub, err error) {

	// TODO: Revamp
	// workaround until fixed: return random Hub

	foundAny := false
	for _, r := range collection {
		// skip self
		if r.Hub.ID == self.ID {
			continue
		}
		// skip hubs that already have a connection
		crane := docks.GetAssignedCrane(r.Hub.ID)
		if crane != nil {
			foundAny = true
			continue
		}
		// return first match
		return r.Hub, nil
	}

	if !foundAny {
		return nil, ErrIAmLonely
	}

	return nil, nil

	/*
		// get port
		lock.Lock()
		port, ok := collection[self.ID]
		lock.Unlock()
		if !ok {
			return nil, errors.New("could not find starting port")
		}

		// check for any connections
		if len(port.Routes) == 0 {
			log.Tracef("spn/navigator: port has no routes, finding nearest port")
			m := NewMap(port, collection, lock)
			var ips []net.IP
			if port.Hub.Info.IPv4 != nil {
				ips = append(ips, port.Hub.Info.IPv4)
			}
			if port.Hub.Info.IPv6 != nil {
				ips = append(ips, port.Hub.Info.IPv6)
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
			return col.All[0].Port.Hub, nil
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
		var candidate *hub.Hub
		candidateDistance := 0
		for port, distance := range hops {
			if distance > minProximity && distance > candidateDistance {
				candidateDistance = distance
				candidate = port.Hub
			}
		}

		// debugging
		// for key, val := range hops {
		// 	fmt.Printf("%s: %d\n", key.Name(), val)
		// }

		return candidate, nil
	*/
}
