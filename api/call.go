package api

import (
	"github.com/tevino/abool"

	"github.com/Safing/safing-core/container"
)

type Call struct {
	Api       API
	Initiator bool
	ID        uint32

	Msgs chan *ApiMsg

	ended *abool.AtomicBool
}

func (call *Call) SendData(c *container.Container) {
	call.send(API_DATA, c)
}

func (call *Call) SendAck() {
	call.send(API_ACK, container.NewContainer(nil))
	call.End()
}

func (call *Call) SendError(msg string) {
	err := NewApiError(msg)
	call.send(API_ERR, container.NewContainer(err.Bytes()))
}

func (call *Call) SendTemporaryError(msg string) {
	err := NewApiError(msg).MarkAsTemporary()
	call.send(API_ERR, container.NewContainer(err.Bytes()))
}

func (call *Call) send(msgType uint8, c *container.Container) {
	if !call.ended.IsSet() {
		call.Api.Send(call.ID, msgType, c)
	}
}

func (call *Call) End() {
	call.ended.Set()
	call.Api.EndCall(call.ID)
}

func (call *Call) IsEnded() bool {
	return call.ended.IsSet()
}
