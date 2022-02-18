package navigator

import (
	"testing"
)

func TestFindNearest(t *testing.T) {
	t.Parallel()

	// Create map and lock faking in order to guarantee reproducability of faked data.
	m := getDefaultTestMap()
	fakeLock.Lock()
	defer fakeLock.Unlock()

	for i := 0; i < 100; i++ {
		// Create a random destination address
		ip4, loc4 := createGoodIP(true)

		nbPins, err := m.findNearestPins(loc4, nil, m.DefaultOptions().Matcher(DestinationHub, m.intel), 10)
		if err != nil {
			t.Error(err)
		} else {
			t.Logf("Pins near %s: %s", ip4, nbPins)
		}
	}

	for i := 0; i < 100; i++ {
		// Create a random destination address
		ip6, loc6 := createGoodIP(true)

		nbPins, err := m.findNearestPins(nil, loc6, m.DefaultOptions().Matcher(DestinationHub, m.intel), 10)
		if err != nil {
			t.Error(err)
		} else {
			t.Logf("Pins near %s: %s", ip6, nbPins)
		}
	}
}

/*
TODO: Find a way to quickly generate good geoip data on the fly, as we don't want to measure IP address generation, but only finding the nearest pins.

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
*/

func findFakeHomeHub(m *Map) {
	// Create fake IP address.
	_, loc4 := createGoodIP(true)
	_, loc6 := createGoodIP(false)

	nbPins, err := m.findNearestPins(loc4, loc6, m.defaultOptions().Matcher(HomeHub, m.intel), 10)
	if err != nil {
		panic(err)
	}
	if len(nbPins.pins) == 0 {
		panic("could not find a Home Hub")
	}

	// Set Home.
	m.home = nbPins.pins[0].pin

	// Recalculate reachability.
	if err := m.recalculateReachableHubs(); err != nil {
		panic(err)
	}
}

func TestNearbyPinsCleaning(t *testing.T) {
	t.Parallel()

	testCleaning(t, []float32{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}, 5)
	testCleaning(t, []float32{10, 11, 12, 13, 14, 15, 70, 80, 90, 100}, 4)
	testCleaning(t, []float32{10, 11, 12, 13, 14, 15, 16, 80, 90, 100}, 3)
	testCleaning(t, []float32{10, 11, 12, 13, 14, 15, 16, 17, 90, 100}, 3)
}

func testCleaning(t *testing.T, proximities []float32, expectedLeftOver int) {
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
