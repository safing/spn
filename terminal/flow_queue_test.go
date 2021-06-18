package terminal

/*
type TestTerminal struct {
	*DuplexFlowQueue

	connected *TestTerminal

	ctx       context.Context
	cancelCtx context.CancelFunc

	sendBuffer chan *container.Container
	sent       int
	recvd      int

	T *testing.T
}

func (t *TestTerminal) submit(c *container.Container) {
	err := t.connected.DuplexFlowQueue.Deliver(c)
	if err != ErrNil {
		t.T.Fatalf("failed to submit to other end: %s", err)
	}
}

func (t *TestTerminal) handler() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-t.DuplexFlowQueue.Receive():
			t.recvd++
		case c := <-t.sendBuffer:
			t.DuplexFlowQueue.Send(c)
			t.sent++
		}
	}
}

func (t *TestTerminal) Init(testingT *testing.T, other *TestTerminal, qSize uint16) {
	t.T = testingT
	t.connected = other
	t.ctx, t.cancelCtx = context.WithCancel(context.Background())
	t.sendBuffer = make(chan *container.Container)
	t.DuplexFlowQueue = NewDuplexFlowQueue(t, qSize, t.submit)
	go t.handler()
}

func TestDuplexFlowQueue(t *testing.T) {
	// Setup Test Terminals.
	term1 := &TestTerminal{}
	term2 := &TestTerminal{}
	term1.Init(t, term2, 100)
	term2.Init(t, term1, 100)

	// Test one direction only.
	for i := 0; i < 1000; i++ {
		term1.sendBuffer <- container.New([]byte("The quick brown fox something something something"))
		term2.sendBuffer <- container.New([]byte("The quick brown fox something something something"))
	}

	term1.cancelCtx()
	term2.cancelCtx()
	t.Logf("term1: sent=%d recvd=%d", term1.sent, term1.recvd)
	t.Logf("term2: sent=%d recvd=%d", term2.sent, term2.recvd)
}
*/
