package terminal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/jess"
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/cabin"
)

const timeoutTicks = 5

// TerminalInterface is a generic interface to a terminal.
type TerminalInterface interface { //nolint:golint // Being explicit is helpful here.
	ID() uint32
	Ctx() context.Context
	Deliver(c *container.Container) *Error
	Abandon(err *Error)
	FmtID() string
	Flush()
}

// TerminalExtension is a generic interface to a terminal extension.
type TerminalExtension interface { //nolint:golint // Being explicit is helpful here.
	OpTerminal

	ReadyToSend() <-chan struct{}
	Send(c *container.Container) *Error
	SendRaw(c *container.Container) *Error
	Receive() <-chan *container.Container
	Abandon(err *Error)
}

// TerminalBase contains the basic functions of a terminal.
type TerminalBase struct { //nolint:golint,maligned // Being explicit is helpful here.
	// TODO: Fix maligned.

	lock sync.RWMutex

	// id is the underlying id of the Terminal.
	id uint32
	// id of the parent component.
	parentID string

	// ext holds the extended Terminal to supply the communication interface and
	// override behavior.
	ext TerminalExtension

	// ctx is the context of the Terminal.
	ctx context.Context
	// cancelCtx cancels ctx.
	cancelCtx context.CancelFunc

	// opMsgQueue is used by operations to submit messages for sending.
	opMsgQueue chan *container.Container
	// waitForFlush signifies if sending should be delayed until the next call
	// to Flush()
	waitForFlush *abool.AtomicBool
	// flush is used to send a finish function to the handler, which will write
	// all pending messages and then call the received function.
	flush chan func()
	// idleTicker ticks for increasing and checking the idle counter.
	idleTicker *time.Ticker
	// idleCounter counts the ticks the terminal has been idle.
	idleCounter *uint32

	// jession is the jess session used for encryption.
	jession *jess.Session
	// jessionLock locks jession.
	jessionLock sync.Mutex
	// encryptionReady is set when the encryption is ready for sending messages.
	encryptionReady chan struct{}
	// identity is the identity used by a remote Terminal.
	identity *cabin.Identity

	// operations holds references to all active operations that require persistence.
	operations map[uint32]Operation
	// nextOpID holds the next operation ID.
	nextOpID *uint32
	// permission holds the permissions of the terminal.
	permission Permission

	// opts holds the terminal options. It must not be modified after the terminal
	// has started.
	opts *TerminalOpts

	// Abandoning indicates if the Terminal is being abandoned. The main handlers
	// will keep running until the context has been canceled by the abandon
	// procedure.
	// No new operations should be started.
	// Whoever initiates the abandoning must also start the abandon procedure.
	Abandoning *abool.AtomicBool
}

func createTerminalBase(
	ctx context.Context,
	id uint32,
	parentID string,
	remote bool,
	initMsg *TerminalOpts,
) *TerminalBase {
	t := &TerminalBase{
		id:              id,
		parentID:        parentID,
		opMsgQueue:      make(chan *container.Container),
		waitForFlush:    abool.New(),
		flush:           make(chan func()),
		idleTicker:      time.NewTicker(time.Minute),
		idleCounter:     new(uint32),
		encryptionReady: make(chan struct{}),
		operations:      make(map[uint32]Operation),
		nextOpID:        new(uint32),
		opts:            initMsg,
		Abandoning:      abool.New(),
	}
	t.idleTicker.Stop() // Stop ticking to disable timeout.
	if remote {
		atomic.AddUint32(t.nextOpID, 4)
	}

	t.ctx, t.cancelCtx = context.WithCancel(ctx)
	return t
}

// ID returns the Terminal's ID.
func (t *TerminalBase) ID() uint32 {
	return t.id
}

// Ctx returns the Terminal's context.
func (t *TerminalBase) Ctx() context.Context {
	return t.ctx
}

// SetTerminalExtension sets the Terminal's extension. This function is not
// guarded and may only be used during initialization.
func (t *TerminalBase) SetTerminalExtension(ext TerminalExtension) {
	t.ext = ext
}

// SetTimeout sets the Terminal's idle timeout duration.
// It is broken down into slots internally.
func (t *TerminalBase) SetTimeout(d time.Duration) {
	t.idleTicker.Reset(d / timeoutTicks)
}

// Deliver on TerminalBase only exists to conform to the interface. It must be
// overridden by an actual implementation.
func (t *TerminalBase) Deliver(c *container.Container) *Error {
	return ErrIncorrectUsage
}

// Abandon abandons the Terminal with the given error.
func (t *TerminalBase) Abandon(err *Error) {
	panic("incorrect terminal base inheritance")
}

const (
	sendThresholdLength  = 100  // bytes
	sendMaxLength        = 4000 // bytes
	sendThresholdMaxWait = 20 * time.Millisecond
)

// Handler receives and handles messages and must be started as a worker in the
// module where the Terminal is used.
func (t *TerminalBase) Handler(_ context.Context) error {
	defer t.ext.Abandon(ErrInternalError.With("handler died"))

	for {
		select {
		case <-t.ctx.Done():
			// Call Abandon just in case.
			// Normally, the only the StopProcedure function should cancel the context.
			t.ext.Abandon(nil)
			return nil // Controlled worker exit.

		case <-t.idleTicker.C:
			// If nothing happens for a while, end the session.
			if atomic.AddUint32(t.idleCounter, 1) > timeoutTicks {
				// Abandon the terminal and reset the counter.
				t.ext.Abandon(ErrTimeout.With("no activity"))
				atomic.StoreUint32(t.idleCounter, 0)
			}

		case c := <-t.ext.Receive():
			if c.HoldsData() {
				err := t.handleReceive(c)
				if err != nil && !errors.Is(err, ErrStopping) {
					t.ext.Abandon(err.Wrap("failed to handle"))
				}
			}

			// Register activity.
			atomic.StoreUint32(t.idleCounter, 0)
		}
	}
}

// Sender handles sending messages and must be started as a worker in the
// module where the Terminal is used.
func (t *TerminalBase) Sender(_ context.Context) error {
	// Don't send messages, if the encryption is net yet set up.
	// The server encryption session is only initialized with the first
	// operative message, not on Terminal creation.
	if t.opts.Encrypt {
		select {
		case <-t.ctx.Done():
			// Call Abandon just in case.
			// Normally, the only the StopProcedure function should cancel the context.
			t.ext.Abandon(nil)
			return nil // Controlled worker exit.
		case <-t.encryptionReady:
		}
	}

	// Be sure to call Stop even in case of sudden death.
	defer t.ext.Abandon(ErrInternalError.With("sender died"))

	msgBuffer := container.New()
	var msgBufferLen int
	var msgBufferLimitReached bool
	var sendMsgs bool
	var sendMaxWait *time.Timer
	var flushFinished func()

	// Only receive message when not sending the current msg buffer.
	recvOpMsgs := func() <-chan *container.Container {
		// Don't handle more messages, if the buffer is full.
		if msgBufferLimitReached {
			return nil
		}
		return t.opMsgQueue
	}

	// Only wait for sending slot when the current msg buffer is ready to be sent.
	readyToSend := func() <-chan struct{} {
		if sendMsgs {
			return t.ext.ReadyToSend()
		}
		return nil
	}

	// Calculate current max wait time to send the msg buffer.
	getSendMaxWait := func() <-chan time.Time {
		if sendMaxWait != nil {
			return sendMaxWait.C
		}
		return nil
	}

handling:
	for {
		select {
		case <-t.ctx.Done():
			// Call Stop just in case.
			// Normally, the only the StopProcedure function should cancel the context.
			t.ext.Abandon(nil)
			return nil // Controlled worker exit.

		case <-t.idleTicker.C:
			// If nothing happens for a while, end the session.
			if atomic.AddUint32(t.idleCounter, 1) > timeoutTicks {
				// Abandon the terminal and reset the counter.
				t.ext.Abandon(ErrTimeout.With("no activity"))
				atomic.StoreUint32(t.idleCounter, 0)
			}

		case c := <-recvOpMsgs():
			// Add container to current buffer.
			msgBufferLen += c.Length()
			msgBuffer.AppendContainer(c)

			// Check if there is enough data to hit the sending threshold.
			if msgBufferLen >= sendThresholdLength {
				sendMsgs = true
			} else if sendMaxWait == nil && t.waitForFlush.IsNotSet() {
				sendMaxWait = time.NewTimer(sendThresholdMaxWait)
			}

			if msgBufferLen >= sendMaxLength {
				msgBufferLimitReached = true
			}

			// Register activity.
			atomic.StoreUint32(t.idleCounter, 0)

		case <-getSendMaxWait():
			// The timer for waiting for more data has ended.
			// Send all available data if not forced to wait for a flush.
			if t.waitForFlush.IsNotSet() {
				sendMsgs = true
			}

		case newFlushFinishedFn := <-t.flush:
			// We are flushing - stop waiting.
			t.waitForFlush.UnSet()

			// Signal immediately if msg buffer is empty.
			if msgBufferLen == 0 {
				newFlushFinishedFn()
			} else {
				// If there already is a flush finished function, stack them.
				if flushFinished != nil {
					stackedFlushFinishFn := flushFinished
					flushFinished = func() {
						stackedFlushFinishFn()
						newFlushFinishedFn()
					}
				} else {
					flushFinished = newFlushFinishedFn
				}
			}

			// Force sending data now.
			sendMsgs = true

		case <-readyToSend():
			// Reset sending flags.
			sendMsgs = false
			msgBufferLimitReached = false

			// Send if there is anything to send.
			if msgBufferLen > 0 {
				err := t.sendOpMsgs(msgBuffer)
				if err != nil {
					t.ext.Abandon(err.With("failed to send"))
					continue handling
				}
			}

			// Reset buffer.
			msgBuffer = container.New()
			msgBufferLen = 0

			// Reset send wait timer.
			if sendMaxWait != nil {
				sendMaxWait.Stop()
				sendMaxWait = nil
			}

			// Check if we are flushing and need to notify.
			if flushFinished != nil {
				flushFinished()
				flushFinished = nil
			}
		}
	}
}

// WaitForFlush makes the terminal pause all sending until the next call to
// Flush().
func (t *TerminalBase) WaitForFlush() {
	t.waitForFlush.Set()
}

// Flush sends all data waiting to be sent.
func (t *TerminalBase) Flush() {
	// Create channel and function for notifying.
	wait := make(chan struct{})
	finished := func() {
		close(wait)
	}
	// Request flush and return when stopping.
	select {
	case t.flush <- finished:
	case <-t.Ctx().Done():
		return
	}
	// Wait for flush to finish and return when stopping.
	select {
	case <-wait:
	case <-t.Ctx().Done():
	}
}

func (t *TerminalBase) encrypt(c *container.Container) (*container.Container, *Error) {
	if !t.opts.Encrypt {
		return c, nil
	}

	t.jessionLock.Lock()
	defer t.jessionLock.Unlock()

	letter, err := t.jession.Close(c.CompileData())
	if err != nil {
		return nil, ErrIntegrity.With("failed to encrypt: %w", err)
	}

	encryptedData, err := letter.ToWire()
	if err != nil {
		return nil, ErrInternalError.With("failed to pack letter: %w", err)
	}

	return encryptedData, nil
}

func (t *TerminalBase) decrypt(c *container.Container) (*container.Container, *Error) {
	if !t.opts.Encrypt {
		return c, nil
	}

	t.jessionLock.Lock()
	defer t.jessionLock.Unlock()

	letter, err := jess.LetterFromWire(c)
	if err != nil {
		return nil, ErrMalformedData.With("failed to parse letter: %w", err)
	}

	// Setup encryption if not yet done.
	if t.jession == nil {
		if t.identity == nil {
			return nil, ErrInternalError.With("missing identity for setting up incoming encryption")
		}

		// Create jess session.
		t.jession, err = letter.WireCorrespondence(t.identity)
		if err != nil {
			return nil, ErrIntegrity.With("failed to initialize incoming encryption: %w", err)
		}

		// Don't need that anymore.
		t.identity = nil

		// Encryption is ready for sending.
		close(t.encryptionReady)
	}

	decryptedData, err := t.jession.Open(letter)
	if err != nil {
		return nil, ErrIntegrity.With("failed to decrypt: %w", err)
	}

	return container.New(decryptedData), nil
}

func (t *TerminalBase) handleReceive(c *container.Container) *Error {
	// Debugging:
	// log.Errorf("terminal %s handling tmsg: %s", t.FmtID(), spew.Sdump(c.CompileData()))

	// Check if message is empty. This will be the case if a message was only
	// for updated the available space of the flow queue.
	if !c.HoldsData() {
		return nil
	}

	// Decrypt if enabled.
	var tErr *Error
	c, tErr = t.decrypt(c)
	if tErr != nil {
		return tErr
	}

	// Handle operation messages.
	for c.HoldsData() {
		// Get next messagt length.
		msgLength, err := c.GetNextN32()
		if err != nil {
			return ErrMalformedData.With("failed to get operation msg length: %w", err)
		}
		if msgLength == 0 {
			// Remainder is padding.
			// Padding can only be at the end of the segment.
			t.handlePaddingMsg(c)
			return nil
		}

		// Get op msg data.
		msgData, err := c.GetAsContainer(int(msgLength))
		if err != nil {
			return ErrMalformedData.With("failed to get operation msg data: %w", err)
		}

		// Handle op msg.
		if handleErr := t.handleOpMsg(msgData); handleErr != nil {
			return handleErr
		}
	}

	return nil
}

func (t *TerminalBase) handleOpMsg(data *container.Container) *Error {
	// Debugging:
	// log.Errorf("terminal %s handling opmsg: %s", t.FmtID(), spew.Sdump(data.CompileData()))

	// Parse message operation id, type.
	opID, msgType, err := ParseIDType(data)
	if err != nil {
		return ErrMalformedData.With("failed to parse operation msg id/type: %w", err)
	}

	switch msgType {
	case MsgTypeInit:
		t.runOperation(t.ctx, t.ext, opID, data)

	case MsgTypeData:
		op, ok := t.GetActiveOp(opID)
		if ok {
			err := op.Deliver(data)
			if err != nil {
				if err.IsOK() {
					t.OpEnd(op, err)
				} else {
					t.OpEnd(op, err.Wrap("data delivery failed"))
				}
			}
		} else {
			// If an active op is not found, this is likely just left-overs from a
			// ended or failed operation.
			log.Tracef("spn/terminal: %s received data msg for unknown op %d", fmtTerminalID(t.parentID, t.id), opID)
		}

	case MsgTypeStop:
		// Parse received error.
		opErr, parseErr := ParseExternalError(data.CompileData())
		if parseErr != nil {
			log.Warningf("spn/terminal: %s failed to parse end error: %s", fmtTerminalID(t.parentID, t.id), parseErr)
			opErr = ErrUnknownError.AsExternal()
		}

		// End operation.
		op, ok := t.GetActiveOp(opID)
		if ok {
			t.OpEnd(op, opErr)
		} else {
			log.Tracef("spn/terminal: %s received end msg for unknown op %d", fmtTerminalID(t.parentID, t.id), opID)
		}

	default:
		log.Warningf("spn/terminal: %s received unexpected message type: %d", t.FmtID(), msgType)
		return ErrUnexpectedMsgType
	}

	return nil
}

func (t *TerminalBase) handlePaddingMsg(c *container.Container) {
	padding := c.GetAll()
	if len(padding) > 0 {
		rngFeeder.SupplyEntropyIfNeeded(padding, len(padding))
	}
}

func (t *TerminalBase) sendOpMsgs(c *container.Container) *Error {
	if t.opts.Padding > 0 {
		// Add Padding if needed.
		paddingNeeded := (int(t.opts.Padding) - c.Length()) % int(t.opts.Padding)
		if paddingNeeded > 0 {
			// Add padding message header.
			c.Append([]byte{0})
			paddingNeeded--

			// Add needed padding data.
			if paddingNeeded > 0 {
				padding, err := rng.Bytes(paddingNeeded)
				if err != nil {
					log.Debugf("terminal: %s failed to get random data, using zeros instead", t.FmtID())
					padding = make([]byte, paddingNeeded)
				}
				c.Append(padding)
			}
		}
	}

	// Encrypt operative data.
	var tErr *Error
	c, tErr = t.encrypt(c)
	if tErr != nil {
		return tErr
	}

	// Send data.
	return t.ext.Send(c)
}

func (t *TerminalBase) addToOpMsgSendBuffer(
	opID uint32,
	msgType MsgType,
	data *container.Container,
	timeout time.Duration,
) *Error {
	// Add header.
	MakeMsg(data, opID, msgType)

	// Prepare submit timeout.
	var submitTimeout <-chan time.Time
	if timeout > 0 {
		submitTimeout = time.After(timeout)
	}

	// Submit message to buffer, if space is available.
	select {
	case t.opMsgQueue <- data:
		return nil
	case <-submitTimeout:
		return ErrTimeout.With("op msg send timeout")
	case <-t.ctx.Done():
		return ErrStopping
	}
}

// StartAbandonProcedure sends a stop message with the given error if wanted, ends
// all operations with a nil error, executes the given finalizeFunc and finally
// cancels the terminal context. This function is usually not called directly,
// but at the end of an Abandon() implementation.
func (t *TerminalBase) StartAbandonProcedure(err *Error, sendError bool, finalizeFunc func()) {
	module.StartWorker("terminal abandon procedure", func(_ context.Context) error {
		t.handleAbandonProcedure(err, sendError, finalizeFunc)
		return nil
	})
}

func (t *TerminalBase) handleAbandonProcedure(err *Error, sendError bool, finalizeFunc func()) {
	// End all operations.
	for _, op := range t.allOps() {
		t.OpEnd(op, nil)
	}

	// Wait 20s for all operations to end.
	// TODO: Use a signal for this instead of polling.
	for i := 1; i <= 1000 && t.GetActiveOpCount() > 0; i++ {
		time.Sleep(20 * time.Millisecond)
		if i == 1000 {
			log.Warningf(
				"spn/terminal: terminal %s is continuing shutdown with %d active operations",
				t.ext.FmtID(),
				t.GetActiveOpCount(),
			)
		}
	}

	if sendError {
		stopMsg := container.New(err.Pack())
		MakeMsg(stopMsg, t.id, MsgTypeStop)

		tErr := t.ext.SendRaw(stopMsg)
		if tErr != nil {
			log.Warningf("spn/terminal: terminal %s failed to send stop msg: %s", t.ext.FmtID(), tErr)
		}
	}

	// Call specialized finalizing function.
	if finalizeFunc != nil {
		finalizeFunc()
	}

	// Flush all messages before stopping.
	t.ext.Flush()

	// Stop all other connected workers.
	t.cancelCtx()
	t.idleTicker.Stop()
}

func (t *TerminalBase) allOps() []Operation {
	t.lock.Lock()
	defer t.lock.Unlock()

	ops := make([]Operation, 0, len(t.operations))
	for _, op := range t.operations {
		ops = append(ops, op)
	}

	return ops
}
