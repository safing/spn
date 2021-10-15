package crew

import (
	"github.com/safing/portbase/modules"
)

var module *modules.Module

func init() {
	module = modules.Register("crew", nil, nil, nil, "navigator", "intel", "cabin")
}
