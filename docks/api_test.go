package docks

import (
	"context"
	"fmt"
	"testing"

	"github.com/safing/spn/access"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
)

func TestAPI(t *testing.T) {

	// create ships
	ship1_2 := ships.NewTestShip()
	ship2_3 := ships.NewTestShip()
	ship3_4 := ships.NewTestShip()

	// create bottles
	hub2ID, err := cabin.CreateIdentity(context.Background(), hub.ScopeTest)
	hub2ID.Hub().Info.Transports = []string{"tcp:17"}
	if err != nil {
		t.Fatalf("could not create identity: %s", err)
	}

	hub3ID, err := cabin.CreateIdentity(context.Background(), hub.ScopeTest)
	hub3ID.Hub().Info.Transports = []string{"tcp:17"}
	if err != nil {
		t.Fatalf("could not create identity: %s", err)
	}

	hub4ID, err := cabin.CreateIdentity(context.Background(), hub.ScopeTest)
	hub4ID.Hub().Info.Transports = []string{"tcp:17"}
	if err != nil {
		t.Fatalf("could not create identity: %s", err)
	}

	// create crane1
	crane1, err := NewCrane(ship1_2, nil, hub2ID.Hub())
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	// create crane2_1
	crane2_1, err := NewCrane(ship1_2.Reverse(), hub2ID, nil)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	// create crane2_3
	crane2_3, err := NewCrane(ship2_3, nil, hub3ID.Hub())
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	// create crane3_2
	crane3_2, err := NewCrane(ship2_3.Reverse(), hub3ID, nil)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	// create crane3_4
	crane3_4, err := NewCrane(ship3_4, nil, hub4ID.Hub())
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	// create crane4
	crane4, err := NewCrane(ship3_4.Reverse(), hub4ID, nil)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}

	// start cranes
	errs := make(chan error)
	go func() { errs <- crane1.Start() }()
	go func() { errs <- crane2_1.Start() }()
	go func() { errs <- crane2_3.Start() }()
	go func() { errs <- crane3_2.Start() }()
	go func() { errs <- crane3_4.Start() }()
	go func() { errs <- crane4.Start() }()
	for i := 0; i < 6; i++ {
		err = <-errs
		if err != nil {
			t.Fatalf("failed to start crane: %s", err)
		}
	}

	// access hub2 via crane1
	AssignCrane(hub2ID.Hub().ID, crane1)
	// access hub3 via crane2_3
	AssignCrane(hub3ID.Hub().ID, crane2_3)
	// access hub4 via crane3_4
	AssignCrane(hub4ID.Hub().ID, crane3_4)

	// start test

	// get access code
	code, err := access.Get()
	if err != nil {
		t.Fatal(err)
	}

	node2Api, err := NewClient(conf.CurrentVersion, hub2ID.Hub())
	if err != nil {
		t.Fatalf("failed to create client to Hub2: %s", err)
	}

	info2, err := node2Api.Info()
	if err != nil {
		t.Fatalf("failed to get Hub info from Hub2: %s", err)
	}
	fmt.Printf("info2: %v\n", info2)

	err = node2Api.UserAuth(code)
	if err != nil {
		t.Fatalf("failed to auth at Hub2: %s", err)
	}

	node3Api, err := node2Api.Hop(conf.CurrentVersion, hub3ID.Hub())
	if err != nil {
		t.Fatalf("failed to hop to Hub3: %s", err)
	}

	info3, err := node3Api.Info()
	if err != nil {
		t.Fatalf("failed to get Hub info from Hub3: %s", err)
	}
	fmt.Printf("info3: %v\n", info3)

	err = node3Api.UserAuth(code)
	if err != nil {
		t.Fatalf("failed to auth at Hub3: %s", err)
	}

	node4Api, err := node3Api.Hop(conf.CurrentVersion, hub4ID.Hub())
	if err != nil {
		t.Fatalf("failed to hop to Hub4: %s", err)
	}

	info4, err := node4Api.Info()
	if err != nil {
		t.Fatalf("failed to get Hub info from Hub4: %s", err)
	}
	fmt.Printf("info4: %v\n", info4)

	err = node4Api.UserAuth(code)
	if err != nil {
		t.Fatalf("failed to auth at Hub4: %s", err)
	}

}
