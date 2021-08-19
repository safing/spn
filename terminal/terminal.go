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
	// DefaultTerminalTimeout is the default time duration after which an idle
	// terminal session times out and is abandoned.
	DefaultTerminalTimeout = 1 * time.Minute
)

type TerminalInterface interface {
	ID() uint32
	Ctx() context.Context
	Deliver(c *container.Container) *Error
	Abandon(err *Error)
	FmtID() string
}

type TerminalExtension interface {
	ReadyToSend() <-chan struct{}
	Send(c *container.Container) *Error
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

	// jession is the jess session used for encryption.
	jession *jess.Session
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
		id:           id,
		parentID:     parentID,
		opMsgQueue:   make(chan *container.Container),
		waitForFlush: abool.New(),
		flush:        make(chan func()),
		operations:   make(map[uint32]Operation),
		nextOpID:     new(uint32),
		opts:         initMsg,
		Abandoned:    abool.New(),
	}
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

// Handler handles all Terminal internals and must be started as a worker in
// the module where the Terminal is used.
func (t *TerminalBase) Handler(_ context.Context) error {
	defer t.ext.Abandon(ErrInternalError.With("handler died"))

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
		// Don't handle messages, if the encryption is net yet set up.
		// The server encryption session is only initialized with the first
		// operative message, not on Terminal creation.
		if t.opts.Encrypt && t.jession == nil {
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

		case <-time.After(DefaultTerminalTimeout):
			// If nothing happens for a while, end the session.
			log.Debugf("spn/terminal: %s timed out: shutting down", t.FmtID())
			t.ext.Abandon(nil)
			return nil // Controlled worker exit.

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

func (t *TerminalBase) handleReceive(c *container.Container) *Error {
	// Debugging:
	// log.Errorf("terminal %s handling tmsg: %s", t.FmtID(), spew.Sdump(c.CompileData()))

	// Check if message is empty. This will be the case if a message was only
	// for updated the available space of the flow queue.
	if !c.HoldsData() {
		return nil
	}

	// Decrypt if enabled.
	if t.opts.Encrypt {
		letter, err := jess.LetterFromWire(c)
		if err != nil {
			return ErrMalformedData.With("failed to parse letter: %w", err)
		}

		// Setup encryption if not yet done.
		if t.jession == nil {
			if t.identity == nil {
				return ErrInternalError.With("missing identity for setting up incoming encryption")
			}

			// Create jess session.
			t.jession, err = letter.WireCorrespondence(t.identity)
			if err != nil {
				return ErrIntegrity.With("failed to initialize incoming encryption: %w", err)
			}

			// Don't need that anymore.
			t.identity = nil
		}

		decryptedData, err := t.jession.Open(letter)
		if err != nil {
			return ErrIntegrity.With("failed to decrypt: %w", err)
		}

		c = container.New(decryptedData)
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
		t.runOperation(t.ctx, t, opID, data)

	case MsgTypeData:
		op, ok := t.GetActiveOp(opID)
		if ok {
			err := op.Deliver(data)
			if err != nil {
				t.OpEnd(op, err.With("data delivery failed"))
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
	if t.opts.Encrypt {
		letter, err := t.jession.Close(c.CompileData())
		if err != nil {
			return ErrIntegrity.With("failed to encrypt: %w", err)
		}

		c, err = letter.ToWire()
		if err != nil {
			return ErrInternalError.With("failed to pack letter: %w", err)
		}
	}

	// Send data.
	return t.ext.Send(c)
}

func (t *TerminalBase) addToOpMsgSendBuffer(
	opID uint32,
	msgType MsgType,
	data *container.Container,
	async bool,
) *Error {
	// Add header.
	MakeMsg(data, opID, msgType)

	// Submit message to buffer and fall back to async.
	select {
	case t.opMsgQueue <- data:
		return nil
	case <-t.ctx.Done():
		return ErrStopping
	default:
		if async {
			// Operative Message Queue is full, delay sending.
			// TODO: Find better way of handling this.
			module.StartWorker("delayed operative message queuing", func(ctx context.Context) error {
				select {
				case t.opMsgQueue <- data:
				case <-t.ctx.Done():
				}
				return nil
			})
			return nil
		}
	}

	// Wait forever to submit message to buffer.
	select {
	case t.opMsgQueue <- data:
		return nil
	case <-t.ctx.Done():
		return ErrStopping
	}
}

// StopAll ends all operations with the given paramaters and cancels the
// workers. This function is usually not called directly, but at the end of an
// Abandon() implementation.
func (t *TerminalBase) StopAll(err *Error) {
	// End all operations.
	for _, op := range t.allOps() {
		op.End(err)
	}

	// Stop all connected workers.
	t.cancelCtx()
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
