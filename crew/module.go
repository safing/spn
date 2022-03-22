package crew

import (
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/terminal"
)

var module *modules.Module

func init() {
	module = modules.Register("crew", nil, start, nil, "navigator", "intel", "cabin")
}

func start() error {
	return registerMetrics()
}

var connectErrors = make(chan *terminal.Error, 10)

func reportConnectError(tErr *terminal.Error) {
	select {
	case connectErrors <- tErr:
	default:
	}
}

// ConnectErrors returns errors of connect operations.
// It only has a small and shared buffer and may only be used for indications,
// not for full monitoring.
func ConnectErrors() <-chan *terminal.Error {
	return connectErrors
}
