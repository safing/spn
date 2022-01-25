package ships

import (
	"net"
	"sync"

	"github.com/safing/spn/hub"
)

var (
	virtNetLock   sync.Mutex
	virtNetConfig *hub.VirtualNetworkConfig
)

func SetVirtualNetworkConfig(config *hub.VirtualNetworkConfig) {
	virtNetLock.Lock()
	defer virtNetLock.Unlock()

	virtNetConfig = config
}

func GetVirtualNetworkConfig() *hub.VirtualNetworkConfig {
	virtNetLock.Lock()
	defer virtNetLock.Unlock()

	return virtNetConfig
}

func GetVirtualNetworkAddress(dstHubID string) net.IP {
	virtNetLock.Lock()
	defer virtNetLock.Unlock()

	// Check if we have a virtual network config.
	if virtNetConfig == nil {
		return nil
	}

	// Return mapping for given Hub ID.
	return virtNetConfig.Mapping[dstHubID]
}
