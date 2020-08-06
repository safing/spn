package docks

import "github.com/safing/portbase/container"

const (
	CONV_MONKEY uint8 = 0
	CONV_MUX    uint8 = 1
)

// Conveyor transports and processes containers between the ship (with its crane) to the shore.
type Conveyor interface {
	AttachConveyorBelts(lineID string, fromShip, toShip, fromShore, toShore chan *container.Container)
	Run()
}

// ConveyorBase provides basic functionality for the Conveyor interface.
type ConveyorBase struct {
	lineID string

	fromShip  chan *container.Container
	toShip    chan *container.Container
	fromShore chan *container.Container
	toShore   chan *container.Container
}

// AttachConveyorBelts attaches the Conveyor to a line.
func (cb *ConveyorBase) AttachConveyorBelts(lineID string, fromShip, toShip, fromShore, toShore chan *container.Container) {
	cb.lineID = lineID
	cb.fromShip = fromShip
	cb.toShip = toShip
	cb.fromShore = fromShore
	cb.toShore = toShore
}

// LastConveyor is the last processing step to handle containers.
type LastConveyor interface {
	AttachConveyorBelts(lineID string, fromShip, toShip chan *container.Container)
	Run()
}

// LastConveyorBase provides basic functionality for the Conveyor interface.
type LastConveyorBase struct {
	lineID string

	fromShip chan *container.Container
	toShip   chan *container.Container
}

func (cb *LastConveyorBase) Run() {}

// AttachConveyorBelts attaches the Conveyor to a line.
func (cb *LastConveyorBase) AttachConveyorBelts(lineID string, fromShip, toShip chan *container.Container) {
	cb.lineID = lineID
	cb.fromShip = fromShip
	cb.toShip = toShip
}
