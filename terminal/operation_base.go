package terminal

import (
	"github.com/safing/portbase/container"
	"github.com/tevino/abool"
)

type OpBase struct {
	id    uint32
	ended *abool.AtomicBool
}

func (op *OpBase) ID() uint32 {
	return op.id
}

func (op *OpBase) SetID(id uint32) {
	op.id = id
}

func (op *OpBase) HasEnded(end bool) bool {
	if end {
		// Return false if we just only it to ended.
		return !op.ended.SetToIf(false, true)
	}
	return op.ended.IsSet()
}

func (op *OpBase) Init() {
	op.ended = abool.New()
}

type OpBaseRequest struct {
	OpBase

	Delivered chan *container.Container
	Ended     chan *Error
}

func (op *OpBaseRequest) Init(deliverQueueSize int) {
	op.OpBase.Init()
	op.Delivered = make(chan *container.Container, deliverQueueSize)
	op.Ended = make(chan *Error, 1)
}

func (op *OpBaseRequest) Deliver(data *container.Container) *Error {
	select {
	case op.Delivered <- data:
		return nil
	default:
		return ErrIncorrectUsage.With("request was not waiting for data")
	}
}

func (op *OpBaseRequest) End(err *Error) {
	select {
	case op.Ended <- err:
	default:
	}
}
