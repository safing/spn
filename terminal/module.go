package terminal

import (
	"time"

	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/unit"
)

var (
	module    *modules.Module
	rngFeeder *rng.Feeder = rng.NewFeeder()
)

func init() {
	module = modules.Register("terminal", nil, start, nil, "base")
}

func start() error {
	rngFeeder = rng.NewFeeder()

	scheduler = unit.NewScheduler(nil)
	module.StartServiceWorker("msg unit scheduler", 0, scheduler.SlotScheduler)

	lockOpRegistry()

	// Debug unit leaks.
	// Search for "Debug unit leaks" to find all other related lines.
	// scheduler.StartDebugLog()
}

var waitForever chan time.Time

// TimedOut returns a channel that triggers when the timeout is reached.
func TimedOut(timeout time.Duration) <-chan time.Time {
	if timeout == 0 {
		return waitForever
	}
	return time.After(timeout)
}
