package ships

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/safing/spn/hub"

	"github.com/stretchr/testify/assert"
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
	ctx := context.Background()
	var wg sync.WaitGroup

	registryLock.Lock()
	defer registryLock.Unlock()
	for protocol, builder := range registry {
		t.Run(protocol, func(t *testing.T) {

			// docking requests
			requests := make(chan *DockingRequest, 1)
			transport := &hub.Transport{
				Protocol: protocol,
				Port:     getTestPort(),
			}

			// create listener
			pier, err := builder.EstablishPier(ctx, transport, requests)
			if err != nil {
				t.Fatal(err)
			}
			wg.Add(1)
			go func() { //nolint:staticcheck // we wait for the goroutine
				err := pier.Docking(ctx)
				if err != nil {
					t.Fatal(err)
				}
				wg.Done()
			}()

			// connect to listener
			ship, err := builder.LaunchShip(ctx, transport, localhost)
			if err != nil {
				t.Fatal(err)
			}

			// client send
			ok, err := ship.Load(testData)
			if err != nil {
				t.Fatalf("%s failed: %s", ship, err)
			}
			if !ok {
				t.Fatalf("%s sunk", ship)
			}

			// dock client
			request := <-requests
			if request.Err != nil {
				t.Fatalf("%s failed to dock: %s", request.Pier, request.Err)
			}
			srvShip := request.Ship

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

			for i := 0; i < 100; i++ {
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

				// client send
				ok, err = ship.Load(testData)
				if err != nil {
					t.Fatalf("%s failed: %s", ship, err)
				}
				if !ok {
					t.Fatalf("%s sunk", ship)
				}

				// server recv
				buf = getTestBuf()
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
			}

			ship.Sink()
			srvShip.Sink()
			pier.Abolish()
			wg.Wait() // wait for docking procedure to end

		})
	}
}
