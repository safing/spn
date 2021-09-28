package conf

import "github.com/tevino/abool"

var (
	hubHasV4 = abool.New()
	hubHasV6 = abool.New()
)

func SetHubNetworks(v4, v6 bool) {
	hubHasV4.SetTo(v4)
	hubHasV6.SetTo(v6)
}

func HubHasIPv4() bool {
	return hubHasV4.IsSet()
}

func HubHasIPv6() bool {
	return hubHasV6.IsSet()
}
