package cabin

import (
	"context"
	"testing"
	"time"

	"github.com/safing/spn/hub"
)

func TestKeyMaintenance(t *testing.T) {
	id, err := CreateIdentity(context.Background(), hub.ScopePublic)
	if err != nil {
		t.Fatal(err)
	}

	iterations := 1000
	changeCnt := 0

	now := time.Now()
	for i := 0; i < iterations; i++ {
		changed, err := id.MaintainExchKeys(id.Hub.Status, now)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			changeCnt += 1
			t.Logf("===== exchange keys updated at %s:\n", now)
			for keyID, exchKey := range id.ExchKeys {
				t.Logf("[%s] %s %v\n", exchKey.Created, keyID, exchKey.key)
			}
		}
		now = now.Add(1 * time.Hour)
	}

	if iterations/changeCnt > 25 { // one new key every 24 hours/ticks
		t.Fatal("more changes than expected")
	}
	if len(id.ExchKeys) > 17 { // one new key every day for two weeks + 3 in use
		t.Fatal("more keys than expected")
	}
}
