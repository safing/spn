package terminal

import (
	"github.com/safing/portbase/container"
	"github.com/safing/spn/unit"
)

var scheduler *unit.Scheduler

type Msg struct {
	FlowID uint32
	Type   MsgType
	Data   *container.Container

	// Unit scheduling.
	// Note: With just 100B per packet, a uint64 (the Unit ID) is enough for
	// over 1800 Exabyte. No need for overflow support.
	*unit.Unit
}

// NewMsg returns a new msg.
func NewMsg(data []byte) *Msg {
	return &Msg{
		Type: MsgTypeData,
		Data: container.New(data),
		Unit: scheduler.NewUnit(),
	}
}

// NewEmptyMsg returns a new empty msg with an initialized Unit.
func NewEmptyMsg() *Msg {
	return &Msg{
		Unit: scheduler.NewUnit(),
	}
}

// Pack prepends the message header with ID+Type to the data.
func (msg *Msg) Pack() {
	MakeMsg(msg.Data, msg.FlowID, msg.Type)
}

// Consume adds another Message to itself.
// The given Msg is packed before adding it to the data.
// The data is moved - not copied!
// High priority mark is inherited.
func (msg *Msg) Consume(other *Msg) {
	// Pack message to be added.
	other.Pack()

	// Move data.
	msg.Data.AppendContainer(other.Data)

	// Inherit high priority.
	if other.IsHighPriorityUnit() {
		msg.MakeUnitHighPriority()
	}

	// Finish other unit.
	other.FinishUnit()
}
