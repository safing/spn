package docks

import (
	"fmt"
	"os"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/safing/spn/cabin"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
	"github.com/safing/spn/terminal"
)

func TestExpansion(t *testing.T) {
	testExpansion(t, "plain-expansion", false, 100)
}

func testExpansion(t *testing.T, testID string, encrypting bool, countTo uint64) {
	var identity2, identity3 *cabin.Identity
	var connectedHub2, connectedHub3 *hub.Hub
	if encrypting {
		identity2, connectedHub2 = getTestIdentity(t)
		identity3, connectedHub3 = getTestIdentity(t)
	}

	// Build ships and cranes.
	optimalMinLoadSize = 100
	ship1to2 := ships.NewTestShip(!encrypting, 100)
	ship2to3 := ships.NewTestShip(!encrypting, 100)

	var crane1, crane2to1, crane2to3, crane3 *Crane
	var craneWg sync.WaitGroup
	craneWg.Add(4)

	go func() {
		var err error
		crane1, err = NewCrane(ship1to2, connectedHub2, nil)
		if err != nil {
			panic(fmt.Sprintf("expansion test %s could not create crane1: %s", testID, err))
			return
		}
		crane1.ID = "c1"
		err = crane1.Start()
		if err != nil {
			panic(fmt.Sprintf("expansion test %s could not start crane1: %s", testID, err))
			return
		}
		craneWg.Done()
	}()
	go func() {
		var err error
		crane2to1, err = NewCrane(ship1to2.Reverse(), nil, identity2)
		if err != nil {
			panic(fmt.Sprintf("expansion test %s could not create crane2to1: %s", testID, err))
			return
		}
		crane2to1.ID = "c2to1"
		err = crane2to1.Start()
		if err != nil {
			panic(fmt.Sprintf("expansion test %s could not start crane2to1: %s", testID, err))
			return
		}
		craneWg.Done()
	}()
	go func() {
		var err error
		crane2to3, err = NewCrane(ship2to3, connectedHub3, nil)
		if err != nil {
			panic(fmt.Sprintf("expansion test %s could not create crane2to3: %s", testID, err))
			return
		}
		crane2to3.ID = "c2to3"
		err = crane2to3.Start()
		fmt.Println("====================================================================")
		fmt.Printf("%+v\n", err)
		if err != nil {
			fmt.Printf("%+v\n", err)
			panic(fmt.Sprintf("expansion test %s could not start crane2to3: %s", testID, err))
			return
		}
		craneWg.Done()
	}()
	go func() {
		var err error
		crane3, err = NewCrane(ship2to3.Reverse(), nil, identity3)
		if err != nil {
			panic(fmt.Sprintf("expansion test %s could not create crane3: %s", testID, err))
			return
		}
		crane3.ID = "c3"
		err = crane3.Start()
		if err != nil {
			panic(fmt.Sprintf("expansion test %s could not start crane3: %s", testID, err))
			return
		}
		craneWg.Done()
	}()
	craneWg.Wait()

	// Assign crane.
	crane3HubID := testID + "-crane3HubID"
	AssignCrane(crane3HubID, crane2to3)

	t.Logf("expansion test %s setup complete", testID)

	// Wait async for test to complete, print stack after timeout.
	finished := make(chan struct{})
	go func() {
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			fmt.Printf("expansion test %s is taking too long, print stack:\n", testID)
			_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			os.Exit(1)
		}
	}()

	// Start initial crane.
	homeTerminal, initData, tErr := NewLocalCraneTerminal(crane1, nil, &terminal.TerminalOpts{}, crane1.submitTerminalMsg)
	if tErr != nil {
		t.Fatalf("expansion test %s failed to create home terminal: %s", testID, tErr)
	}
	tErr = crane1.EstablishNewTerminal(homeTerminal, initData)
	if tErr != nil {
		t.Fatalf("expansion test %s failed to connect home terminal: %s", testID, tErr)
	}

	time.Sleep(3 * time.Second)

	// Start expansion.
	expansionTerminalTo3, err := ExpandTo(homeTerminal, crane3HubID, connectedHub3)
	if err != nil {
		t.Fatalf("expansion test %s failed to expand to %s: %s", testID, crane3HubID, tErr)
	}

	// Start counters for testing.
	op1, tErr := terminal.NewCounterOp(expansionTerminalTo3, countTo, 10*time.Microsecond)
	if tErr != nil {
		t.Fatalf("crane test %s failed to run counter op: %s", testID, tErr)
	}
	module.StartWorker(testID+" counter op1", op1.CounterWorker)
	// op2, tErr := terminal.NewCounterOp(crane2.Controller, countTo)
	// if tErr != nil {
	// 	t.Fatalf("crane test %s failed to run counter op: %s", testID, tErr)
	// }
	// module.StartWorker(testID+" counter op2", op2.CounterWorker)

	// Wait for completion.
	op1.Wait()
	// op2.Wait()
	close(finished)

	// Wait a little so that all errors can be propagated, so we can truly see
	// if we succeeded.
	time.Sleep(1 * time.Second)

	// Check errors.
	if op1.Error != nil {
		t.Fatalf("crane test %s counter op1 failed: %s", testID, op1.Error)
	}
	// if op2.Error != nil {
	// t.Fatalf("crane test %s counter op2 failed: %s", testID, op2.Error)
	// }
}
