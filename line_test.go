package port17

import (
	"fmt"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/Safing/safing-core/container"
	"github.com/Safing/safing-core/port17/ships"
)

func TestLine(t *testing.T) {

	ship := ships.NewDummyShip()
	serverBottle, err := newPortIdentity("server")
	if err != nil {
		t.Fatalf("could not create bottle: %s", err)
	}

	crane1, err := NewCrane(ship, serverBottle.Public())
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane1.Initialize()
	line1, err := NewConveyorLine(crane1, 1)
	if err != nil {
		t.Fatalf("could not create line: %s", err)
	}
	endpoint1 := &LastConveyorBase{
		fromShip: make(chan *container.Container),
		toShip:   make(chan *container.Container),
	}
	line1.AddLastConveyor(endpoint1)
	crane1.lines[1] = line1

	crane2, err := NewCrane(ship.Reverse(), serverBottle)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	crane2.Initialize()
	line2, err := NewConveyorLine(crane2, 1)
	if err != nil {
		t.Fatalf("could not create line: %s", err)
	}
	endpoint2 := &LastConveyorBase{
		fromShip: make(chan *container.Container),
		toShip:   make(chan *container.Container),
	}
	line2.AddLastConveyor(endpoint2)
	crane2.lines[1] = line2

	fmt.Print("crane test setup complete.\n")

	finished := make(chan struct{})
	go func() {
		// wait for test to complete, panic after timeout
		time.Sleep(10 * time.Second)
		select {
		case <-finished:
		default:
			fmt.Println("===== TAKING TOO LONG FOR TEST - PRINTING STACK TRACES =====")
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			os.Exit(1)
		}
	}()

	// send data
	go func() {
		fmt.Print("sending: ")
		for i := 0; i < 1000; i++ {
			endpoint1.toShip <- container.NewContainer(testData)
			fmt.Print(".")
		}
		fmt.Print(" done\n")
	}()
	totalLength := len(testData) * 1000

	// receive and check data
	var c *container.Container
	var char []byte
	fmt.Print("receiving: ")
	for i := 0; i < totalLength; i++ {
		if c != nil {
			char = c.GetMax(1)
		}
		if len(char) == 0 {
			c = <-endpoint2.fromShip
			if c == nil {
				t.Fatalf("crane stopped")
			}
			if c.HasError() {
				t.Fatalf("received error: %s", c.Error())
			}
			char = c.GetMax(1)
		}
		if char[0] != testData[i%len(testData)] {
			t.Fatalf("mismatch at byte %d, expected '%s', got '%s', remaining received data is: '%s'", i, string(testData[i%len(testData)]), string(char[0]), string(c.CompileData()))
		}
		if i%len(testData) == 0 {
			fmt.Print("-")
		}
	}
	fmt.Print(" done\n")

	close(finished)

}
