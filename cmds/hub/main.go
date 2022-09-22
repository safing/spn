package main

import (
	"os"

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
	info.Set("SPN Hub", "0.5.0", "AGPLv3", true)

	// Configure metrics.
	_ = metrics.SetNamespace("hub")

	// Configure SPN mode.
	conf.EnablePublicHub(true)
	conf.EnableClient(false)

	// Disable module management, as we want to start all modules.
	modules.DisableModuleManagement()

	// adapt portmaster updates module
	updates.UserAgent = "Hub"
	helper.IntelOnly()

	// start
	os.Exit(run.Run())
}
