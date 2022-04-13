package crew

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/safing/portmaster/intel"
	"github.com/safing/portmaster/network"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/terminal"
)

const (
	testPadding   = 8
	testQueueSize = 10
)

func TestConnectOp(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping test in short mode, as it interacts with the network")
	}

	// Create test terminal pair.
	a, b, err := terminal.NewSimpleTestTerminalPair(
		0,
		&terminal.TerminalOpts{
			QueueSize: testQueueSize,
			Padding:   testPadding,
		},
	)
	if err != nil {
		t.Fatalf("failed to create test terminal pair: %s", err)
	}

	// Set up connect op.
	b.GrantPermission(terminal.MayConnect)
	conf.EnablePublicHub(true)
	identity, err := cabin.CreateIdentity(module.Ctx, "test")
	if err != nil {
		t.Fatalf("failed to create identity: %s", err)
	}
	EnableConnecting(identity.Hub)

	for i := 0; i < 10; i++ {
		appConn, sluiceConn := net.Pipe()
		_, tErr := NewConnectOp(&Tunnel{
			connInfo: &network.Connection{
				Entity: &intel.Entity{
					Protocol: 6,
					Port:     80,
					Domain:   "orf.at.",
					IP:       net.IPv4(194, 232, 104, 142),
				},
			},
			conn:        sluiceConn,
			dstTerminal: a,
		})
		if tErr != nil {
			t.Fatalf("failed to start connect op: %s", tErr)
		}

		// Send request.
		requestURL, err := url.Parse("http://orf.at/")
		if err != nil {
			t.Fatalf("failed to parse request url: %s", err)
		}
		r := http.Request{
			Method: http.MethodHead,
			URL:    requestURL,
		}
		err = r.Write(appConn)
		if err != nil {
			t.Fatalf("failed to write request: %s", err)
		}

		// Recv response.
		data := make([]byte, 1500)
		n, err := appConn.Read(data)
		if err != nil {
			t.Fatalf("failed to read request: %s", err)
		}
		if n == 0 {
			t.Fatal("received empty reply")
		}

		t.Log("received data:")
		fmt.Println(string(data[:n]))

		time.Sleep(500 * time.Millisecond)
	}
}
