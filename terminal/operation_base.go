package terminal

import (
	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
)

// OpBase is a base for quickly building operations.
type OpBase struct {
	id    uint32
	ended *abool.AtomicBool
}

// ID returns the ID of the operation.
func (op *OpBase) ID() uint32 {
	return op.id
}

// SetID sets the ID of the operation.
func (op *OpBase) SetID(id uint32) {
	op.id = id
}

// HasEnded returns whether the operation has ended.
func (op *OpBase) HasEnded(end bool) bool {
	if end {
		// Return false if we just only it to ended.
		return !op.ended.SetToIf(false, true)
	}
	return op.ended.IsSet()
}

// Init initializes the operation base.
func (op *OpBase) Init() {
	op.ended = abool.New()
}

// OpBaseRequest is an extended operation base for request-like operations.
type OpBaseRequest struct {
	OpBase

	Delivered chan *container.Container
	Ended     chan *Error
}

// Init initializes the operation base.
func (op *OpBaseRequest) Init(deliverQueueSize int) {
	op.OpBase.Init()
	op.Delivered = make(chan *container.Container, deliverQueueSize)
	op.Ended = make(chan *Error, 1)
}

// Deliver delivers data to the operation.
func (op *OpBaseRequest) Deliver(data *container.Container) *Error {
	select {
	case op.Delivered <- data:
		return nil
	default:
		return ErrIncorrectUsage.With("request was not waiting for data")
	}
}

// End ends the operation.
func (op *OpBaseRequest) End(err *Error) (errorToSend *Error) {
	select {
	case op.Ended <- err:
	default:
	}
	return err
}
