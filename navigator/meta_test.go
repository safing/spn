package navigator

func buildTestNet() map[string]*Port {
	return make(map[string]*Port)
}

/*
TODO: fix these tests

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/safing/spn/bottle"
	"github.com/safing/spn/hub"
)

// This is the test network:
// Ports are described as [ID]c[Cost]
// Routes have the cost encoded in the middle of the line
//
// 12c5----4---11c2----4---10c4
//   |           |           |
//   4          13          27
//   |           |           |
//  7c6----3----8c3----7----9c1
//   |           |           |
//  18          29           3
//   |           |           |
//  6c1----1----4c3----3----3c2
//   |           |           |
//   5           3           3
//   |           |           |
//  5c2---100---1c1----3----2c3.1
//

func buildTestNet() map[string]*Port {
	b1 := createTestHub(1, 1000, "32.50.191.159", "2607:8700:101::1")
	b2 := createTestHub(2, 3100, "97.213.160.223", "2607:d400::1")
	b3 := createTestHub(3, 3000, "108.217.201.149", "2607:f018::1")
	b4 := createTestHub(4, 3000, "203.37.246.135", "2607:f140::1")
	b5 := createTestHub(5, 2000, "54.41.62.147", "2607:f1c0::1")
	b6 := createTestHub(6, 1000, "237.63.9.41", "2a00:14a0::1")
	b7 := createTestHub(7, 6000, "63.111.239.90", "2a00:1638::1")
	b8 := createTestHub(8, 3000, "55.145.64.55", "2a02:2878::1")
	b9 := createTestHub(9, 1000, "186.203.160.171", "2a02:28e8::1")
	b10 := createTestHub(10, 4000, "233.18.172.63", "2a02:29a0::1")
	b11 := createTestHub(11, 2000, "229.191.250.251", "2001:df3:6b00::1")
	b12 := createTestHub(12, 5000, "1.48.221.208", "2001:df4:3500::1")

	connect(b1, b2, 3000)
	connect(b1, b4, 3000)
	connect(b1, b5, 100000)
	connect(b2, b3, 3000)
	connect(b3, b4, 3000)
	connect(b3, b9, 3000)
	connect(b4, b6, 3000)
	connect(b4, b8, 29000)
	connect(b5, b6, 5000)
	connect(b6, b7, 18000)
	connect(b7, b8, 3000)
	connect(b7, b12, 4000)
	connect(b8, b9, 7000)
	connect(b8, b11, 13000)
	connect(b9, b10, 27000)
	connect(b10, b11, 4000)
	connect(b11, b12, 4000)

	collection := makePorts(b1, b2, b3, b4, b5, b6, b7, b8, b9, b10, b11, b12)

	return collection
}

func setCost(h *hub.Hub, cost int) {
	// we have no performance requirements here, so make it agnostic to used function
	port := NewPort(h)
	for i := 0; i < 1000; i++ {
		port.Load = i
		c := port.Cost()
		if c > cost {
			b.Load = i
			return
		}
	}
}

func connect(first, second *bottle.Bottle, cost int) {
	first.AddConnection(second, cost)
	second.AddConnection(first, cost)
}

func makePorts(bottles ...*bottle.Bottle) map[string]*Port {
	collection := make(map[string]*Port)
	var locker sync.RWMutex

	for _, b := range bottles {
		updateBottle(collection, &locker, b)
	}
	return collection
}

func createTestBottle(id uint8, cost int, ip4, ip6 string) *bottle.Bottle {
	new := &hub.Hub{
		ID: fmt.Sprintf("%d", id),
		IPv4:     net.ParseIP(ip4),
		IPv6:     net.ParseIP(ip6),
	}
	setCost(new, cost)
	return new
}

func comparePath(result []*Port, destinations, expected []uint8, considerActiveRoutes bool) error {
	if len(result) != len(expected) {
		return fmt.Errorf("path to %s%s: length mismatch: was %s, should be %s", printDestIDs(destinations), considerRoutesToString(considerActiveRoutes), printPortPath(result), printIDPath(expected))
	}
	for i := 0; i < len(result); i++ {
		if result[i].Name() != fmt.Sprintf("%d", expected[i]) {
			return fmt.Errorf("path to %s%s: path mismatch: was %s, should be %s", printDestIDs(destinations), considerRoutesToString(considerActiveRoutes), printPortPath(result), printIDPath(expected))
		}
	}
	return nil
}

func considerRoutesToString(considerActiveRoutes bool) string {
	if considerActiveRoutes {
		return " (w/AR)"
	}
	return ""
}

func printIDPath(path []uint8) string {
	s := ""
	for _, entry := range path {
		s += fmt.Sprintf("%d-", entry)
	}
	return strings.Trim(s, "-")
}

func printDestIDs(IDs []uint8) string {
	s := ""
	for _, entry := range IDs {
		s += fmt.Sprintf("%d,", entry)
	}
	return strings.Trim(s, ",")
}
*/
