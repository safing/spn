package navigator

import "time"

const (
	nearestPinsMaxProximityDifference = 50
	nearestPinsMinimum                = 3
)

// CalculateLaneCost calculates the cost of using a Lane based on the given
// Lane latency and capacity.
func CalculateLaneCost(latency time.Duration, capacity int) (cost float32) {
	// - One point for every ms in latency (linear)
	cost += float32(latency) / float32(time.Millisecond)

	switch {
	case capacity < cap1Mbit:
		// - Between 1000 and 10000 points for ranges below 1Mbit/s
		cost += 1000 + 9000*((cap1Mbit-float32(capacity))/cap1Mbit)
	case capacity < cap10Mbit:
		// - Between 100 and 1000 points for ranges below 10Mbit/s
		cost += 100 + 900*((cap10Mbit-float32(capacity))/cap10Mbit)
	case capacity < cap100Mbit:
		// - Between 20 and 100 points for ranges below 100Mbit/s
		cost += 20 + 80*((cap100Mbit-float32(capacity))/cap100Mbit)
	case capacity < cap1Gbit:
		// - Between 5 and 20 points for ranges below 1Gbit/s
		cost += 5 + 15*((cap1Gbit-float32(capacity))/cap1Gbit)
	case capacity < cap10Gbit:
		// - Between 0 and 5 points for ranges below 10Gbit/s
		cost += 5 * ((cap10Gbit - float32(capacity)) / cap10Gbit)
	}

	return cost
}

// CalculateHubCost calculates the cost of using a Hub based on the given Hub load.
func CalculateHubCost(load int) (cost float32) {
	switch {
	case load >= 100:
		return 1000
	case load >= 95:
		return 100
	case load >= 80:
		return 50
	default:
		return 10
	}
}

// CalculateDestinationCost calculates the cost of a destination hub to a
// destination server based on the given proximity.
func CalculateDestinationCost(proximity float32) (cost float32) {
	// Invert from proximity (0-100) to get a distance value.
	distance := 100 - proximity

	// Take the distance to the power of two and then divide by ten in order to
	// make high distances exponentially more expensive.
	return float32(distance*distance) / 10
}
