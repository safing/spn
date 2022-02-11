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
	"github.com/safing/spn/access"
)

var (
	bootstrapped = abool.New()
	ready        = abool.New()

	spnTestPhaseStatusLinkButton = notifications.Action{
		Text:    "Test Phase Status",
		Type:    notifications.ActionTypeOpenURL,
		Payload: "https://docs.safing.io/spn/broader-testing/status",
	}
	spnLoginButton = notifications.Action{
		Text:    "Login",
		Type:    notifications.ActionTypeOpenPage,
		Payload: "spn",
	}
	spnOpenAccountWeb = notifications.Action{
		Text:    "Open account.safing.io",
		Type:    notifications.ActionTypeOpenURL,
		Payload: "https://account.safing.io",
	}
	spnSettingsButton = notifications.Action{
		Text: "Configure",
		Type: notifications.ActionTypeOpenSetting,
		Payload: &notifications.ActionTypeOpenSettingPayload{
			Key: CfgOptionEnableSPNKey,
		},
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

func clientManager(ctx context.Context) error {
	defer bootstrapped.UnSet()
	defer resetSPNStatus(StatusDisabled)

	module.Hint(
		"spn:establishing-home-hub",
		"Connecting to SPN...",
		"Connecting to the SPN network is in progress.",
	)

	for {
		err := preFlightCheck(ctx)
		if err != nil {
			log.Warningf("spn/captain: pre-flight check failed: %s", err)
		} else {
			bootstrapped.Set()

			err = homeHubManager(ctx)
			if err != nil {
				log.Warningf("spn/captain: primary hub manager failed: %s", err)
			}
		}

		// Try again after a short break.
		select {
		case <-ctx.Done():
			module.Resolve("")
			return nil
		case <-time.After(1 * time.Second):
		}
	}
}

func preFlightCheck(ctx context.Context) error {
	// Get SPN user.
	user, err := access.GetUser()
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		notifications.NotifyError(
			"spn:failed-to-get-user",
			"SPN Internal Error",
			`Please restart Portmaster.`,
			spnSettingsButton,
		).AttachToModule(module)
		resetSPNStatus(StatusFailed)
		return fmt.Errorf("internal error: %w", err)
	}

	// Check if user is logged in.
	if user == nil || !user.IsLoggedIn() {
		notifications.NotifyWarn(
			"spn:not-logged-in",
			"SPN Login Required",
			`Please log in with your SPN account.`,
			spnLoginButton,
			spnSettingsButton,
		).AttachToModule(module)
		resetSPNStatus(StatusFailed)
		return access.ErrNotLoggedIn
	}

	// TODO: When we are starting and the SPN module is faster online than the
	// nameserver, then updating the account will fail as the DNS query is
	// redirected to a closed port.
	// We also can't add the nameserver as a module dependency, as the nameserver
	// is not part of the server.
	time.Sleep(1 * time.Second)

	// Update account and get tokens.
	err = access.UpdateAccount(ctx, nil)
	if err != nil {
		if errors.Is(err, access.ErrMayNotUseSPN) {
			notifications.NotifyError(
				"spn:subscription-inactive",
				"SPN Subscription Inactive",
				`Your account is currently not subscribed to the SPN. Follow the instructions on account.safing.io to get started.`,
				spnOpenAccountWeb,
				spnSettingsButton,
			).AttachToModule(module)
			resetSPNStatus(StatusFailed)
			return errors.New("user may not use SPN")
		}
		log.Errorf("captain: failed to update account in pre-flight: %s", err)
		module.NewErrorMessage("pre-flight account update", err).Report()

		// There was an error updating the account.
		// Check if we have enough tokens to continue anyway.
		regular, fallback := access.GetTokenAmount(access.ExpandAndConnectZones)
		if regular == 0 && fallback == 0 {
			notifications.NotifyError(
				"spn:tokens-exhausted",
				"SPN Access Tokens Exhausted",
				`The Portmaster failed to get new access tokens to access SPN. Please try again later.`,
				spnSettingsButton,
			).AttachToModule(module)
			resetSPNStatus(StatusFailed)
			return errors.New("access tokens exhausted")
		}
	}

	// looking good so far!
	return nil
}
