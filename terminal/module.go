package terminal

import (
	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/rng"
)

var (
	module    *modules.Module
	rngFeeder *rng.Feeder = rng.NewFeeder()
)

func init() {
	module = modules.Register("terminal", nil, start, nil, "base")
}

func start() error {
	rngFeeder = rng.NewFeeder()
	return nil
}
