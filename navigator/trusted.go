package navigator

import (
	"bytes"
	"sync"
)

// TODO: revamp, maybe move to bottle loading

var (
	trustedPorts     = make(map[string][]byte)
	trustedPortsLock sync.RWMutex
)

func UpdateTrustedPorts(list map[string][]byte) {
	trustedPortsLock.Lock()
	defer trustedPortsLock.Unlock()
	trustedPorts = list
}

func IsPortTrusted(port *Port) bool {
	trustedPortsLock.RLock()
	defer trustedPortsLock.RUnlock()
	trustedPortID, ok := trustedPorts[port.Name()]
	if ok && bytes.Equal(port.Bottle.PortID, trustedPortID) {
		return true
	}
	return false
}
