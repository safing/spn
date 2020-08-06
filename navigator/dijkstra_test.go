package navigator

import (
	"fmt"
	"os"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/safing/spn/conf"

	"github.com/safing/spn/docks"
)

func TestDijkstra(t *testing.T) {

	finished := make(chan struct{})
	go func() {
		// wait for test to complete, panic after timeout
		time.Sleep(3 * time.Second)
		select {
		case <-finished:
		default:
			t.Log("===== TAKING TOO LONG FOR TEST - PRINTING STACK TRACES =====")
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			os.Exit(1)
		}
	}()

	collection := buildTestNet()
	var lock sync.Mutex

	// t.Log("printing collection:")
	// for _, entry := range collection {
	// 	t.Logf("%s\n", entry)
	// }
	// t.Log("--------------------")

	d := NewMap(collection["1"], collection, &lock)

	// imitate active connection to 3
	p3 := collection["3"]
	p3.ActiveAPI = docks.NewAPI(conf.CurrentVersion, false, true)
	p3.ActiveRoute = []*Port{
		collection["1"],
		collection["2"],
		p3,
	}

	// imitate active, but bad connection to 5
	p5 := collection["5"]
	p5.ActiveAPI = docks.NewAPI(conf.CurrentVersion, false, true)
	p5.ActiveRoute = []*Port{
		collection["1"],
		p5,
	}

	// t.Logf("1: %v\n", collection["1"])
	// t.Logf("5: %v\n", p5)
	//
	// t.Logf("5:ARC: %d\n", p5.ActiveRouteCost())
	// t.Logf("5:AC: %d\n", p5.ActiveCost())

	// test path without considering active routes
	testPath(t, d, []uint8{5}, []uint8{1, 4, 6, 5}, false)
	testPath(t, d, []uint8{10}, []uint8{1, 4, 3, 9, 10}, false)
	testPath(t, d, []uint8{11}, []uint8{1, 4, 3, 9, 8, 11}, false)
	testPath(t, d, []uint8{12}, []uint8{1, 4, 6, 7, 12}, false)
	testPath(t, d, []uint8{10, 11, 12}, []uint8{1, 4, 3, 9, 8, 11}, false)

	// now consider active routes
	testPath(t, d, []uint8{5}, []uint8{1, 4, 6, 5}, true)
	testPath(t, d, []uint8{10}, []uint8{3, 9, 10}, true)
	testPath(t, d, []uint8{11}, []uint8{3, 9, 8, 11}, true)
	testPath(t, d, []uint8{12}, []uint8{3, 9, 8, 7, 12}, true)
	testPath(t, d, []uint8{10, 11, 12}, []uint8{3, 9, 8, 11}, true)

	close(finished)

	// let all the logs get out before we might fail
	// time.Sleep(100 * time.Millisecond)

}

func testPath(t *testing.T, m *Map, dests []uint8, expectedPath []uint8, considerActiveRoutes bool) {
	var destPorts []*Port
	for _, destID := range dests {
		destPorts = append(destPorts, m.Collection[fmt.Sprintf("%d", destID)])
	}

	path, ok, err := m.FindShortestPath(considerActiveRoutes, destPorts...)

	if !ok {
		t.Errorf("unable to find route")
	}
	if err != nil {
		t.Errorf("error finding route: %s", err)
	}

	// TODO: fix
	_ = path
	// err = comparePath(path, dests, expectedPath, considerActiveRoutes)
	// if err != nil {
	// 	t.Error(err)
	// }
}
