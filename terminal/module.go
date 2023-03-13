package terminal

import (
	"flag"
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

	debugUnitScheduling bool
)

func init() {
	flag.BoolVar(&debugUnitScheduling, "debug-unit-scheduling", false, "enable debug logs of the SPN unit scheduler")

	module = modules.Register("terminal", nil, start, nil, "base")
}

func start() error {
	rngFeeder = rng.NewFeeder()

	scheduler = unit.NewScheduler(schedulerConfig())
	module.StartServiceWorker("msg unit scheduler", 0, scheduler.SlotScheduler)

	lockOpRegistry()

	// Debug unit leaks.
	if debugUnitScheduling {
		scheduler.StartDebugLog()
	}

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
	// Client Scheduler Config.
	if conf.Client() {
		return &unit.SchedulerConfig{
			MinSlotPace:             10,  // 1000pps - Choose a small starting pace for low end devices.
			WorkSlotPercentage:      0.9, // 90%
			SlotChangeRatePerStreak: 0.1, // 10%
		}
	}

	// Server Scheduler Config.
	return &unit.SchedulerConfig{
		MinSlotPace:             100,
		WorkSlotPercentage:      0.7,  // 70%
		SlotChangeRatePerStreak: 0.02, // 2%
	}
}
