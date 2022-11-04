package main

import (
	"os"
	"runtime"

	"github.com/safing/portbase/info"
	"github.com/safing/portbase/metrics"
	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/run"
	_ "github.com/safing/portmaster/core/base"
	"github.com/safing/portmaster/updates"
	"github.com/safing/portmaster/updates/helper"
	_ "github.com/safing/spn/captain"
	"github.com/safing/spn/conf"
)

func main() {
	info.Set("SPN Hub", "0.5.1", "AGPLv3", true)

	// Configure metrics.
	_ = metrics.SetNamespace("hub")

	// Configure SPN mode.
	conf.EnablePublicHub(true)
	conf.EnableClient(false)

	// Disable module management, as we want to start all modules.
	modules.DisableModuleManagement()

	// Configure microtask threshold.
	// Scale with CPU/GOMAXPROCS count, but keep a baseline and minimum:
	// CPUs -> MicroTasks
	//    0 ->  8 (increased to minimum)
	//    1 ->  8 (increased to minimum)
	//    2 ->  8
	//    3 -> 10
	//    4 -> 12
	//    8 -> 20
	//   16 -> 36
	//
	// Start with number of GOMAXPROCS.
	microTasksThreshold := runtime.GOMAXPROCS(0) * 2
	// Use at least 4 microtasks based on GOMAXPROCS.
	if microTasksThreshold < 4 {
		microTasksThreshold = 4
	}
	// Add a 4 microtask baseline.
	microTasksThreshold += 4
	// Set threshold.
	modules.SetMaxConcurrentMicroTasks(microTasksThreshold)

	// adapt portmaster updates module
	updates.UserAgent = "Hub"
	helper.IntelOnly()

	// start
	os.Exit(run.Run())
}
