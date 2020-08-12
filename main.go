package main

import (
	"os"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/info"
	"github.com/safing/portbase/run"
	"github.com/safing/spn/captain"
	"github.com/safing/spn/conf"

	// include packages here
	_ "github.com/safing/portmaster/core/base"
	"github.com/safing/portmaster/updates"
)

func main() {
	info.Set("SPN Hub", "0.2.1", "AGPLv3", true)

	// configure SPN
	conf.EnablePublicHub(true)
	conf.EnableClient(false)
	config.SetDefaultConfigOption(captain.CfgOptionEnableSPNKey, true)

	// adapt portmaster updates module
	updates.MandatoryUpdates = []string{}
	updates.UserAgent = "Hub"

	// start
	os.Exit(run.Run())
}
