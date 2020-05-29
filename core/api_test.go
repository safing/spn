package core

import (
	"fmt"
	"testing"

	"github.com/safing/spn/ships"
)

func TestPort17Api(t *testing.T) {

	// create ships
	ship1_2 := ships.NewDummyShip()
	ship2_3 := ships.NewDummyShip()
	ship3_4 := ships.NewDummyShip()

	// create bottles
	bottle2, err := newPortIdentity("Port2")
	if err != nil {
		t.Fatalf("could not create bottle: %s", err)
	}

	bottle3, err := newPortIdentity("Port3")
	if err != nil {
		t.Fatalf("could not create bottle: %s", err)
	}

	bottle4, err := newPortIdentity("Port4")
	if err != nil {
		t.Fatalf("could not create bottle: %s", err)
	}

	// create crane1
	crane1, err := NewCrane(ship1_2, bottle2.Public())
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane1.Initialize()

	// create crane2_1
	crane2_1, err := NewCrane(ship1_2.Reverse(), bottle2)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane2_1.Initialize()

	// create crane2_3
	crane2_3, err := NewCrane(ship2_3, bottle3.Public())
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane2_3.Initialize()

	// create crane3_2
	crane3_2, err := NewCrane(ship2_3.Reverse(), bottle3)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane3_2.Initialize()

	// create crane3_4
	crane3_4, err := NewCrane(ship3_4, bottle4.Public())
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane3_4.Initialize()

	// create crane4
	crane4, err := NewCrane(ship3_4.Reverse(), bottle4)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane4.Initialize()

	// access bottle2 via crane1
	AssignCrane(bottle2.PortName, crane1)
	// access bottle3 via crane2_3
	AssignCrane(bottle3.PortName, crane2_3)
	// access bottle4 via crane3_4
	AssignCrane(bottle4.PortName, crane3_4)

	// start test

	init2 := NewInitializer()
	init2.KeyexIDs = []int{1}
	node2Api, err := NewClient(init2, bottle2.Public())
	if err != nil {
		t.Fatalf("failed to create port17 client to bottle2: %s", err)
	}

	info2, err := node2Api.Info()
	if err != nil {
		t.Fatalf("failed to get port17 info from bottle2: %s", err)
	}
	fmt.Printf("info2: %v\n", info2)

	init3 := NewInitializer()
	init3.KeyexIDs = []int{1}
	node3Api, err := node2Api.Hop(init3, bottle3.Public())
	if err != nil {
		t.Fatalf("failed to hop to bottle3: %s", err)
	}

	info3, err := node3Api.Info()
	if err != nil {
		t.Fatalf("failed to get port17 info from bottle3: %s", err)
	}
	fmt.Printf("info3: %v\n", info3)

	init4 := NewInitializer()
	init4.KeyexIDs = []int{1}
	node4Api, err := node3Api.Hop(init4, bottle4.Public())
	if err != nil {
		t.Fatalf("failed to hop to bottle4: %s", err)
	}

	info4, err := node4Api.Info()
	if err != nil {
		t.Fatalf("failed to get port17 info from bottle4: %s", err)
	}
	fmt.Printf("info4: %v\n", info4)

}
