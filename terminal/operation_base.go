package terminal

import "github.com/safing/portbase/container"

type OpBase struct {
	id uint32
}

func (op *OpBase) ID() uint32 {
	return op.id
}

func (op *OpBase) SetID(id uint32) {
	op.id = id
}

type OpBaseRequest struct {
	OpBase

	Delivered chan *container.Container
	Ended     chan *Error
}

func (op *OpBaseRequest) Init(deliverQueueSize int) {
	op.Delivered = make(chan *container.Container, deliverQueueSize)
	op.Ended = make(chan *Error, 1)
}

func (op *OpBaseRequest) Deliver(data *container.Container) *Error {
	select {
	case op.Delivered <- data:
		return ErrIncorrectUsage.With("request was not waiting for data")
	default:
		return ErrQueueOverflow
	}
}

func (op *OpBaseRequest) End(err *Error) {
	select {
	case op.Ended <- err:
	default:
	}
}
