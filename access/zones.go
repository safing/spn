package access

import (
	"fmt"

	"github.com/safing/spn/conf"

	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/access/token"
	"github.com/safing/spn/terminal"
)

var (
	ExpandAndConnectZones = []string{"pblind1", "alpha2", "fallback1"}
	persistentZones       = ExpandAndConnectZones

	zonePermissions = map[string]terminal.Permission{
		"pblind1":   terminal.AddPermissions(terminal.MayExpand, terminal.MayConnect),
		"alpha2":    terminal.AddPermissions(terminal.MayExpand, terminal.MayConnect),
		"fallback1": terminal.AddPermissions(terminal.MayExpand, terminal.MayConnect),
	}
)

func initializeZones() error {
	// Special client zone config.
	var requestSignalHandler func(token.Handler)
	if conf.Client() {
		requestSignalHandler = shouldRequestTokensHandler
	}

	// Register pblind1 as the first primary zone.
	ph, err := token.NewPBlindHandler(token.PBlindOptions{
		Zone:                "pblind1",
		CurveName:           "P-256",
		PublicKey:           "eXoJXzXbM66UEsM2eVi9HwyBPLMfVnNrC7gNrsfMUJDs",
		UseSerials:          true,
		BatchSize:           1000,
		RandomizeOrder:      true,
		SignalShouldRequest: requestSignalHandler,
	})
	if err != nil {
		return fmt.Errorf("failed to create pblind1 token handler: %w", err)
	}
	err = token.RegisterPBlindHandler(ph)
	if err != nil {
		return fmt.Errorf("failed to register pblind1 token handler: %w", err)
	}

	// Register fallback1 zone as fallback when the issuer is not available.
	sh, err := token.NewScrambleHandler(token.ScrambleOptions{
		Zone:             "fallback1",
		Algorithm:        lhash.BLAKE2b_256,
		InitialVerifiers: []string{"ZwkQoaAttVBMURzeLzNXokFBMAMUUwECfM1iHojcVKBmjk"},
		Fallback:         true,
	})
	if err != nil {
		return fmt.Errorf("failed to create fallback1 token handler: %w", err)
	}
	err = token.RegisterScrambleHandler(sh)
	if err != nil {
		return fmt.Errorf("failed to register fallback1 token handler: %w", err)
	}

	// Register alpha2 zone for transition phase.
	sh, err = token.NewScrambleHandler(token.ScrambleOptions{
		Zone:             "alpha2",
		Algorithm:        lhash.BLAKE2b_256,
		InitialVerifiers: []string{"ZwojEvXZmAv7SZdNe7m94Xzu7F9J8vULqKf7QYtoTpN2tH"},
	})
	if err != nil {
		return fmt.Errorf("failed to create alpha2 token handler: %w", err)
	}
	err = token.RegisterScrambleHandler(sh)
	if err != nil {
		return fmt.Errorf("failed to register alpha2 token handler: %w", err)
	}

	return nil
}

func resetZones() {
	token.ResetRegistry()
}

func shouldRequestTokensHandler(_ token.Handler) {
	// accountUpdateTask is always set in client mode and when the module is online.
	// Check if it's set in case this gets executed in other circumstances.
	if accountUpdateTask == nil {
		log.Warningf("access: trying to trigger account update, but the task is available")
		return
	}

	accountUpdateTask.StartASAP()
}

func GetTokenAmount(zones []string) (regular, fallback int) {
handlerLoop:
	for _, zone := range zones {
		// Get handler and check if it should be used.
		handler, ok := token.GetHandler(zone)
		if !ok {
			log.Warningf("access: use of non-registered zone %q", zone)
			continue handlerLoop
		}

		if handler.IsFallback() {
			fallback += handler.Amount()
		} else {
			regular += handler.Amount()
		}
	}

	return
}

func GetToken(zones []string) (t *token.Token, err error) {
handlerSelection:
	for _, zone := range zones {
		// Get handler and check if it should be used.
		handler, ok := token.GetHandler(zone)
		switch {
		case !ok:
			log.Warningf("access: use of non-registered zone %q", zone)
			continue handlerSelection
		case handler.IsFallback() && !TokenIssuerIsFailing():
			// Skip fallback zone if everything works.
			continue handlerSelection
		}

		// Get token from handler.
		t, err = token.GetToken(zone)
		if err == nil {
			return t, nil
		}
	}

	// Return existing error, if exists.
	if err != nil {
		return nil, err
	}
	return nil, token.ErrEmpty
}

func VerifyRawToken(data []byte) (granted terminal.Permission, err error) {
	t, err := token.ParseRawToken(data)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token: %w", err)
	}

	return VerifyToken(t)
}

func VerifyToken(t *token.Token) (granted terminal.Permission, err error) {
	handler, ok := token.GetHandler(t.Zone)
	if !ok {
		return terminal.NoPermission, token.ErrZoneUnknown
	}

	// Check if the token is a fallback token.
	if handler.IsFallback() && !healthCheck() {
		return terminal.NoPermission, ErrFallbackNotAvailable
	}

	// Verify token.
	err = handler.Verify(t)
	if err != nil {
		return 0, fmt.Errorf("failed to verify token: %w", err)
	}

	// Return permission of zone.
	granted, ok = zonePermissions[t.Zone]
	if !ok {
		return terminal.NoPermission, nil
	}
	return granted, nil
}
