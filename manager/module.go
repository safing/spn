package manager

import (
	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/modules/subsystems"
	"github.com/safing/spn/mode"

	_ "github.com/safing/portmaster/netenv"
)

var (
	module *modules.Module
)

func init() {
	module = modules.Register("spn", nil, start, nil, "base", "netenv", "updates", "identitymgr")
	subsystems.Register(
		"spn",
		"SPN",
		"Safing Privacy Network",
		module,
		"",
		nil,
	)
}

func start() error {
	if mode.Node() {
		go handleRequests("tcp", ":17")
		go handleRequests("udp", ":17")
	}
	return nil
}
