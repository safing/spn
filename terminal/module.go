package terminal

import (
	"time"

	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/unit"
)

var (
	module    *modules.Module
	rngFeeder *rng.Feeder = rng.NewFeeder()

	scheduler *unit.Scheduler
)

func init() {
	module = modules.Register("terminal", nil, start, nil, "base")
}

func start() error {
	rngFeeder = rng.NewFeeder()

	scheduler = unit.NewScheduler(schedulerConfig())
	module.StartServiceWorker("msg unit scheduler", 0, scheduler.SlotScheduler)

	lockOpRegistry()

	// Debug unit leaks.
	// Search for "Debug unit leaks" to find all other related lines.
	// scheduler.StartDebugLog()

	return registerMetrics()
}

var waitForever chan time.Time

// TimedOut returns a channel that triggers when the timeout is reached.
func TimedOut(timeout time.Duration) <-chan time.Time {
	if timeout == 0 {
		return waitForever
	}
	return time.After(timeout)
}

// StopScheduler stops the unit scheduler.
func StopScheduler() {
	if scheduler != nil {
		scheduler.Stop()
	}
}

func schedulerConfig() *unit.SchedulerConfig {
	if conf.Client() {
		return &unit.SchedulerConfig{
			MinSlotPace:             10, // 1000pps - Choose a small starting pace for low end devices.
			AdjustFractionPerStreak: 10, // 10% - Adapt quicker, as clients will often be at min pace.
		}
	}

	return nil
}
