package captain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/notifications"
	"github.com/safing/spn/access"
)

var (
	ready = abool.New()
)

func ClientReady() bool {
	return ready.IsSet()
}

func clientManager(ctx context.Context) error {
	for {
		// wait / try again
		select {
		case <-ctx.Done():
			module.Resolve("")
			return nil
		case <-time.After(1 * time.Second):
		}

		err := preFlightCheck(ctx)
		if err != nil {
			log.Warningf("spn/captain: pre-flight check failed: %s", err)
			continue
		}

		err = homeHubManager(ctx)
		if err != nil {
			log.Warningf("spn/captain: primary hub manager failed: %s", err)
			continue
		}
	}
}

func preFlightCheck(ctx context.Context) error {
	// 0) Check for existing access code
	_, err := access.GetCode(access.MainZone)
	if err == nil {
		return nil
	}

	// 1) Check access code config
	if cfgOptionSpecialAccessCode() == cfgOptionSpecialAccessCodeDefault {
		notifications.NotifyWarn(
			"spn:no-access-code",
			"SPN Requires Access Code",
			"Please enter your special access code for the testing phase in the settings.",
			notifications.Action{
				Text: "Enter Code",
				Type: notifications.ActionTypeOpenSetting,
				Payload: &notifications.ActionTypeOpenSettingPayload{
					Key: cfgOptionSpecialAccessCodeKey,
				},
			},
		).AttachToModule(module)
		return errors.New("no access code configured")
	}
	module.Resolve("spn:no-access-code")

	// 2) Parse and import access code
	code, err := access.ParseCode(cfgOptionSpecialAccessCode())
	if err == nil {
		err = access.Import(code)
	}
	if err != nil {
		notifications.NotifyWarn(
			"spn:invalid-access-code",
			"SPN Access Code Invalid",
			"Your special access code is invalid: "+err.Error(),
			notifications.Action{
				Text: "Enter Code",
				Type: notifications.ActionTypeOpenSetting,
				Payload: &notifications.ActionTypeOpenSettingPayload{
					Key: cfgOptionSpecialAccessCodeKey,
				},
			},
		).AttachToModule(module)
		return errors.New("invalid access code")
	}
	module.Resolve("spn:invalid-access-code")

	// 3) Get access code
	_, err = access.GetCode(access.MainZone)
	if err != nil {
		notifications.NotifyWarn(
			"spn:internal-access-code-error",
			"SPN Access Code Invalid",
			"Internal access code error: "+err.Error(),
			notifications.Action{
				Text: "Enter Code",
				Type: notifications.ActionTypeOpenSetting,
				Payload: &notifications.ActionTypeOpenSettingPayload{
					Key: cfgOptionSpecialAccessCodeKey,
				},
			},
		).AttachToModule(module)
		return fmt.Errorf("failed to get access code: %s", err)
	}
	module.Resolve("spn:internal-access-code-error")

	// looking good so far!
	return nil
}
