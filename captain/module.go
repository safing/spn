package captain

import (
	"fmt"
	"time"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/modules/subsystems"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/crew"
	"github.com/safing/spn/ships"
	"github.com/tevino/abool"

	_ "github.com/safing/spn/sluice"
)

var (
	module *modules.Module

	gossipQueryInitiated = abool.New()
)

func init() {
	module = modules.Register("captain", prep, start, nil, "base", "cabin", "docks", "crew", "navigator", "sluice", "netenv")
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
	maskingBytes, err := rng.Bytes(16)
	if err != nil {
		return fmt.Errorf("failed to get random bytes for masking: %w", err)
	}
	ships.EnableMasking(maskingBytes)

	// Initialize intel and other required resources.
	if err := loadRequiredResources(); err != nil {
		return err
	}
	if err := registerIntelUpdateHook(); err != nil {
		return err
	}
	if err := updateSPNIntel(module.Ctx, nil); err != nil {
		return err
	}

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

		// Enable connect operation.
		crew.EnableConnecting(publicIdentity.Hub)
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

	// client + home hub manager
	if conf.Client() {
		module.StartServiceWorker("client manager", 0, clientManager)
	}

	return nil
}
