package captain

import (
	"time"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/modules/subsystems"
	"github.com/safing/spn/conf"

	_ "github.com/safing/spn/sluice"
)

var (
	module *modules.Module
)

func init() {
	module = modules.Register("captain", prep, start, nil, "base", "cabin", "docks", "navigator", "sluice")
	subsystems.Register(
		"spn",
		"SPN",
		"Safing Privacy Network",
		module,
		"config:spn/",
		&config.Option{
			Name:         "SPN Module",
			Key:          CfgOptionEnableSPNKey,
			Description:  "Start the Safing Privacy Network module. If turned off, the SPN is fully disabled on this device.",
			OptType:      config.OptTypeBool,
			DefaultValue: false,
			Annotations: config.Annotations{
				config.DisplayOrderAnnotation: cfgOptionEnableSPNOrder,
				config.CategoryAnnotation:     "General",
			},
		},
	)
}

func prep() error {
	initDockHooks()

	err := prepBootstrapHubFlag()
	if err != nil {
		return err
	}

	return prepConfig()
}

func start() error {
	// identity and piers
	if conf.PublicHub() {
		// load identity
		if err := loadPublicIdentity(); err != nil {
			return err
		}
		if err := prepPublicIdentityMgmt(); err != nil {
			return err
		}
		if err := startPierMgmt(); err != nil {
			return err
		}
	}

	// bootstrapping
	if err := processBootstrapHubFlag(); err != nil {
		return err
	}
	if err := processBootstrapFileFlag(); err != nil {
		return err
	}

	// network optimizer
	if conf.PublicHub() {
		module.NewTask("optimize network", optimizeNetwork).
			Repeat(1 * time.Minute).
			Schedule(time.Now().Add(15 * time.Second))
	}

	// client + primary hub manager
	if conf.Client() {
		module.StartServiceWorker("client manager", 0, clientManager)
	}

	return nil
}
