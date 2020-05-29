package navigator

import (
	"errors"
	"net"
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/bottle"
)

var (
	publicPorts     = make(map[string]*Port)
	publicPortsLock sync.RWMutex

	localPorts     = make(map[string]*Port)
	localPortsLock sync.RWMutex

	primaryPort     *Port
	primaryPortLock sync.RWMutex
)

// SetPrimaryPort sets the primary port for map calculations
func SetPrimaryPort(port *Port) {
	primaryPortLock.Lock()
	defer primaryPortLock.Unlock()
	primaryPort = port
}

func GetPublicPort(portName string) *Port {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()
	port, ok := publicPorts[portName]
	if !ok {
		return nil
	}
	return port
}

// FindNearestPorts returns the nearest ports to a set of IP addresses.
func FindNearestPorts(ips []net.IP) (*ProximityCollection, error) {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()
	primaryPortLock.Lock()
	defer primaryPortLock.Unlock()

	m := NewMap(primaryPort, publicPorts, publicPortsLock.RLocker())
	return m.FindNearestPorts(ips)
}

func FindPathToPorts(ports []*Port) ([]*Port, error) {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()
	primaryPortLock.RLock()
	defer primaryPortLock.RUnlock()

	if primaryPort == nil {
		return nil, errors.New("no primary port available")
	}

	m := NewMap(primaryPort, publicPorts, publicPortsLock.RLocker())
	path, ok, err := m.FindShortestPath(true, ports...)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("port17/navigator: could not find path to requested ports")
	}
	return path, nil
}

func UpdatePublicBottle(newBottle *bottle.Bottle) {
	log.Infof("navigator: updating public bottle %s", newBottle.PortName)
	updateBottle(publicPorts, &publicPortsLock, newBottle)
}

func UpdateLocalBottle(newBottle *bottle.Bottle) {
	log.Infof("navigator: updating local bottle %s", newBottle.PortName)
	updateBottle(localPorts, &localPortsLock, newBottle)
}

func StartPublicReset() {
	publicPortsLock.Lock()
	publicPorts = make(map[string]*Port)
}

func FeedPublicBottle(newBottle *bottle.Bottle) {
	updateBottle(publicPorts, nil, newBottle)
}

func FinishPublicReset() {
	publicPortsLock.Unlock()
}

func StartLocalReset() {
	localPortsLock.Lock()
	localPorts = make(map[string]*Port)
}

func FinishLocalReset() {
	localPortsLock.Unlock()
}

func updateBottle(collection map[string]*Port, locker *sync.RWMutex, newBottle *bottle.Bottle) {
	// check authenticity of bottle
	// TODO: check authenticity of bottle

	if locker != nil {
		locker.Lock()
		defer locker.Unlock()
	}

	// create or update Port
	port, ok := collection[newBottle.PortName]
	if ok {
		// update Port
		port.Lock()
		port.Bottle = newBottle
		port.Unlock()
		port.CheckLocation()
	} else {
		// create new Port
		port = NewPort(newBottle)
		collection[newBottle.PortName] = port
	}

	// update Routes

	// add
	for _, route := range newBottle.Connections {
		newPort, ok := collection[route.PortName]
		if ok {
			port.AddRoute(newPort, route.Cost)
		}
	}

	// delete
	if len(port.Routes) > len(newBottle.Connections) {
		for _, route := range port.Routes {
			// look if current route is still present in bottle
			found := false
			for _, bottleConn := range newBottle.Connections {
				if route.Port.Name() == bottleConn.PortName {
					found = true
					break
				}
			}
			// if not, remove
			if !found {
				port.RemoveRoute(route.Port)
			}
		}
	}
}
