package access

import (
	"testing"

	"github.com/safing/jess/lhash"
)

func TestGenerateAndCheck(t *testing.T) {
	// Part 1: Manual Setup

	// setup handler
	zone := "test-static"
	handler := &SaticCodeHandler{
		scrambleAlg: lhash.BLAKE2b_256,
	}
	RegisterZone(zone, handler)

	// generate a new code
	randomData, err := getBeautifulRandom(staticCodeSecretSize)
	if err != nil {
		t.Fatal(err)
	}
	configuredCode := &Code{
		Zone: zone,
		Data: randomData,
	}
	t.Logf("configured code: %s", configuredCode.String())

	// import code, have it scrambled
	err = Import(configuredCode)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scrambled code: %s", handler.scrambledCode.String())

	// create verification hash from scrambled code
	verificationHash := lhash.Digest(handler.scrambleAlg, handler.scrambledCode.Data)
	t.Logf("verification hash: %s", verificationHash.String())

	// load verification hash like the constructor does
	verifier, err := lhash.LoadFromString(verificationHash.String())
	if err != nil {
		t.Fatal(err)
	}
	handler.verifier = verifier

	// check if scrambled code is valid
	err = Check(handler.scrambledCode)
	if err != nil {
		t.Fatal(err)
	}

	// Part 2: Production Storyline

	// server: create zone with verification hash at start
	zone = "test-static-2"
	handler2, err := NewSaticCodeHandler(verificationHash.String(), lhash.BLAKE2b_256)
	if err != nil {
		t.Fatal(err)
	}
	RegisterZone(zone, handler2)

	// client: get configured code and import it
	configuredCode2 := &Code{
		Zone: zone,
		Data: configuredCode.Data,
	}
	err = Import(configuredCode2)
	if err != nil {
		t.Fatal(err)
	}

	// client: when connecting, get code for authorization
	scrambledCode2, err := handler2.Get()
	if err != nil {
		t.Fatal(err)
	}

	// server: check code
	err = Check(scrambledCode2)
	if err != nil {
		t.Fatal(err)
	}
}
