package ships

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTestShip(t *testing.T) {
	tShip := NewTestShip()

	// interface conformance test
	var ship Ship = tShip

	srvShip := tShip.Reverse()

	for i := 0; i < 100; i++ {
		// client send
		ok, err := ship.Load(testData)
		if err != nil {
			t.Fatalf("%s failed: %s", ship, err)
		}
		if !ok {
			t.Fatalf("%s sunk", ship)
		}

		// server recv
		buf := getTestBuf()
		_, ok, err = srvShip.UnloadTo(buf)
		if err != nil {
			t.Fatalf("%s failed: %s", ship, err)
		}
		if !ok {
			t.Fatalf("%s sunk", ship)
		}

		// check data
		assert.Equal(t, testData, buf, "should match")
		fmt.Print(".")

		// server send
		ok, err = srvShip.Load(testData)
		if err != nil {
			t.Fatalf("%s failed: %s", ship, err)
		}
		if !ok {
			t.Fatalf("%s sunk", ship)
		}

		// client recv
		buf = getTestBuf()
		_, ok, err = ship.UnloadTo(buf)
		if err != nil {
			t.Fatalf("%s failed: %s", ship, err)
		}
		if !ok {
			t.Fatalf("%s sunk", ship)
		}

		// check data
		assert.Equal(t, testData, buf, "should match")
		fmt.Print(".")
	}

	ship.Sink()
	srvShip.Sink()
}
