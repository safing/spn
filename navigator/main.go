package navigator

import (
	"errors"
	"net"
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
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

func GetHub(ID string) *hub.Hub {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()
	port, ok := publicPorts[ID]
	if !ok {
		return nil
	}
	return port.Hub
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

func GetRandomPort() *Port {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()

	for _, port := range publicPorts {
		return port
	}
	return nil
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

func UpdateHub(h *hub.Hub) {
	log.Infof("spn/navigator: updating Hub %s", h)
	updateHub(publicPorts, &publicPortsLock, h)
}

func RemovePublicHub(id string) {
	publicPortsLock.Lock()
	defer publicPortsLock.Unlock()
	delete(publicPorts, id)
}

func updateHub(collection map[string]*Port, locker *sync.RWMutex, h *hub.Hub) {
	if locker != nil {
		locker.Lock()
		defer locker.Unlock()
	}

	// create or update Port
	port, ok := collection[h.ID]
	if ok {
		// update Port
		port.Lock()
		port.Hub = h
		port.Unlock()
		port.CheckLocation()
	} else {
		// create new Port
		port = NewPort(h)
		collection[h.ID] = port
	}

	// update Routes
	if h.Status == nil {
		return
	}

	// add
	for _, route := range h.Status.Connections {
		newPort, ok := collection[route.ID]
		if ok {
			port.AddRoute(newPort, route.Latency)
		}
	}

	// delete
	if len(port.Routes) > len(h.Status.Connections) {
		for _, route := range port.Routes {
			// look if current route is still present in bottle
			found := false
			for _, conn := range h.Status.Connections {
				if route.Port.Name() == conn.ID {
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
