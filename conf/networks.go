package conf

import "github.com/tevino/abool"

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
