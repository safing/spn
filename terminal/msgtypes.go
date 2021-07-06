package terminal

import (
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
)

/*

Terminal Message Format:

- ID [varint; by Crane]
- AddAvailableSpace [varint; by Flow Queue]
- MsgType [varint; by Terminal; one of None, Failure, Shutdown, OperativeData]
- Data [bytes; by Terminal; only when MsgType is Failure or OperativeData]
	- Shutdown: string
	- OperativeData (encrypted): Blocks of Operative Messages

Operative Message Format [by Terminal]:

- MsgType [varint; one of Init, Data, Error, End, Padding]
	- Padding only consists of MsgType and optional data [bytes; not blocked!]
- OpID [varint]
- Data Block (only Init, Data, Error)
	- Init: OpType, Initial Data
	- Data: Data
	- Error: String

*/

type TerminalMsgType uint8

func (msgType TerminalMsgType) Pack() []byte {
	return varint.Pack8(uint8(msgType))
}

func ParseTerminalMsgType(c *container.Container) (TerminalMsgType, error) {
	msgType, err := c.GetNextN8()
	return TerminalMsgType(msgType), err
}

const (
	// MsgTypeNone is used to add available space only.
	MsgTypeNone TerminalMsgType = 0

	// MsgTypeEstablish is used to create a new terminal.
	MsgTypeEstablish TerminalMsgType = 1

	// MsgTypeOperativeData is used to send encrypted data for an operation.
	MsgTypeOperativeData TerminalMsgType = 2

	// MsgTypeAbandon is used to communciate that the other end of the Terminal
	// is being abandoned, with an optional error.
	MsgTypeAbandon TerminalMsgType = 3
)

type OpMsgType uint8

func (msgType OpMsgType) Pack() []byte {
	return varint.Pack8(uint8(msgType))
}

func ParseOpMsgType(c *container.Container) (OpMsgType, error) {
	msgType, err := c.GetNextN8()
	return OpMsgType(msgType), err
}

const (
	// MsgTypeInit is used to start a new operation.
	MsgTypeInit OpMsgType = 1

	// MsgTypeData is used to send data to an active operation.
	MsgTypeData OpMsgType = 2

	// MsgTypeEnd is used to end an active operation, with an optional error.
	MsgTypeEnd OpMsgType = 3

	// MsgTypePadding is used to add padding to increase the message size.
	MsgTypePadding OpMsgType = 4
)
