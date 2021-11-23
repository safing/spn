package access

import (
	"testing"

	"github.com/safing/jess/lhash"
	"github.com/safing/spn/terminal"
)

func TestGenerateAndCheck(t *testing.T) {
	// Part 1: Bootstrap

	_, accessCode, scrambledCode, verificationHash, err := BootstrapStaticCodeHandler(
		"test-static-bootstrap",
		lhash.BLAKE2b_256,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("access code: %s", accessCode)
	t.Logf("scrambled code: %s", scrambledCode)
	t.Logf("verification hash: %s", verificationHash.Base58())

	// Part 2: Production Storyline

	// Client and Server: Create a static code handler with the verification hash.
	zone := "test-static-prod"
	handler, err := NewSaticCodeHandler(verificationHash.Base58(), lhash.BLAKE2b_256)
	if err != nil {
		t.Fatal(err)
	}
	RegisterZone(zone, handler, terminal.AddPermissions(
		terminal.MayExpand,
		terminal.MayConnect,
	))

	// Client only: Import access code.
	accessCode.Zone = zone
	err = Import(accessCode)
	if err != nil {
		t.Fatal(err)
	}

	// Client only: Get scrambled code to send to server.
	codeForServer, err := handler.Get()
	if err != nil {
		t.Fatal(err)
	}

	// Server only: Check code provided by client.
	_, err = Check(codeForServer)
	if err != nil {
		t.Fatal(err)
	}
}
