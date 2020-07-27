package cabin

import (
	"context"
	"testing"

	"github.com/safing/spn/hub"
)

func TestIdentity(t *testing.T) {
	// maintenance

	id, err := CreateIdentity(context.Background(), hub.ScopePublic)
	if err != nil {
		t.Fatal(err)
	}

	changed, err := id.MaintainAnnouncement()
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("unexpected change of announcement")
	}

	changed, err = id.MaintainStatus(nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("unexpected change of status")
	}

	connections := []*hub.HubConnection{
		&hub.HubConnection{
			ID:       "A",
			Capacity: 1,
			Latency:  2,
		},
		&hub.HubConnection{
			ID:       "B",
			Capacity: 3,
			Latency:  4,
		},
		&hub.HubConnection{
			ID:       "C",
			Capacity: 5,
			Latency:  6,
		},
	}
	changed, err = id.MaintainStatus(connections)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("status should have changed")
	}

	changed, err = id.MaintainStatus(connections)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("unexpected change of status")
	}

	// export

}
