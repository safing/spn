package captain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/log"
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
			// workaround to clear any status
			module.Hint("reset", "Stopping.")
			module.Resolve("reset")
			return nil
		case <-time.After(1 * time.Second):
		}

		err := preFlightCheck(ctx)
		if err != nil {
			log.Warningf("spn/captain: pre-flight check failed: %s", err)
			continue
		}

		err = primaryHubManager(ctx)
		if err != nil {
			log.Warningf("spn/captain: primary hub manager failed: %s", err)
			continue
		}
	}
}

func preFlightCheck(ctx context.Context) error {
	// 0) Check for existing access code
	_, err := access.Get()
	if err == nil {
		return nil
	}

	// 1) Check access code config
	if cfgOptionSpecialAccessCode() == cfgOptionSpecialAccessCodeDefault {
		module.Warning("no-access-code", "Please enter your special access code in the settings.")
		return errors.New("no access code configured")
	}
	module.Resolve("no-access-code")

	// 2) Parse and import access code
	code, err := access.ParseCode(cfgOptionSpecialAccessCode())
	if err == nil {
		err = access.Import(code)
	}
	if err != nil {
		module.Warning("invalid-access-code", "Your special access code is invalid: "+err.Error())
		return errors.New("invalid access code")
	}
	module.Resolve("invalid-access-code")

	// 3) Get access code
	_, err = access.Get()
	if err != nil {
		module.Warning("internal-code-error", "Internal access code error: "+err.Error())
		return fmt.Errorf("failed to get access code: %s", err)
	}
	module.Resolve("internal-code-error")

	// looking good so far!
	return nil
}
