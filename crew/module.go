package crew

import (
	"github.com/safing/portbase/modules"
)

var module *modules.Module

func init() {
	module = modules.Register("crew", nil, start, nil, "navigator", "intel", "cabin")
}

func start() error {
	return registerMetrics()
}
