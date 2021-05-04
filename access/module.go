package access

import (
	"flag"
	"fmt"

	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/modules"
)

var (
	module         *modules.Module
	testMode       bool
	accessCodeFlag string
)

func init() {
	module = modules.Register("access-codes", nil, nil, nil)
	flag.StringVar(&accessCodeFlag, "access-code", "", "Supply an SPN Special Access Code")
}

func prep() error {
	if testMode {
		// test handler
		testHandler, err := NewSaticCodeHandler(
			"Zwj41uHqLw9U3hNTTgUCfiZYJ1SNyt6JiSJPqdKHUHogNA",
			lhash.BLAKE2b_256,
		)
		if err != nil {
			return fmt.Errorf("failed to create test handler: %s", err)
		}
		RegisterZone("test", testHandler)

		// test code
		code, err := ParseCode("test:DcAszve1aLxQLEfUPcXnMTsnRrbChRxscaWK3s3rrz79")
		if err != nil {
			return fmt.Errorf("failed to parse test code: %s", err)
		}
		return Import(code)
	}

	// alpha1 handler
	alpha1Handler, err := NewSaticCodeHandler(
		"ZwojEvXZmAv7SZdNe7m94Xzu7F9J8vULqKf7QYtoTpN2tH",
		lhash.BLAKE2b_256,
	)
	if err != nil {
		return fmt.Errorf("failed to create alpha1 handler: %s", err)
	}
	RegisterZone("alpha2", alpha1Handler)

	// parse access code flag
	if accessCodeFlag != "" {
		// test code
		code, err := ParseCode(accessCodeFlag)
		if err != nil {
			return fmt.Errorf("the supplied access code is malformed: %s", err)
		}
		err = Import(code)
		if err != nil {
			return fmt.Errorf("failed to import supplied access code: %s", err)
		}
	}

	return nil
}

// TestMode activates test mode and only uses a fixed testing access code.
func TestMode() {
	testMode = true
}
