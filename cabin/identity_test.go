package cabin

import (
	"context"
	"testing"

	"github.com/safing/spn/hub"
)

func TestIdentity(t *testing.T) {
	// create

	id, err := CreateIdentity(context.Background(), hub.ScopePublic)
	if err != nil {
		t.Fatal(err)
	}

	// maintain

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

	lanes := []*hub.Lane{
		&hub.Lane{
			ID:       "A",
			Capacity: 1,
			Latency:  2,
		},
		&hub.Lane{
			ID:       "B",
			Capacity: 3,
			Latency:  4,
		},
		&hub.Lane{
			ID:       "C",
			Capacity: 5,
			Latency:  6,
		},
	}
	changed, err = id.MaintainStatus(lanes)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("status should have changed")
	}

	changed, err = id.MaintainStatus(lanes)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("unexpected change of status")
	}

	// export

	_, err = id.ExportAnnouncement()
	if err != nil {
		t.Fatal(err)
	}

	_, err = id.ExportStatus()
	if err != nil {
		t.Fatal(err)
	}

	// check if identity was registered in the hub DB

	_, err = hub.GetHub(id.Scope, id.ID)
	if err != nil {
		t.Fatal(err)
	}
}
