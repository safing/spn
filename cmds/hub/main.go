package main

import (
	"os"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/info"
	"github.com/safing/portbase/metrics"
	"github.com/safing/portbase/run"
	"github.com/safing/spn/captain"
	"github.com/safing/spn/conf"

	// include packages here
	_ "github.com/safing/portmaster/core/base"
	"github.com/safing/portmaster/updates"
	"github.com/safing/portmaster/updates/helper"
)

func main() {
	info.Set("SPN Hub", "0.3.15", "AGPLv3", true)

	// Configure metrics.
	metrics.SetNamespace("hub")

	// configure SPN
	conf.EnablePublicHub(true)
	conf.EnableClient(false)
	config.SetDefaultConfigOption(captain.CfgOptionEnableSPNKey, true)

	// adapt portmaster updates module
	updates.UserAgent = "Hub"
	helper.IntelOnly()

	// start
	os.Exit(run.Run())
}
