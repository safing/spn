package main

import (
	"os"

	"github.com/safing/portbase/info"
	"github.com/safing/portbase/run"
	"github.com/safing/spn/mode"

	// include packages here
	_ "github.com/safing/portbase/modules/subsystems"
	_ "github.com/safing/portmaster/core/base"
	_ "github.com/safing/portmaster/network"
	_ "github.com/safing/spn/manager"
)

func main() {
	mode.SetNode(true)
	info.Set("SPN Node", "0.2.0", "AGPLv3", true)
	os.Exit(run.Run())
}
