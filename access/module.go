package access

import (
	"flag"
	"fmt"

	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/terminal"
)

var (
	module         *modules.Module
	accessCodeFlag string
)

func init() {
	module = modules.Register("access-codes", nil, nil, nil)
	flag.StringVar(&accessCodeFlag, "access-code", "", "Supply an SPN Special Access Code")
}

func prep() error {
	// alpha2 handler
	alpha2Handler, err := NewSaticCodeHandler(
		"ZwojEvXZmAv7SZdNe7m94Xzu7F9J8vULqKf7QYtoTpN2tH",
		lhash.BLAKE2b_256,
	)
	if err != nil {
		return fmt.Errorf("failed to create alpha2 handler: %s", err)
	}
	RegisterZone("alpha2", alpha2Handler, terminal.AddPermissions(
		terminal.MayExpand,
		terminal.MayTunnel,
	))

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
