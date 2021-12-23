package navigator

import (
	"net"
	"testing"

	"github.com/brianvoe/gofakeit"
)

func TestFindNearest(t *testing.T) {
	// Create map and lock faking in order to guarantee reproducability of faked data.
	m := getDefaultTestMap()
	fakeLock.Lock()
	defer fakeLock.Unlock()

	for i := 0; i < 100; i++ {
		// Create a random destination address
		dstIP := createGoodIP(i%2 == 0)

		nbPins, err := m.findNearestPins(dstIP, m.DefaultOptions().Matcher(DestinationHub), 10)
		if err != nil {
			t.Error(err)
		} else {
			t.Logf("Pins near %s: %s", dstIP, nbPins)
		}
	}
}

func BenchmarkFindNearest(b *testing.B) {
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

		_, err := m.findNearestPins(dstIP, m.DefaultOptions().Matcher(DestinationHub), 10)
		if err != nil {
			b.Error(err)
		}
	}
}

func findFakeHomeHub(m *Map) {
	// Create fake IP address.
	var myIP net.IP
	if gofakeit.Number(0, 1) == 0 {
		myIP = net.ParseIP(gofakeit.IPv4Address())
	} else {
		myIP = net.ParseIP(gofakeit.IPv6Address())
	}
	if myIP == nil {
		panic("failed to set IP")
	}

	// Get nearest Hubs.
	nbPins, err := m.findNearestPins(myIP, m.defaultOptions().Matcher(HomeHub), 10)
	if err != nil {
		panic(err)
	}
	if len(nbPins.pins) == 0 {
		panic("could not find a Home Hub")
	}

	// Set Home.
	m.Home = nbPins.pins[0].pin

	// Recalculate reachability.
	m.recalculateReachableHubs()
}

func TestNearbyPinsCleaning(t *testing.T) {
	testCleaning(t, []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}, 5)
	testCleaning(t, []int{10, 11, 12, 13, 14, 15, 70, 80, 90, 100}, 4)
	testCleaning(t, []int{10, 11, 12, 13, 14, 15, 16, 80, 90, 100}, 3)
	testCleaning(t, []int{10, 11, 12, 13, 14, 15, 16, 17, 90, 100}, 3)
}

func testCleaning(t *testing.T, proximities []int, expectedLeftOver int) {
	t.Helper()

	nb := &nearbyPins{
		minPins:     3,
		maxPins:     5,
		cutOffLimit: 50,
	}

	// Simulate usage.
	for _, prox := range proximities {
		// Add to list.
		nb.add(nil, prox)

		// Clean once in a while.
		if len(nb.pins) > nb.maxPins {
			nb.clean()
		}
	}
	// Final clean.
	nb.clean()

	// Check results.
	t.Logf("result: %+v", nb.pins)
	if len(nb.pins) != expectedLeftOver {
		t.Fatalf("unexpected amount of left over pins: %+v", nb.pins)
	}
}
