package unit

import (
	"github.com/safing/portbase/modules"
)

var module *modules.Module

func init() {
	module = modules.Register("unit", nil, start, nil)
}

func start() error {
	module.StartServiceWorker("unit scheduler", 0, slotScheduler)
	return nil
}
