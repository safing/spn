package terminal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/spn/cabin"

	"github.com/safing/jess"

	"github.com/safing/portbase/rng"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/tevino/abool"
)

const (
	timeoutTicks = 5
)

type TerminalInterface interface {
	ID() uint32
	Ctx() context.Context
	Deliver(c *container.Container) *Error
	Abandon(err *Error)
	FmtID() string
}

type TerminalExtension interface {
	OpTerminal

	ReadyToSend() <-chan struct{}
	Send(c *container.Container) *Error
	SendRaw(c *container.Container) *Error
	Receive() <-chan *container.Container
	Abandon(err *Error)
}

// TerminalBase contains the basic functions of a terminal.
type TerminalBase struct {
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

	// Abandoned indicates if the Terminal has been abandoned. Whoever abandoned
	// the terminal already took care of notifying everyone, so a silent fail is
	// normally the best response.
	Abandoned *abool.AtomicBool
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
		Abandoned:       abool.New(),
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
			t.ext.Abandon(nil)
			return nil // Controlled worker exit.

		case <-t.idleTicker.C:
			// If nothing happens for a while, end the session.
			if atomic.AddUint32(t.idleCounter, 1) > timeoutTicks {
				t.ext.Abandon(ErrTimeout.With("no activity"))
				return nil // Controlled worker exit.
			}

		case c := <-t.ext.Receive():
			if c.HoldsData() {
				err := t.handleReceive(c)
				if err != nil {
					if !errors.Is(err, ErrStopping) {
						t.ext.Abandon(err.Wrap("failed to handle"))
					}
					return nil // Controlled worker exit.
				}
				switch err {
				case nil:
					// Continue.
				case ErrStopping:
				default:
					return nil // Controlled worker exit.
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
			t.ext.Abandon(nil)
			return nil // Controlled worker exit.
		case <-t.encryptionReady:
		}
	}

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

	for {
		select {
		case <-t.ctx.Done():
			t.ext.Abandon(nil)
			return nil // Controlled worker exit.

		case <-t.idleTicker.C:
			// If nothing happens for a while, end the session.
			if atomic.AddUint32(t.idleCounter, 1) > timeoutTicks {
				t.ext.Abandon(ErrTimeout.With("no activity"))
				return nil // Controlled worker exit.
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
					return nil // Controlled worker exit.
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
	// Create channel for notifying.
	wait := make(chan struct{})
	// Request flush and send close function.
	t.flush <- func() {
		close(wait)
	}
	// Wait for handler to finish flushing.
	<-wait
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
				if err.IsSpecial() {
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

// Shutdown sends a stop message with the given error (if it is external) and
// ends all operations with a nil error and finally cancels the terminal
// context. This function is usually not called directly, but at the end of an
// Abandon() implementation.
func (t *TerminalBase) Shutdown(err *Error, sendError bool) {
	// End all operations.
	for _, op := range t.allOps() {
		op.End(nil)
	}

	if sendError {
		stopMsg := container.New(err.Pack())
		MakeMsg(stopMsg, t.id, MsgTypeStop)

		tErr := t.ext.SendRaw(stopMsg)
		if tErr != nil {
			log.Warningf("spn/terminal: terminal %s failed to send stop msg: %s", t.ext.FmtID(), tErr)
		}
	}

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
