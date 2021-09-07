package navigator

import (
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
)

var (
	Main = NewMap("main")
)

// Map represent a collection of Pins and their relationship and status.
type Map struct {
	sync.RWMutex
	Name string

	All   map[string]*Pin
	Intel *hub.Intel

	Home *Pin
}

// NewMap returns a new and empty Map.
func NewMap(name string) *Map {
	return &Map{
		Name: name,
		All:  make(map[string]*Pin),
	}
}

// SetHome sets the given Hub as the new home.
func (m *Map) SetHome(id string) (ok bool) {
	m.Lock()
	defer m.Unlock()

	// Get pin from map.
	newHome, ok := m.All[id]
	if !ok {
		return false
	}

	// Remove home hub state from all pins.
	for _, pin := range m.All {
		pin.removeStates(StateIsHomeHub)
	}

	// Set pin as home.
	m.Home = newHome
	m.Home.addStates(StateIsHomeHub)

	// Recalculate reachable.
	err := m.recalculateReachableHubs()
	if err != nil {
		log.Warningf("spn/navigator: failed to recalculate reachable hubs: %s", err)
	}

	return true
}

// isEmpty returns whether the Map is regarded as empty.
func (m *Map) isEmpty() bool {
	// We also regard a map with only one entry to be empty, as this will be the
	// case for Hubs, which will have their own entry in the Map.
	return len(m.All) <= 1
}

/*
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

	m.Lock()
	defer m.Unlock()

	for _, port := range m.All {

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
*/
