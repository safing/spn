package terminal

import (
	"sync"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/api"
	"github.com/safing/portbase/metrics"
)

var metricsRegistered = abool.New()

func registerMetrics() (err error) {
	// Only register metrics once.
	if !metricsRegistered.SetToIf(false, true) {
		return nil
	}

	_, err = metrics.NewGauge(
		"spn/scheduling/unit/slotpace/max",
		nil,
		getMaxLeveledSlotPace,
		&metrics.Options{
			Name:       "SPN Scheduling Max Leveled Slot Pace",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	_, err = metrics.NewGauge(
		"spn/scheduling/unit/slotpace/avg",
		nil,
		getAvgSlotPace,
		&metrics.Options{
			Name:       "SPN Scheduling Avg Slot Pace",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

var (
	nextMaxLeveledPaceReset     time.Time
	nextMaxLeveledPaceResetLock sync.Mutex

	nextAvgSlotPaceReset     time.Time
	nextAvgSlotPaceResetLock sync.Mutex
)

func getMaxLeveledSlotPace() float64 {
	value := float64(scheduler.GetMaxLeveledSlotPace())

	nextMaxLeveledPaceResetLock.Lock()
	defer nextMaxLeveledPaceResetLock.Unlock()

	if time.Now().After(nextMaxLeveledPaceReset) {
		nextMaxLeveledPaceReset = time.Now().Add(50 * time.Second)
		scheduler.ResetMaxLeveledSlotPace()
	}

	return value
}

func getAvgSlotPace() float64 {
	value := float64(scheduler.GetAvgSlotPace())

	nextAvgSlotPaceResetLock.Lock()
	defer nextAvgSlotPaceResetLock.Unlock()

	if time.Now().After(nextAvgSlotPaceReset) {
		nextAvgSlotPaceReset = time.Now().Add(50 * time.Second)
		scheduler.ResetAvgSlotPace()
	}

	return value
}
