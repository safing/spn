package terminal

import (
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
)

/*

Terminal Message Format:

- Length [varint; by Crane]
- MsgType [varint; by Crane; one of Establish, OperativeData, Abandon]
- ID [varint; by Crane]
- Data [bytes; possibly omitted, by Terminal]
	- When MsgType is Establish:
		- Init Data [bytes]
	- When MsgType is OperativeData:
		- AddAvailableSpace [varint; by Flow Queue]
		- (Encrypted) Blocked Operative Messages [bytes]
	- When MsgType is Abandon:
		-  [string]

Operative Message Format [by Terminal]:

- MsgType [varint; one of Init, Data, End, Padding]
- OpID [varint; omitted when MsgType is Padding]
- Data Block [bytes block; omitted when MsgType is Padding]
	- Init: OpType [bytes block], Initial Data [remaining bytes]
	- Data: Data [remaining bytes]
	- Error: Error [varint]

Padding MsgType Format:
The Padding MsgType used by the terminal may only be used as the last operative
message in a block of operative messages contained in a OperativeData message.
It effectively means that any remaining data is padding.

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
	// MsgTypeEstablish is used to add available space only.
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
