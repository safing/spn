package access

import (
	"sync"

	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/terminal"
)

var (
	initTestCode sync.Once
)

func EnableTestMode() {
	initTestCode.Do(registerTestCode)
}

func registerTestCode() {
	// Reset zones to only have test code in there.
	zonesLock.Lock()
	zones = make(map[string]*Zone)
	zonesLock.Unlock()

	// test handler
	testHandler, err := NewSaticCodeHandler(
		"Zwj41uHqLw9U3hNTTgUCfiZYJ1SNyt6JiSJPqdKHUHogNA",
		lhash.BLAKE2b_256,
	)
	if err != nil {
		log.Criticalf("failed to create test handler: %s", err)
		return
	}
	RegisterZone("test", testHandler, terminal.AddPermissions(
		terminal.MayExpand,
		terminal.MayTunnel,
	))

	// test code
	code, err := ParseCode("test:DcAszve1aLxQLEfUPcXnMTsnRrbChRxscaWK3s3rrz79")
	if err != nil {
		log.Criticalf("failed to parse test code: %s", err)
		return
	}
	err = Import(code)
	if err != nil {
		log.Criticalf("failed to import test code: %s", err)
		return
	}

	log.Warningf("spn/access: test code registered")
}
