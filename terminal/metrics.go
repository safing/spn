package terminal

import (
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
		"spn/scheduling/unit/currentunitid",
		nil,
		floatify(scheduler.GetCurrentUnitID),
		&metrics.Options{
			Name:       "SPN Scheduling Current Unit ID",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	_, err = metrics.NewGauge(
		"spn/scheduling/unit/slotpace",
		nil,
		floatify(scheduler.GetSlotPace),
		&metrics.Options{
			Name:       "SPN Scheduling Current Slot Pace",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	_, err = metrics.NewGauge(
		"spn/scheduling/unit/clearanceupto",
		nil,
		floatify(scheduler.GetClearanceUpTo),
		&metrics.Options{
			Name:       "SPN Scheduling Clearance Up to Unit ID",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	_, err = metrics.NewGauge(
		"spn/scheduling/unit/finished",
		nil,
		floatify(scheduler.GetFinished),
		&metrics.Options{
			Name:       "SPN Scheduling Finished Units",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func floatify(fn func() int64) func() float64 {
	return func() float64 {
		return float64(fn())
	}
}
