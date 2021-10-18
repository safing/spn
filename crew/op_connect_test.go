package crew

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/network/packet"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

const (
	logTestCraneMsgs = false
	testPadding      = 8
	testQueueSize    = 10
)

func TestConnectOp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode, as it interacts with the network")
	}

	// Setup terminals.

	var testQueueSize uint16 = 10

	initMsg := &terminal.TerminalOpts{
		QueueSize: testQueueSize,
		Padding:   testPadding,
	}

	var term1 *TestTerminal
	var term2 *TestTerminal
	var initData *container.Container
	var tErr *terminal.Error
	term1, initData, tErr = NewLocalTestTerminal(
		module.Ctx, 127, "c1", nil, initMsg, createTestForwardingFunc(
			t, "c1", "c2", func(c *container.Container) *terminal.Error {
				return term2.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if tErr != nil {
		t.Fatalf("failed to create local terminal: %s", tErr)
	}
	term2, _, tErr = NewRemoteTestTerminal(
		module.Ctx, 127, "c2", nil, initData, createTestForwardingFunc(
			t, "c2", "c1", func(c *container.Container) *terminal.Error {
				return term1.DuplexFlowQueue.Deliver(c)
			},
		),
	)
	if tErr != nil {
		t.Fatalf("failed to create remote terminal: %s", tErr)
	}

	// Set up connect op.
	term2.GrantPermission(terminal.MayConnect)
	conf.EnablePublicHub(true)
	identity, err := cabin.CreateIdentity(module.Ctx, "test")
	if err != nil {
		t.Fatalf("failed to create identity: %s", err)
	}
	EnableConnecting(identity.Hub)

	for i := 0; i < 10; i++ {
		appConn, sluiceConn := net.Pipe()
		_, tErr = NewConnectOp(term1, &ConnectRequest{
			Domain:    "orf.at",
			IP:        net.IPv4(194, 232, 104, 142),
			Protocol:  packet.TCP,
			Port:      80,
			QueueSize: 100,
		}, sluiceConn)
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

type TestTerminal struct {
	// TODO: This is copy from the terminal package.
	// Find a nice way to have only one instance.

	*terminal.TerminalBase
	*terminal.DuplexFlowQueue
}

func NewLocalTestTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	remoteHub *hub.Hub,
	initMsg *terminal.TerminalOpts,
	submitUpstream func(*container.Container),
) (*TestTerminal, *container.Container, *terminal.Error) {
	// Create Terminal Base.
	t, initData, err := terminal.NewLocalBaseTerminal(ctx, id, parentID, remoteHub, initMsg)
	if err != nil {
		return nil, nil, err
	}

	return initTestTerminal(t, initMsg, submitUpstream), initData, nil
}

func NewRemoteTestTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	identity *cabin.Identity,
	initData *container.Container,
	submitUpstream func(*container.Container),
) (*TestTerminal, *terminal.TerminalOpts, *terminal.Error) {
	// Create Terminal Base.
	t, initMsg, err := terminal.NewRemoteBaseTerminal(ctx, id, parentID, identity, initData)
	if err != nil {
		return nil, nil, err
	}

	return initTestTerminal(t, initMsg, submitUpstream), initMsg, nil
}

func initTestTerminal(
	t *terminal.TerminalBase,
	initMsg *terminal.TerminalOpts,
	submitUpstream func(*container.Container),
) *TestTerminal {
	// Create Flow Queue.
	dfq := terminal.NewDuplexFlowQueue(t, initMsg.QueueSize, submitUpstream)

	// Create Crane Terminal and assign it as the extended Terminal.
	ct := &TestTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
	}
	t.SetTerminalExtension(ct)

	// Start workers.
	module.StartWorker("test terminal handler", ct.Handler)
	module.StartWorker("test terminal sender", ct.Sender)
	module.StartWorker("test terminal flow queue", ct.FlowHandler)

	return ct
}

func (t *TestTerminal) Flush() {
	t.TerminalBase.Flush()
}

func (t *TestTerminal) Abandon(err *terminal.Error) {
	if t.Abandoned.SetToIf(false, true) {
		switch err {
		case nil:
			// nil means that the Terminal is being shutdown by the owner.
			log.Tracef("spn/terminal: %s is closing", t.FmtID())
		default:
			// All other errors are faults.
			log.Warningf("spn/terminal: %s: %s", t.FmtID(), err)
		}

		// End all operations and stop all connected workers.
		t.Shutdown(nil, true)
	}
}

func createTestForwardingFunc(t *testing.T, srcName, dstName string, deliverFunc func(*container.Container) *terminal.Error) func(*container.Container) {
	return func(c *container.Container) {
		// Fast track nil containers.
		if c == nil {
			dErr := deliverFunc(c)
			if dErr != nil {
				t.Errorf("%s>%s: failed to deliver nil msg to terminal: %s", srcName, dstName, dErr)
			}
			return
		}

		// Log messages.
		if logTestCraneMsgs {
			t.Logf("%s>%s: %v\n", srcName, dstName, c.CompileData())
		}

		// Deliver to other terminal.
		dErr := deliverFunc(c)
		if dErr != nil {
			t.Errorf("%s>%s: failed to deliver to terminal: %s", srcName, dstName, dErr)
		}
	}
}
