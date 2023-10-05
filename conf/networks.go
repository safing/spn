package conf

import (
	"net"
	"sync"

	"github.com/tevino/abool"
)

var (
	hubHasV4 = abool.New()
	hubHasV6 = abool.New()
)

// SetHubNetworks sets the available IP networks on the Hub.
func SetHubNetworks(v4, v6 bool) {
	hubHasV4.SetTo(v4)
	hubHasV6.SetTo(v6)
}

// HubHasIPv4 returns whether the Hub has IPv4 support.
func HubHasIPv4() bool {
	return hubHasV4.IsSet()
}

// HubHasIPv6 returns whether the Hub has IPv6 support.
func HubHasIPv6() bool {
	return hubHasV6.IsSet()
}

var (
	connectIPv4   net.IP
	connectIPv6   net.IP
	connectIPLock sync.Mutex
)

// SetConnectAddr sets the preferred connect (bind) addresses.
func SetConnectAddr(ip4, ip6 net.IP) {
	connectIPLock.Lock()
	defer connectIPLock.Unlock()

	connectIPv4 = ip4
	connectIPv6 = ip6
}

// GetConnectAddr returns an address with the preferred connect (bind)
// addresses for the given dial network.
// The dial network must have a suffix specify the IP version.
func GetConnectAddr(dialNetwork string) net.Addr {
	connectIPLock.Lock()
	defer connectIPLock.Unlock()

	switch dialNetwork {
	case "ip4":
		if connectIPv4 != nil {
			return &net.IPAddr{IP: connectIPv4}
		}
	case "ip6":
		if connectIPv6 != nil {
			return &net.IPAddr{IP: connectIPv6}
		}
	case "tcp4":
		if connectIPv4 != nil {
			return &net.TCPAddr{IP: connectIPv4}
		}
	case "tcp6":
		if connectIPv6 != nil {
			return &net.TCPAddr{IP: connectIPv6}
		}
	case "udp4":
		if connectIPv4 != nil {
			return &net.UDPAddr{IP: connectIPv4}
		}
	case "udp6":
		if connectIPv6 != nil {
			return &net.UDPAddr{IP: connectIPv6}
		}
	}

	return nil
}
