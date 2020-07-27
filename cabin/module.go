package cabin

import (
	"github.com/safing/portbase/modules"
)

var (
	module *modules.Module
)

func init() {
	modules.Register("cabin", prep, nil, nil, "config")
}

func prep() error {
	if err := initProvidedExchKeySchemes(); err != nil {
		return err
	}

	return prepConfig()
}
