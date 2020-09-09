package navigator

/*
func GetHub(ID string) *hub.Hub {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()
	port, ok := publicPorts[ID]
	if !ok {
		return nil
	}
	return port.Hub
}

func GetPublicPort(portName string) *Pin {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()
	port, ok := publicPorts[portName]
	if !ok {
		return nil
	}
	return port
}

func GetRandomPort() *Pin {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()

	for _, port := range publicPorts {
		return port
	}
	return nil
}
*/

/*
// FindNearestPorts returns the nearest ports to a set of IP addresses.
func FindNearestPorts(ips []net.IP) (*ProximityCollection, error) {
	publicPortsLock.RLock()
	defer publicPortsLock.RUnlock()
	primaryPortLock.Lock()
	defer primaryPortLock.Unlock()

	m := NewMap(primaryPort, publicPorts, publicPortsLock.RLocker())
	return m.FindNearestPorts(ips)
}

func FindPathToPorts(ports []*Pin) ([]*Pin, error) {
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
*/
