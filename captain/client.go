package captain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/notifications"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/portmaster/network/netutils"
	"github.com/safing/spn/access"
	"github.com/safing/spn/crew"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/navigator"
)

var (
	bootstrapped = abool.New()
	ready        = abool.New()

	spnLoginButton = notifications.Action{
		Text:    "Login",
		Type:    notifications.ActionTypeOpenPage,
		Payload: "spn",
	}
	spnPageButton = notifications.Action{
		Text:    "Open",
		Type:    notifications.ActionTypeOpenPage,
		Payload: "spn",
	}
	spnOpenAccountWeb = notifications.Action{
		Text:    "Open account.safing.io",
		Type:    notifications.ActionTypeOpenURL,
		Payload: "https://account.safing.io",
	}
)

// ClientBootstrapping signifies if the SPN is currently bootstrapping and
// requires normal connectivity for download assets.
func ClientBootstrapping() bool {
	return bootstrapped.IsNotSet()
}

// ClientReady signifies if the SPN client is fully ready to handle connections.
func ClientReady() bool {
	return ready.IsSet()
}

type (
	clientComponentFunc   func(ctx context.Context) clientComponentResult
	clientComponentResult uint8
)

const (
	clientResultOk        clientComponentResult = iota // Continue and clean module status.
	clientResultRetry                                  // Go back to start of current step, don't clear module status.
	clientResultReconnect                              // Stop current connection and start from zero.
	clientResultShutdown                               // SPN Module is shutting down.
)

var (
	clientNetworkChangedFlag           = netenv.GetNetworkChangedFlag()
	clientIneligibleAccountUpdateDelay = 1 * time.Minute
	clientRetryConnectBackoffDuration  = 5 * time.Second
	clientInitialHealthCheckDelay      = 10 * time.Second
	clientHealthCheckTickDuration      = 1 * time.Minute
	clientHealthCheckTimeout           = 5 * time.Second

	clientHealthCheckTrigger = make(chan struct{}, 1)
)

func triggerClientHealthCheck() {
	select {
	case clientHealthCheckTrigger <- struct{}{}:
	default:
	}
}

func clientManager(ctx context.Context) error {
	defer func() {
		ready.UnSet()
		bootstrapped.UnSet()
		netenv.ConnectedToSPN.UnSet()
		resetSPNStatus(StatusDisabled, true)
		module.Resolve("")
		clientStopHomeHub(ctx)
	}()

	module.Hint(
		"spn:establishing-home-hub",
		"Connecting to SPN...",
		"Connecting to the SPN network is in progress.",
	)

	// TODO: When we are starting and the SPN module is faster online than the
	// nameserver, then updating the account will fail as the DNS query is
	// redirected to a closed port.
	// We also can't add the nameserver as a module dependency, as the nameserver
	// is not part of the server.
	time.Sleep(1 * time.Second)

reconnect:
	for {
		if ready.SetToIf(true, false) {
			netenv.ConnectedToSPN.UnSet()
			log.Info("spn/captain: client not ready")
		}
		resetSPNStatus(StatusConnecting, true)

		// Check everything and connect to the SPN.
		for _, clientFunc := range []clientComponentFunc{
			clientStopHomeHub,
			clientCheckNetworkReady,
			clientCheckAccountAndTokens,
			clientConnectToHomeHub,
			clientSetActiveConnectionStatus,
		} {
			switch clientFunc(ctx) {
			case clientResultOk:
				// Continue
			case clientResultRetry, clientResultReconnect:
				// Wait for a short time to not loop too quickly.
				time.Sleep(clientRetryConnectBackoffDuration)
				continue reconnect
			case clientResultShutdown:
				return nil
			}
		}

		log.Info("spn/captain: client is ready")
		ready.Set()
		netenv.ConnectedToSPN.Set()

		// Back off before starting initial health checks.
		select {
		case <-time.After(clientInitialHealthCheckDelay):
		case <-ctx.Done():
			return nil
		}

		for {
			// Check health of the current SPN connection and monitor the user status.
		maintainers:
			for _, clientFunc := range []clientComponentFunc{
				clientCheckHomeHubConnection,
				clientCheckAccountAndTokens,
				clientSetActiveConnectionStatus,
			} {
				switch clientFunc(ctx) {
				case clientResultOk:
					// Continue
				case clientResultRetry:
					// Abort and wait for the next run.
					break maintainers
				case clientResultReconnect:
					continue reconnect
				case clientResultShutdown:
					return nil
				}
			}

			// Wait for signal to run maintenance again.
			select {
			case <-time.After(clientHealthCheckTickDuration):
			case <-clientHealthCheckTrigger:
			case <-crew.ConnectErrors():
			case <-clientNetworkChangedFlag.Signal():
				clientNetworkChangedFlag.Refresh()
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func clientCheckNetworkReady(ctx context.Context) clientComponentResult {
	// Check if we are online enough for connecting.
	switch netenv.GetOnlineStatus() { //nolint:exhaustive
	case netenv.StatusOffline,
		netenv.StatusLimited:
		select {
		case <-ctx.Done():
			return clientResultShutdown
		case <-time.After(1 * time.Second):
			return clientResultRetry
		}
	}

	return clientResultOk
}

func clientCheckAccountAndTokens(ctx context.Context) clientComponentResult {
	// Get SPN user.
	user, err := access.GetUser()
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		notifications.NotifyError(
			"spn:failed-to-get-user",
			"SPN Internal Error",
			`Please restart Portmaster.`,
			// TODO: Add restart button.
			// TODO: Use special UI restart action in order to reload UI on restart.
		).AttachToModule(module)
		resetSPNStatus(StatusFailed, true)
		log.Errorf("spn/captain: client internal error: %s", err)
		return clientResultReconnect
	}

	// Check if user is logged in.
	if user == nil || !user.IsLoggedIn() {
		notifications.NotifyWarn(
			"spn:not-logged-in",
			"SPN Login Required",
			`Please log in with your SPN account.`,
			spnLoginButton,
		).AttachToModule(module)
		resetSPNStatus(StatusFailed, true)
		log.Warningf("spn/captain: enabled but not logged in")
		return clientResultReconnect
	}

	// Check if user is eligible.
	if !user.MayUseTheSPN() {
		// Update user in case there was a change.
		// Only update here if we need to - there is an update task in the access
		// module for periodic updates.
		if time.Now().Add(-clientIneligibleAccountUpdateDelay).After(time.Unix(user.Meta().Modified, 0)) {
			_, _, err := access.UpdateUser()
			if err != nil {
				notifications.NotifyError(
					"spn:failed-to-update-user",
					"SPN Account Server Error",
					fmt.Sprintf(`The status of your SPN account could not be updated: %s`, err),
					spnPageButton,
				).AttachToModule(module)
				resetSPNStatus(StatusFailed, true)
				log.Errorf("spn/captain: failed to update ineligible account: %s", err)
				return clientResultReconnect
			}
		}

		// Check if user is eligible after a possible update.
		if !user.MayUseTheSPN() {
			notifications.NotifyError(
				"spn:subscription-inactive",
				"SPN Subscription Inactive",
				`Your account is currently not subscribed to the SPN. Follow the instructions on account.safing.io to get started.`,
				spnOpenAccountWeb,
				spnPageButton,
			).AttachToModule(module)
			resetSPNStatus(StatusFailed, true)
			return clientResultReconnect
		}
	}

	// Check if we have enough tokens.
	if access.ShouldRequest(access.ExpandAndConnectZones) {
		err := access.UpdateTokens()
		if err != nil {
			log.Errorf("spn/captain: failed to get tokens: %s", err)

			// There was an error updating the account.
			// Check if we have enough tokens to continue anyway.
			regular, _ := access.GetTokenAmount(access.ExpandAndConnectZones)
			if regular == 0 /* && fallback == 0 */ { // TODO: Add fallback token check when fallback was tested on servers.
				notifications.NotifyError(
					"spn:tokens-exhausted",
					"SPN Access Tokens Exhausted",
					`The Portmaster failed to get new access tokens to access the SPN. The Portmaster will automatically retry to get new access tokens.`,
					spnPageButton,
				).AttachToModule(module)
				resetSPNStatus(StatusFailed, false)
			}
			return clientResultRetry
		}
	}

	return clientResultOk
}

func clientStopHomeHub(ctx context.Context) clientComponentResult {
	// Don't use the context in this function, as it will likely be canceled
	// already and would disrupt any context usage in here.

	// Get crane connecting to home.
	home, _ := navigator.Main.GetHome()
	if home == nil {
		return clientResultOk
	}
	crane := docks.GetAssignedCrane(home.Hub.ID)
	if crane == nil {
		return clientResultOk
	}

	// Stop crane and all connected terminals.
	crane.Stop(nil)
	return clientResultOk
}

func clientConnectToHomeHub(ctx context.Context) clientComponentResult {
	err := establishHomeHub(ctx)
	if err != nil {
		log.Errorf("failed to establish connection to home hub: %s", err)
		resetSPNStatus(StatusFailed, true)

		switch {
		case errors.Is(err, ErrAllHomeHubsExcluded):
			notifications.NotifyError(
				"spn:all-home-hubs-excluded",
				"All Home Nodes Excluded",
				"Your current Home Node Rules exclude all available SPN Nodes. Please change your rules to allow for at least one available Home Node.",
				notifications.Action{
					Text: "Configure",
					Type: notifications.ActionTypeOpenSetting,
					Payload: &notifications.ActionTypeOpenSettingPayload{
						Key: CfgOptionHomeHubPolicyKey,
					},
				},
			).AttachToModule(module)

		default:
			notifications.NotifyWarn(
				"spn:home-hub-failure",
				"SPN Failed to Connect",
				fmt.Sprintf("Failed to connect to a home hub: %s. The Portmaster will retry to connect automatically.", err),
				spnPageButton,
			).AttachToModule(module)
		}

		return clientResultReconnect
	}

	// Log new connection.
	home, _ := navigator.Main.GetHome()
	if home != nil {
		log.Infof("spn/captain: established new home %s", home.Hub)
	}

	return clientResultOk
}

func clientSetActiveConnectionStatus(ctx context.Context) clientComponentResult {
	// Get current home.
	home, homeTerminal := navigator.Main.GetHome()
	if home == nil || homeTerminal == nil {
		return clientResultReconnect
	}

	// Update notification, if not already correctly set.
	notificationID := "spn:connected-to-home-hub"
	_, currentID, _ := module.FailureStatus()
	if currentID != notificationID {
		notifications.NotifyInfo(
			"spn:connected-to-home-hub",
			"Connected to SPN",
			fmt.Sprintf("You are connected to the SPN at %s. This notification is persistent for awareness.", homeTerminal.RemoteAddr()),
			spnPageButton,
		).AttachToModule(module)
	}

	// Update SPN Status with connection information, if not already correctly set.
	spnStatus.Lock()
	defer spnStatus.Unlock()

	if spnStatus.Status != StatusConnected || spnStatus.HomeHubID != home.Hub.ID {
		// Fill connection status data.
		spnStatus.Status = StatusConnected
		spnStatus.HomeHubID = home.Hub.ID
		spnStatus.HomeHubName = home.Hub.Info.Name

		connectedIP, err := netutils.IPFromAddr(homeTerminal.RemoteAddr())
		if err != nil {
			spnStatus.ConnectedIP = homeTerminal.RemoteAddr().String()
		} else {
			spnStatus.ConnectedIP = connectedIP.String()
		}
		spnStatus.ConnectedTransport = homeTerminal.Transport().String()

		now := time.Now()
		spnStatus.ConnectedSince = &now

		// Push new status.
		pushSPNStatusUpdate()
	}

	return clientResultOk
}

func clientCheckHomeHubConnection(ctx context.Context) clientComponentResult {
	// Check the status of the Home Hub.
	home, homeTerminal := navigator.Main.GetHome()
	if home == nil || homeTerminal == nil || homeTerminal.IsBeingAbandoned() {
		return clientResultReconnect
	}

	// Get crane controller for health check.
	crane := docks.GetAssignedCrane(home.Hub.ID)
	if crane == nil {
		log.Errorf("spn/captain: could not find home hub crane for health check")
		return clientResultOk
	}

	// Ping home hub.
	duration, tErr := docks.Ping(ctx, crane.Controller, clientHealthCheckTimeout)
	if tErr != nil {
		log.Warningf("spn/captain: failed to ping home hub: %s", tErr)
		return clientResultReconnect
	}

	log.Debugf("spn/captain: pinged home hub in %s", duration)
	return clientResultOk
}
