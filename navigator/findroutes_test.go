package navigator

import (
	"net"
	"testing"

	"github.com/brianvoe/gofakeit"
)

func TestFindRoutes(t *testing.T) {
	// Create map and lock faking in order to guarantee reproducability of faked data.
	m := getDefaultTestMap()
	fakeLock.Lock()
	defer fakeLock.Unlock()

	for i := 0; i < 1; i++ {
		// Create a random destination address
		dstIP := createGoodIP(i%2 == 0)

		routes, err := m.FindRoutes(dstIP, m.DefaultOptions(), 10)
		switch {
		case err != nil:
			t.Error(err)
		case len(routes.All) == 0:
			t.Logf("No routes for %s", dstIP)
		default:
			t.Logf("Best route for %s: %s", dstIP, routes.All[0])
		}
	}
}

func BenchmarkFindRoutes(b *testing.B) {
	// Create map and lock faking in order to guarantee reproducability of faked data.
	m := getDefaultTestMap()
	fakeLock.Lock()
	defer fakeLock.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create a random destination address
		var dstIP net.IP
		if i%2 == 0 {
			dstIP = net.ParseIP(gofakeit.IPv4Address())
		} else {
			dstIP = net.ParseIP(gofakeit.IPv6Address())
		}

		routes, err := m.FindRoutes(dstIP, m.DefaultOptions(), 10)
		if err != nil {
			b.Error(err)
		} else {
			b.Logf("Best route for %s: %s", dstIP, routes.All[0])
		}
	}
}
