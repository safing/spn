package port17

import (
	"fmt"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/Safing/safing-core/container"
	"github.com/Safing/safing-core/formats/varint"
	"github.com/Safing/safing-core/port17/ships"
)

var testData = []byte("The quick brown fox jumps over the lazy dog. ")

func TestCrane(t *testing.T) {

	ship := ships.NewDummyShip()
	serverBottle, err := newPortIdentity("server")
	if err != nil {
		t.Fatalf("could not create bottle: %s", err)
	}

	crane1, err := NewCrane(ship, serverBottle)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	go crane1.unloader()
	go crane1.loader()

	crane2, err := NewCrane(ship.Reverse(), serverBottle)
	if err != nil {
		t.Fatalf("could not create crane: %s", err)
	}
	go crane2.unloader()
	go crane2.loader()

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
	fmt.Print("sending: ")
	for i := 0; i < 1000; i++ {
		crane1.toShip <- container.NewContainer(testData)
		fmt.Print(".")
	}
	fmt.Print(" done\n")
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
			c = <-crane2.fromShip

			// get real data part
			newShipmentData := c.CompileData()
			realDataLen, n, err := varint.Unpack32(newShipmentData)
			if err != nil {
				t.Fatalf("crane %s: could not get length of real data: %s", crane2.ID, err)
			}
			dataEnd := n + int(realDataLen)
			if dataEnd > len(newShipmentData) {
				t.Fatalf("crane %s: length of real data is greater than available data: %d", crane2.ID, realDataLen)
			}

			c = container.NewContainer(newShipmentData[n:dataEnd])
			char = c.GetMax(1)
		}
		if char[0] != testData[i%len(testData)] {
			t.Fatalf("mismatch at byte %d, expected '%s', got '%s', remaining received data is: '%s'", i, string(testData[i%len(testData)]), string(char[0]), string(c.CompileData()))
		}
		if i%len(testData) == 0 {
			fmt.Print(".")
		}
	}
	fmt.Print(" done\n")

	close(finished)

}
