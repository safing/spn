package cabin

import (
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/conf"
)

var (
	module *modules.Module
)

func init() {
	modules.Register("cabin", prep, nil, nil, "base")
}

func prep() error {
	if err := initProvidedExchKeySchemes(); err != nil {
		return err
	}

	if conf.PublicHub() {
		if err := prepPublicHubConfig(); err != nil {
			return err
		}
	}

	return nil
}
