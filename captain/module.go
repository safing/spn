package captain

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/safing/portbase/api"
	"github.com/safing/portbase/config"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/modules/subsystems"
	"github.com/safing/portbase/rng"
	"github.com/safing/portmaster/network/netutils"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/crew"
	"github.com/safing/spn/ships"
	_ "github.com/safing/spn/sluice"
)

var module *modules.Module

func init() {
	module = modules.Register("captain", prep, start, stop, "base", "cabin", "docks", "crew", "navigator", "sluice", "netenv")
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
	// Check if we can parse the bootstrap hub flag.
	if err := prepBootstrapHubFlag(); err != nil {
		return err
	}

	// Register SPN status provider.
	if err := registerSPNStatusProvider(); err != nil {
		return err
	}

	if conf.PublicHub() {
		// Register API authenticator.
		if err := api.SetAuthenticator(apiAuthenticator); err != nil {
			return err
		}
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
		log.Errorf("spn/captain: failed to update SPN intel: %s", err)
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

	// Subscribe to updates of cranes.
	startDockHooks()

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

func stop() error {
	// Reset intel resource so that it is loaded again when starting.
	resetSPNIntel()

	// Unregister crane update hook.
	stopDockHooks()

	// Send shutdown status message.
	if conf.PublicHub() {
		publishShutdownStatus()
	}

	return nil
}

// apiAuthenticator grants User permissions for local API requests.
func apiAuthenticator(r *http.Request, s *http.Server) (*api.AuthToken, error) {
	// Get remote IP.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to split host/port: %w", err)
	}
	remoteIP := net.ParseIP(host)
	if remoteIP == nil {
		return nil, fmt.Errorf("failed to parse remote address %s", host)
	}

	if !netutils.GetIPScope(remoteIP).IsLocalhost() {
		return nil, api.ErrAPIAccessDeniedMessage
	}

	return &api.AuthToken{
		Read:  api.PermitUser,
		Write: api.PermitUser,
	}, nil
}
