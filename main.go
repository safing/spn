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
)

func main() {
	// configure
	info.Set("SPN Hub", "0.2.0", "AGPLv3", true)
	conf.EnablePublicHub(true)
	conf.EnableClient(false)
	config.SetDefaultConfigOption(captain.CfgOptionEnableSPNKey, true)

	// start
	os.Exit(run.Run())
}
