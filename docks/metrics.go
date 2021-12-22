package docks

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portbase/api"
	"github.com/safing/portbase/metrics"
	"github.com/tevino/abool"
)

var (
	totalIncomingTraffic *metrics.Counter
	totalOutgoingTraffic *metrics.Counter

	expandOpDurationHistogram    *metrics.Histogram
	expandOpRelayedDataHistogram *metrics.Histogram

	metricsRegistered = abool.New()
)

func registerMetrics() (err error) {
	// Only register metrics once.
	if !metricsRegistered.SetToIf(false, true) {
		return nil
	}

	// Global Traffic Stats.

	totalIncomingTraffic, err = metrics.NewCounter(
		"spn/traffic/in/bytes",
		nil,
		&metrics.Options{
			Name:       "SPN Total Incoming Traffic",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	totalOutgoingTraffic, err = metrics.NewCounter(
		"spn/traffic/out/bytes",
		nil,
		&metrics.Options{
			Name:       "SPN Total Outgoing Traffic",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	// Lane Stats.

	_, err = metrics.NewGauge(
		"spn/lanes/total",
		nil,
		getLaneCntStat,
		&metrics.Options{
			Name:       "SPN Lanes",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	_, err = metrics.NewGauge(
		"spn/lanes/latency/seconds/total",
		nil,
		getTotalLaneLatencyStat,
		&metrics.Options{
			Name:       "SPN Total Lane Latency",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	_, err = metrics.NewGauge(
		"spn/lanes/capacity/bytes/total",
		nil,
		getTotalLaneCapacityStat,
		&metrics.Options{
			Name:       "SPN Total Lane Capacity",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	// Expand Op Stats.

	_, err = metrics.NewGauge(
		"spn/op/expand/active/total",
		nil,
		getActiveExpandOpsStat,
		&metrics.Options{
			Name:       "SPN Active Expand Operations",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	expandOpDurationHistogram, err = metrics.NewHistogram(
		"spn/op/expand/duration/seconds",
		nil,
		&metrics.Options{
			Name:       "SPN Expand Operation Duration",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	expandOpRelayedDataHistogram, err = metrics.NewHistogram(
		"spn/op/expand/traffic/bytes",
		nil,
		&metrics.Options{
			Name:       "SPN Expand Operation Relayed Data",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	return err
}

func getLaneCntStat() (cnt float64) {
	cnt, _, _ = getLaneStats()
	return
}

func getTotalLaneLatencyStat() (latency float64) {
	_, latency, _ = getLaneStats()
	return
}

func getTotalLaneCapacityStat() (capacity float64) {
	_, _, capacity = getLaneStats()
	return
}

func getActiveExpandOpsStat() float64 {
	return float64(atomic.LoadInt64(activeExpandOps))
}

var (
	laneStatsTotal         float64
	laneStatsTotalLatency  float64
	laneStatsTotalCapacity float64
	laneStatsExpires       time.Time
	laneStatsLock          sync.Mutex
	laneStatsTTL           = 1 * time.Minute
)

func getLaneStats() (cnt, latency, capacity float64) {
	laneStatsLock.Lock()
	defer laneStatsLock.Unlock()

	// Return cache if still valid.
	if time.Now().Before(laneStatsExpires) {
		return laneStatsTotal, laneStatsTotalLatency, laneStatsTotalCapacity
	}

	// Refresh.
	laneStatsTotal = 0
	laneStatsTotalLatency = 0
	laneStatsTotalCapacity = 0
	for _, crane := range GetAllAssignedCranes() {
		// Get lane stats.
		laneLatency := crane.GetLaneLatency()
		if laneLatency == 0 {
			continue
		}
		laneCapacity := crane.GetLaneCapacity()
		if laneCapacity == 0 {
			continue
		}

		// Only count if all data is available.
		laneStatsTotal++
		// Convert to base unit: seconds.
		laneStatsTotalLatency += float64(laneLatency) / float64(time.Second)
		// Convert in base unit: bytes.
		laneStatsTotalCapacity += float64(laneCapacity) / 8
	}

	laneStatsExpires = time.Now().Add(laneStatsTTL)
	return laneStatsTotal, laneStatsTotalLatency, laneStatsTotalCapacity
}
