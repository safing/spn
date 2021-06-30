package terminal

import (
	"context"
	"sync"
	"time"

	"github.com/safing/portbase/rng"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
	"github.com/tevino/abool"
)

type TerminalExtension interface {
	ReadyToSend() <-chan struct{}
	Send(c *container.Container) Error
	Receive() <-chan *container.Container
	Abandon(action string, err Error)
}

type TerminalInterface interface {
	ID() uint32
	Ctx() context.Context
	Deliver(c *container.Container) Error
	End(action string, err Error)
}

// TerminalBase contains the basic functions of a terminal.
type TerminalBase struct {
	lock sync.RWMutex

	// id is the underlying id of the Terminal.
	id uint32
	// id of the parent component.
	parentID string

	// ext holds the extended Terminal to supply the communciation interface and
	// override behavior.
	ext TerminalExtension

	// ctx is the context of the Terminal.
	ctx context.Context
	// cancelCtx cancels ctx.
	cancelCtx context.CancelFunc

	// connector is the interface to the underlying communication channel.
	opMsgQueue chan *container.Container
	// waitForFlush signifies if sending should be delayed until the next call
	// to Flush()
	waitForFlush *abool.AtomicBool
	// flush is used to send a finish function to the handler, which will write
	// all pending messages and then call the received function.
	flush chan func()

	// Encryption
	// FIXME

	// operations holds references to all active operations that require persistence.
	operations map[uint32]Operation
	// nextOpID holds the next operation ID.
	nextOpID *uint32
	// permission holds the permissions of the terminal.
	permission Permission

	// abandoned indicates if the Terminal has been abandoned. Whoever abandoned
	// the terminal already took care of notifying everyone, so a silent fail is
	// normally the best response.
	abandoned *abool.AtomicBool

	// server signifies if this Terminal is remote. This controls in which
	// direction errors are forwarded.
	// server bool
}

func NewTerminalBase(ctx context.Context, id uint32, parentID string, initialData *container.Container) *TerminalBase {
	t := &TerminalBase{
		id:           id,
		parentID:     parentID,
		opMsgQueue:   make(chan *container.Container),
		waitForFlush: abool.New(),
		flush:        make(chan func()),
		operations:   make(map[uint32]Operation),
		nextOpID:     new(uint32),
		abandoned:    abool.New(),
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

// Deliver on TerminalBase only exists to conform to the interface. It must be
// overridden by an actual implementation.
func (t *TerminalBase) Deliver(c *container.Container) Error {
	return ErrInvalidConfiguration
}

// End shuts down the Terminal with the given error.
func (t *TerminalBase) End(action string, err Error) {
	t.ext.Abandon(action, err)
}

const (
	sendThresholdLength  = 100  // bytes
	sendMaxLength        = 4000 // bytes
	sendThresholdMaxWait = 20 * time.Millisecond
)

var addPaddingTo = 100 // bytes

func (t *TerminalBase) handler(_ context.Context) error {
	defer t.ext.Abandon("internal error", ErrUnknownError)

	msgBuffer := container.New()
	var msgBufferLen int
	var msgBufferLimitReached bool
	var sendMsgs bool
	var sendMaxWait *time.Timer
	var flushFinished func()

	// Only receive message when not sending the current msg buffer.
	recvOpMsgs := func() <-chan *container.Container {
		if !msgBufferLimitReached {
			return t.opMsgQueue
		}
		return nil
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
			t.ext.Abandon("", ErrNil)
			return nil // Controlled worker exit.

		case <-time.After(10 * time.Second):
			// If nothing happens for 10 seconds, end the session.
			log.Debugf("terminal: %s#%d timed out: shutting down", t.parentID, t.id)
			t.ext.Abandon("", ErrNil)
			return nil // Controlled worker exit.

		case c := <-t.ext.Receive():
			for c.HoldsData() {
				err := t.handleReceive(c)
				switch err {
				case ErrNil:
					// Continue.
				case ErrAbandoning:
					return nil // Controlled worker exit.
				default:
					t.ext.Abandon("failed to receive", err)
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
				if err != ErrNil {
					t.ext.Abandon("failed to send", err)
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

func (t *TerminalBase) handleReceive(c *container.Container) Error {
	msgType, err := ParseTerminalMsgType(c)
	if err != nil {
		return ErrMalformedData
	}

	switch msgType {
	case MsgTypeNone:
		// Message was just for updating the flow queue.
		return ErrNil

	case MsgTypeOperativeData:

		// FIXME: Decrypt

		for c.HoldsData() {
			// Handle operative message.
			if handleErr := t.handleOpMsg(c); handleErr != ErrNil {
				return handleErr
			}
		}

	case MsgTypeAbandon:
		tErr := Error(c.CompileData())
		switch err {
		case ErrNil:
			t.ext.Abandon("", ErrNil)
		default:
			t.ext.Abandon("received error", tErr)
		}
		return ErrAbandoning

	default:
		return ErrUnexpectedMsgType
	}

	return ErrNil
}

func (t *TerminalBase) handleOpMsg(c *container.Container) Error {
	// Parse message type, operation id and data.
	msgType, err := ParseOpMsgType(c)
	if err != nil {
		return ErrMalformedData
	}
	// Check if this is a padding message and handle it specially.
	if msgType == MsgTypePadding {
		t.handlePaddingMsg(c)
		return ErrNil
	}

	// Parse operation id and data.
	opID, err := c.GetNextN32()
	if err != nil {
		return ErrMalformedData
	}
	data, err := c.GetNextBlockAsContainer()
	if err != nil {
		return ErrMalformedData
	}

	switch OpMsgType(msgType) {
	case MsgTypeInit:
		t.runOperation(t.ctx, t, opID, data)

	case MsgTypeData:
		op, ok := t.GetActiveOp(opID)
		if ok {
			err := op.Deliver(data)
			if err != ErrNil {
				t.endOperation(op, "data delivery failed", err, true, true)
			}
		} else {
			// If an active op is not found, this is likely just left-overs from a
			// ended or failed operation.
			log.Tracef("terminal: %s received msg for unknown op %d", fmtTerminalID(t.parentID, t.id), opID)
		}

	case MsgTypeEnd:
		op, ok := t.GetActiveOp(opID)
		if ok {
			t.endOperation(op, "received error", Error(data.CompileData()), true, false)
		} else {
			log.Tracef("terminal: %s received end msg for unknown op %d", fmtTerminalID(t.parentID, t.id), opID)
		}

	default:
		return ErrUnexpectedMsgType
	}

	return ErrNil
}

func (t *TerminalBase) handlePaddingMsg(c *container.Container) {
	padding := c.GetAll()
	if len(padding) > 0 {
		rngFeeder.SupplyEntropyIfNeeded(padding, len(padding))
	}
}

func (t *TerminalBase) sendOpMsgs(c *container.Container) Error {
	if addPaddingTo > 0 {
		// Add Padding if needed.
		paddingNeeded := (addPaddingTo - c.Length()) % addPaddingTo
		if paddingNeeded > 0 {
			// Add padding message header.
			c.Append(MsgTypePadding.Pack())
			paddingNeeded--

			// Add needed padding data.
			if paddingNeeded > 0 {
				padding, err := rng.Bytes(paddingNeeded)
				if err != nil {
					log.Debugf("terminal: failed to get random data, using zeros instead")
					padding = make([]byte, paddingNeeded)
				}
				c.Append(padding)
			}
		}
	}

	// FIXME: Encrypt

	// Add Terminal Message type.
	c.Prepend(MsgTypeOperativeData.Pack())

	// Send data.
	return t.ext.Send(c)
}

func (t *TerminalBase) sendTerminalMsg(
	msgType TerminalMsgType,
	data *container.Container,
) Error {
	if data != nil {
		data.Prepend(msgType.Pack())
	} else {
		data = container.New(msgType.Pack())
	}

	return t.ext.Send(data)
}

func (t *TerminalBase) addToOpMsgSendBuffer(
	opID uint32,
	msgType OpMsgType,
	data *container.Container,
	async bool,
) Error {
	if data != nil {
		// Add message metadata.
		data.PrependLength()
		data.Prepend(varint.Pack32(opID))
		data.Prepend(msgType.Pack())
	} else {
		// Or create new message
		data = container.New(
			msgType.Pack(),
			varint.Pack32(opID),
			varint.Pack8(0),
		)
	}

	// Submit message to buffer and fall back to async.
	select {
	case t.opMsgQueue <- data:
		return ErrNil
	case <-t.ctx.Done():
		return ErrAbandoning
	default:
		if async {
			// Operative Message Queue is full, delay sending.
			// TODO: Find better way of handling this.
			module.StartWorker("delayed operative message queuing", func(ctx context.Context) error {
				select {
				case t.opMsgQueue <- data:
				case <-t.ctx.Done():
				case <-ctx.Done():
				}
				return nil
			})
			return ErrNil
		}
	}

	// Submit message to buffer and, and wait forever.
	select {
	case t.opMsgQueue <- data:
		return ErrNil
	case <-t.ctx.Done():
		return ErrAbandoning
	}
}
