package api

import "github.com/safing/portbase/container"

const (
	API_CALL uint8 = iota // initiate
	API_ACK               // ack and stop
	API_DATA              // data
	API_ERR               // error, must not mean stop
	API_END               // silent stop
)

type API interface {
	Init(server, initiator bool, fromShip, toShip chan *container.Container)
	Run()
	Send(id uint32, msgType uint8, c *container.Container)
	EndCall(id uint32)
}

type ApiHandler func(call *Call, c *container.Container)
