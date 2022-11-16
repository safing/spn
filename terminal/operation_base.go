package terminal

import (
	"time"

	"github.com/tevino/abool"
)

// OperationBase provides the basic operation functionality.
type OperationBase struct {
	id       uint32
	terminal Terminal
	stopped  abool.AtomicBool
}

// InitOperationBase initialize the operation with the ID and attached terminal.
// Should not be overridden by implementations.
func (op *OperationBase) InitOperationBase(t Terminal, opID uint32) {
	op.id = opID
	op.terminal = t
}

// ID returns the ID of the operation.
// Should not be overridden by implementations.
func (op *OperationBase) ID() uint32 {
	return op.id
}

// Type returns the operation's type ID.
// Should be overridden by implementations to return correct type ID.
func (op *OperationBase) Type() string {
	return "unknown"
}

// Deliver delivers a message to the operation.
// Meant to be overridden by implementations.
func (op *OperationBase) Deliver(_ *Msg) *Error {
	return ErrInternalError.With("Deliver not implemented")
}

// NewMsg creates a new message from this operation.
// Should not be overridden by implementations.
func (op *OperationBase) NewMsg(data []byte) *Msg {
	msg := NewMsg(data)
	return msg
}

// Send sends a message to the other side.
// Should not be overridden by implementations.
func (op *OperationBase) Send(msg *Msg, timeout time.Duration) *Error {
	// Wait for processing slot.
	msg.WaitForUnitSlot()

	// Add and update metadata.
	msg.FlowID = op.id
	if msg.Type == MsgTypeData && msg.IsHighPriorityUnit() && UsePriorityDataMsgs {
		msg.Type = MsgTypePriorityData
	}

	// Send message.
	tErr := op.terminal.Send(msg, timeout)
	if tErr.IsError() {
		// Finish message unit on failure.
		msg.FinishUnit()
	}
	return tErr
}

// Stopped returns whether the operation has stopped.
// Should not be overridden by implementations.
func (op *OperationBase) Stopped() bool {
	return op.stopped.IsSet()
}

// markStopped marks the operation as stopped.
// It returns whether the stop flag was set.
func (op *OperationBase) markStopped() bool {
	return op.stopped.SetToIf(false, true)
}

// Stop stops the operation by unregistering it from the terminal and calling HandleStop().
// Should not be overridden by implementations.
func (op *OperationBase) Stop(self Operation, err *Error) {
	// Stop operation from terminal.
	op.terminal.StopOperation(self, err)
}

// HandleStop gives the operation the ability to cleanly shut down.
// The returned error is the error to send to the other side.
// Should never be called directly. Call Stop() instead.
// Meant to be overridden by implementations.
func (op *OperationBase) HandleStop(err *Error) (errorToSend *Error) {
	return err
}

// RequestOperationBase is an operation base for request-like operations.
type RequestOperationBase struct {
	OperationBase

	Delivered chan *Msg
	Ended     chan *Error
}

// Init initializes the operation base.
func (op *RequestOperationBase) Init(deliverQueueSize int) {
	op.Delivered = make(chan *Msg, deliverQueueSize)
	op.Ended = make(chan *Error, 1)
}

// Deliver delivers data to the operation.
func (op *RequestOperationBase) Deliver(msg *Msg) *Error {
	select {
	case op.Delivered <- msg:
		return nil
	default:
		return ErrIncorrectUsage.With("request was not waiting for data")
	}
}

// End ends the operation.
func (op *RequestOperationBase) End(err *Error) (errorToSend *Error) {
	select {
	case op.Ended <- err:
	default:
	}
	return err
}
