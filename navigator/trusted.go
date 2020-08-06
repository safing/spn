package navigator

import (
	"sync"
)

// TODO: revamp, maybe move to bottle loading

var (
	trustedPorts     = make(map[string]struct{})
	trustedPortsLock sync.RWMutex
)

func UpdateTrustedPorts(list map[string]struct{}) {
	trustedPortsLock.Lock()
	defer trustedPortsLock.Unlock()
	trustedPorts = list
}

func IsPortTrusted(port *Port) bool {
	trustedPortsLock.RLock()
	defer trustedPortsLock.RUnlock()
	_, ok := trustedPorts[port.Hub.ID]
	return ok
}
