package main

import (
	"os"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/info"
	"github.com/safing/portbase/metrics"
	"github.com/safing/portbase/run"
	_ "github.com/safing/portmaster/core/base"
	"github.com/safing/portmaster/updates"
	"github.com/safing/portmaster/updates/helper"
	"github.com/safing/spn/captain"
	"github.com/safing/spn/conf"
)

func main() {
	info.Set("SPN Hub", "0.4.6", "AGPLv3", true)

	// Configure metrics.
	_ = metrics.SetNamespace("hub")

	// configure SPN
	conf.EnablePublicHub(true)
	conf.EnableClient(false)
	_ = config.SetDefaultConfigOption(captain.CfgOptionEnableSPNKey, true)

	// adapt portmaster updates module
	updates.UserAgent = "Hub"
	helper.IntelOnly()

	// start
	os.Exit(run.Run())
}
