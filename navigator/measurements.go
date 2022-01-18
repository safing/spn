package navigator

import (
	"context"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/terminal"
)

const NavigatorMeasurementTTL = 6 * time.Hour

func (m *Map) measureHubs(ctx context.Context, _ *modules.Task) error {
	if home, _ := m.GetHome(); home == nil {
		log.Debug("navigator: skipping measuring, no home hub set")
		return nil
	}

	var unknownErrCnt int
	matcher := m.DefaultOptions().Matcher(TransitHub)

	// Find first pin where any measurement has expired.
	for _, pin := range m.pinList(true) {
		var checkWithTTL time.Duration

		// Check if pin should be skipped.
		switch {
		case !matcher(pin):
			// Pin is disregarded.
			continue
		case pin.measurements == nil:
			// Measurements are not enabled for this Pin.
			continue
		case pin.measurements.Expired(NavigatorMeasurementTTL):
			// Measure.
			checkWithTTL = NavigatorMeasurementTTL
		case pin.HopDistance == 2 &&
			pin.measurements.Expired(docks.CraneMeasurementTTL):
			// Measure directly connected Hubs earlier.
			checkWithTTL = docks.CraneMeasurementTTL
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
