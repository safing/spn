package navigator

import (
	"math"
	"net"
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/intel/geoip"
)

// Dijkstra "#AI"
type Map struct {
	Collection     map[string]*Port
	CollectionLock sync.Locker
	PrimaryPort    *Port
	GoodEnough     int
	IgnoreAbove    int
	solutions      map[string]*Solution
}

func NewMap(primaryPort *Port, collection map[string]*Port, lock sync.Locker) *Map {
	return &Map{
		Collection:     collection,
		CollectionLock: lock,
		GoodEnough:     math.MaxInt32,
		IgnoreAbove:    math.MaxInt32,
		PrimaryPort:    primaryPort,
	}
}

// FindNearestPorts returns the nearest ports to a set of IP addresses.
func (m *Map) FindNearestPorts(ips []net.IP) (*ProximityCollection, error) {

	// TODO: also consider node load

	col := NewProximityCollection(10)

	// sort and get geoip for all destination IPs

	var ip4s []net.IP
	var ip4Locs []*geoip.Location

	var ip6s []net.IP
	var ip6Locs []*geoip.Location

	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {

			loc, err := geoip.GetLocation(ip)
			ip4s = append(ip4s, ip)
			if err != nil {
				log.Warningf("port17/navigator: failed to get location of destination IP %s: %s", ip, err)
				ip4Locs = append(ip4Locs, nil)
			} else {
				ip4Locs = append(ip4Locs, loc)
			}

		} else {

			loc, err := geoip.GetLocation(ip)
			ip6s = append(ip6s, ip)
			if err != nil {
				log.Warningf("port17/navigator: failed to get location of destination IP %s: %s", ip, err)
				ip6Locs = append(ip6Locs, nil)
			} else {
				ip6Locs = append(ip6Locs, loc)
			}

		}
	}

	geoMatch := true
	if len(ip4s) == 0 && len(ip6s) == 0 {
		// return nil, errors.New("could not get geolocation of any ip")
		geoMatch = false
	}

	// start searching for nearby ports

	m.CollectionLock.Lock()
	defer m.CollectionLock.Unlock()

	for _, port := range m.Collection {

		// exclude primary if given
		if m.PrimaryPort != nil && m.PrimaryPort.Name() == port.Name() {
			continue
		}

		if port.Hub.Info.IPv4 != nil {
			for i := 0; i < len(ip4s); i++ {
				proximity := 0
				if geoMatch {
					if ip4Locs[i] != nil {
						proximity = ip4Locs[i].EstimateNetworkProximity(port.Location4)
					}
				} else {
					proximity = geoip.PrimitiveNetworkProximity(port.Hub.Info.IPv4, ip4s[i], 4)
				}
				if proximity >= col.MinProximity {
					col.Add(&ProximityResult{
						IP:        ip4s[i],
						Port:      port,
						Proximity: proximity,
					})
				}
			}
		}

		if port.Hub.Info.IPv6 != nil {
			for i := 0; i < len(ip6s); i++ {
				proximity := 0
				if geoMatch {
					if ip6Locs[i] != nil {
						proximity = ip6Locs[i].EstimateNetworkProximity(port.Location6)
					}
				} else {
					proximity = geoip.PrimitiveNetworkProximity(port.Hub.Info.IPv6, ip6s[i], 6)
				}
				if proximity >= col.MinProximity {
					col.Add(&ProximityResult{
						IP:        ip6s[i],
						Port:      port,
						Proximity: proximity,
					})
				}
			}
		}

	}

	col.Clean()
	return col, nil
}

func (m *Map) Reset() {
	m.solutions = nil
}
