package navigator

import (
	"context"
	"sort"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/terminal"
)

const (
	NavigatorMeasurementTTLDefault    = 2 * time.Hour
	NavigatorMeasurementTTLByCostBase = 3 * time.Minute
	NavigatorMeasurementTTLByCostMin  = 2 * time.Hour
	NavigatorMeasurementTTLByCostMax  = 30 * time.Hour

	// With a base TTL of 3m, this leads to:
	// 20c     -> 1h -> raised to 2h.
	// 50c     -> 2h30m
	// 100c    -> 5h
	// 1000c   -> 50h -> capped to 30h.
)

func (m *Map) measureHubs(ctx context.Context, _ *modules.Task) error {
	if home, _ := m.GetHome(); home == nil {
		log.Debug("navigator: skipping measuring, no home hub set")
		return nil
	}

	var unknownErrCnt int
	matcher := m.DefaultOptions().Matcher(TransitHub)

	// Get list and sort in order to check near/low-cost hubs earlier.
	list := m.pinList(true)
	sort.Sort(sortByLowestMeasuredCost(list))

	// Find first pin where any measurement has expired.
	for _, pin := range list {
		// Check if measuring is enabled.
		if pin.measurements == nil {
			continue
		}

		// Check if Pin is regarded.
		if !matcher(pin) {
			continue
		}

		// Calculate dynamic TTL.
		var checkWithTTL time.Duration
		if pin.HopDistance == 2 { // Hub is directly connected.
			checkWithTTL = calculateMeasurementTTLByCost(
				pin.measurements.GetCalculatedCost(),
				docks.CraneMeasurementTTLByCostBase,
				docks.CraneMeasurementTTLByCostMin,
				docks.CraneMeasurementTTLByCostMax,
			)
		} else {
			checkWithTTL = calculateMeasurementTTLByCost(
				pin.measurements.GetCalculatedCost(),
				NavigatorMeasurementTTLByCostBase,
				NavigatorMeasurementTTLByCostMin,
				NavigatorMeasurementTTLByCostMax,
			)
		}

		// Check if we have measured the pin within the TTL.
		if !pin.measurements.Expired(checkWithTTL) {
			continue
		}

		// Measure connection.
		tErr := docks.MeasureHub(ctx, pin.Hub, checkWithTTL)

		// Independent of outcome, recalculate the cost.
		latency, _ := pin.measurements.GetLatency()
		capacity, _ := pin.measurements.GetCapacity()
		calculatedCost := CalculateLaneCost(latency, capacity)
		pin.measurements.SetCalculatedCost(calculatedCost)
		// Log result.
		log.Infof(
			"navigator: updated measurements for connection to %s: %s %.2fMbit/s %.2fc",
			pin.Hub,
			latency,
			float64(capacity)/1000000,
			calculatedCost,
		)

		switch {
		case tErr.IsOK():
			// All good, continue.

		case tErr.Is(terminal.ErrTryAgainLater):
			if tErr.IsExternal() {
				// Remote is measuring, just continue with next.
				log.Debugf("navigator: remote %s is measuring, continuing with next", pin.Hub)
			} else {
				// We are measuring, abort and restart measuring again later.
				log.Debugf("navigator: postponing measuring because we are currently engaged in measuring")
				return nil
			}

		default:
			log.Warningf("navigator: failed to measure connection to %s: %s", pin.Hub, tErr)
			unknownErrCnt++
			if unknownErrCnt >= 3 {
				log.Warningf("navigator: postponing measuring task because of multiple errors")
				return nil
			}
		}
	}

	return nil
}

func (m *Map) SaveMeasuredHubs() {
	m.RLock()
	defer m.RUnlock()

	for _, pin := range m.all {
		if !pin.measurements.IsPersisted() {
			if err := pin.Hub.Save(); err != nil {
				log.Warningf("navigator: failed to save Hub %s to persist measurements: %s", pin.Hub, err)
			}
		}
	}
}

func calculateMeasurementTTLByCost(cost float32, base, min, max time.Duration) time.Duration {
	calculated := time.Duration(cost) * base
	switch {
	case calculated < min:
		return min
	case calculated > max:
		return max
	default:
		return calculated
	}
}
