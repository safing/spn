package crew

import (
	"sync/atomic"

	"github.com/safing/portbase/api"
	"github.com/safing/portbase/metrics"
	"github.com/tevino/abool"
)

var (
	connectOpDurationHistogram     *metrics.Histogram
	connectOpDownloadDataHistogram *metrics.Histogram
	connectOpUploadDataHistogram   *metrics.Histogram

	metricsRegistered = abool.New()
)

func registerMetrics() (err error) {
	// Only register metrics once.
	if !metricsRegistered.SetToIf(false, true) {
		return nil
	}

	// Connect Op Stats.

	_, err = metrics.NewGauge(
		"spn/op/connect/active/total",
		nil,
		getActiveConnectOpsStat,
		&metrics.Options{
			Name:       "SPN Active Connect Operations",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	connectOpDurationHistogram, err = metrics.NewHistogram(
		"spn/op/connect/duration/seconds",
		nil,
		&metrics.Options{
			Name:       "SPN Connect Operation Duration",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	connectOpDownloadDataHistogram, err = metrics.NewHistogram(
		"spn/op/connect/download/bytes",
		nil,
		&metrics.Options{
			Name:       "SPN Connect Operation Downloaded Data",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	connectOpUploadDataHistogram, err = metrics.NewHistogram(
		"spn/op/connect/upload/bytes",
		nil,
		&metrics.Options{
			Name:       "SPN Connect Operation Uploaded Data",
			Permission: api.PermitUser,
		},
	)
	if err != nil {
		return err
	}

	return err
}

func getActiveConnectOpsStat() float64 {
	return float64(atomic.LoadInt64(activeConnectOps))
}
