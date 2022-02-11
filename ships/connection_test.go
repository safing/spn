package ships

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/safing/spn/hub"
)

var (
	testPort  uint16 = 65000
	testData         = []byte("The quick brown fox jumps over the lazy dog")
	localhost        = net.IPv4(127, 0, 0, 1)
)

func getTestPort() uint16 {
	testPort++
	return testPort
}

func getTestBuf() []byte {
	return make([]byte, len(testData))
}

func TestConnections(t *testing.T) {
	t.Parallel()

	registryLock.Lock()
	t.Cleanup(func() {
		registryLock.Unlock()
	})

	for k, v := range registry { //nolint:paralleltest // False positive.
		protocol, builder := k, v
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()

			var wg sync.WaitGroup
			ctx := context.Background()

			// docking requests
			requests := make(chan *DockingRequest, 1)
			transport := &hub.Transport{
				Protocol: protocol,
				Port:     getTestPort(),
			}

			// create listener
			pier, err := builder.EstablishPier(transport, requests)
			if err != nil {
				t.Fatal(err)
			}
			wg.Add(1)
			var dockingErr error
			go func() {
				err := pier.Docking(ctx)
				dockingErr = err
				wg.Done()
			}()

			// connect to listener
			ship, err := builder.LaunchShip(ctx, transport, localhost)
			if err != nil {
				t.Fatal(err)
			}

			// client send
			err = ship.Load(testData)
			if err != nil {
				t.Fatalf("%s failed: %s", ship, err)
			}

			// dock client
			request := <-requests
			if request.Err != nil {
				t.Fatalf("%s failed to dock: %s", request.Pier, request.Err)
			}
			srvShip := request.Ship

			// server recv
			buf := getTestBuf()
			_, err = srvShip.UnloadTo(buf)
			if err != nil {
				t.Fatalf("%s failed: %s", ship, err)
			}

			// check data
			assert.Equal(t, testData, buf, "should match")
			fmt.Print(".")

			for i := 0; i < 100; i++ {
				// server send
				err = srvShip.Load(testData)
				if err != nil {
					t.Fatalf("%s failed: %s", ship, err)
				}

				// client recv
				buf = getTestBuf()
				_, err = ship.UnloadTo(buf)
				if err != nil {
					t.Fatalf("%s failed: %s", ship, err)
				}

				// check data
				assert.Equal(t, testData, buf, "should match")
				fmt.Print(".")

				// client send
				err = ship.Load(testData)
				if err != nil {
					t.Fatalf("%s failed: %s", ship, err)
				}

				// server recv
				buf = getTestBuf()
				_, err = srvShip.UnloadTo(buf)
				if err != nil {
					t.Fatalf("%s failed: %s", ship, err)
				}

				// check data
				assert.Equal(t, testData, buf, "should match")
				fmt.Print(".")
			}

			// Check for docking error.
			if dockingErr != nil {
				t.Fatal(err)
			}

			ship.Sink()
			srvShip.Sink()
			pier.Abolish()
			wg.Wait() // wait for docking procedure to end

			// Check for docking error again.
			if dockingErr != nil {
				t.Fatal(err)
			}
		})
	}
}
