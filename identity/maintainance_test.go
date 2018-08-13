package identity

import (
	"testing"
	"time"
)

func TestIdentityManagement(t *testing.T) {
	identity, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	checkAddresses(identity)
	t.Logf("initial identity: %v", identity)

	iterations := 1000
	changeCnt := 0

	now := time.Now()
	for i := 0; i < iterations; i++ {
		changed := checkEphermalKeys(identity, now)
		if changed {
			changeCnt += 1
			t.Logf("identity changed at timeslot %d: %v | PUB: %v", i, identity, identity.PublicWithMinValidity(now.Add(minAdvertiseValidity)))
		}
		now = now.Add(1 * time.Hour)
	}

	if iterations/changeCnt > 100 {
		t.Fatal("more changes than expected")
	}
	if len(identity.Keys) > 10 {
		t.Fatal("more keys than expected")
	}
}
