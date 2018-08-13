package port17

import (
	"github.com/Safing/safing-core/container"
)

type SimpleConveyorLine struct {
	ID string

	toShore       chan *container.Container
	fromShore     chan *container.Container
	nextToShore   chan *container.Container
	nextFromShore chan *container.Container

	toShip   chan *container.Container
	fromShip chan *container.Container
}

func NewSimpleConveyorLine() *SimpleConveyorLine {
	new := &SimpleConveyorLine{
		ID:        "local line",
		toShore:   make(chan *container.Container, 10),
		fromShore: make(chan *container.Container, 10),
		toShip:    make(chan *container.Container, 10),
		fromShip:  make(chan *container.Container, 10),
	}

	new.nextToShore = new.toShore
	new.nextFromShore = new.fromShore

	return new
}

func (line *SimpleConveyorLine) AddConveyor(conveyor Conveyor) {
	if line.nextToShore == nil {
		return
	}
	newToShore := make(chan *container.Container, 0)
	newFromShore := make(chan *container.Container, 0)
	conveyor.AttachConveyorBelts(line.ID, line.nextToShore, line.nextFromShore, newFromShore, newToShore)
	line.nextToShore = newToShore
	line.nextFromShore = newFromShore
	go conveyor.Run()
}

func (line *SimpleConveyorLine) AddLastConveyor(conveyor LastConveyor) {
	if line.nextToShore == nil {
		return
	}
	conveyor.AttachConveyorBelts(line.ID, line.nextToShore, line.nextFromShore)
	line.nextToShore = nil
	line.nextFromShore = nil
	go conveyor.Run()
}
