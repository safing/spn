package core

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/safing/portbase/container"
	"github.com/tevino/abool"
)

const (
	BackOffStart = 32 * time.Microsecond
	BackOffLimit = 100 * time.Millisecond
)

type ConveyorLine struct {
	crane *Crane
	ID    uint32

	toShore       chan *container.Container
	fromShore     chan *container.Container
	nextToShore   chan *container.Container
	nextFromShore chan *container.Container

	fromShip chan *container.Container

	// represets space of this node
	shoreCap           int32
	shoreSpace         *int32
	reportedShoreSpace *int32

	// represents space of the connected node
	shipSpace *int32

	abandoned *abool.AtomicBool
}

func NewConveyorLine(crane *Crane, lineID uint32) (*ConveyorLine, error) {
	var shoreSpace int32 = 100
	var reportedShoreSpace int32 = 100
	var shipSpace int32 = 100

	new := &ConveyorLine{
		crane:              crane,
		ID:                 lineID,
		toShore:            make(chan *container.Container, 0),
		fromShore:          make(chan *container.Container, 0),
		fromShip:           make(chan *container.Container, 101),
		shoreCap:           100,
		shoreSpace:         &shoreSpace,
		reportedShoreSpace: &reportedShoreSpace,
		shipSpace:          &shipSpace,
		abandoned:          abool.NewBool(false),
	}

	new.nextToShore = new.toShore
	new.nextFromShore = new.fromShore

	go new.handler()
	go new.dispatcher()

	return new, nil
}

func (line *ConveyorLine) getLineID() string {
	return fmt.Sprintf("line %d at crane %s", line.ID, line.crane.ID)
}

func (line *ConveyorLine) AddConveyor(conveyor Conveyor) {
	if line.nextToShore == nil {
		return
	}
	newToShore := make(chan *container.Container, 0)
	newFromShore := make(chan *container.Container, 0)
	conveyor.AttachConveyorBelts(line.getLineID(), line.nextToShore, line.nextFromShore, newFromShore, newToShore)
	line.nextToShore = newToShore
	line.nextFromShore = newFromShore
	go conveyor.Run()
}

func (line *ConveyorLine) AddLastConveyor(conveyor LastConveyor) {
	if line.nextToShore == nil {
		return
	}
	conveyor.AttachConveyorBelts(line.getLineID(), line.nextToShore, line.nextFromShore)
	line.nextToShore = nil
	line.nextFromShore = nil
	go conveyor.Run()
}

func (line *ConveyorLine) handler() {
	var c *container.Container
	for {
		c = <-line.fromShip
		if c == nil {
			line.toShore <- nil
			line.fromShore <- nil
			return
		}
		line.toShore <- c
		line.increaseShoreSpace()
	}
}

func (line *ConveyorLine) dispatcher() {
	// add container ID, then send to ship

	var c *container.Container
	backoff := BackOffStart

	for {

		c = <-line.fromShore
		if c == nil || c.HasError() {
			line.crane.Controller.discardLine(line.ID)
			return
		}

		for {
			if line.availableShipSpace() > 0 {
				line.crane.dispatchContainer(line.ID, c)
				line.decreaseAvailableShipSpace()
				backoff = BackOffStart
				break
			} else {
				time.Sleep(backoff)
				backoff *= 2
				if backoff > BackOffLimit {
					// log.Debugf("crane %s: line %d: waiting...", line.crane.ID, line.ID)
					backoff = BackOffLimit
				}
			}
		}

	}

}

func (line *ConveyorLine) notifyOfNewContainer() (report bool, space int32) {
	// decrease shoreSpace and reportedShoreSpace
	space = atomic.AddInt32(line.shoreSpace, -1)
	reported := atomic.AddInt32(line.reportedShoreSpace, -1)
	// log.Debugf("crane: %s: line %d: space=%d, reported=%d", line.crane.ID, line.ID, space, reported)
	// if reported shore space under 10% of available shorespace, report
	if reported == 0 || line.shoreCap/reported >= 10 {
		if space <= 0 {
			return false, 0
		}
		atomic.StoreInt32(line.reportedShoreSpace, space)
		return true, space - reported
	}
	return false, 0
}

func (line *ConveyorLine) getShoreSpaceForReport() (report bool, space int32) {
	// get shoreSpace and reportedShoreSpace
	space = atomic.LoadInt32(line.shoreSpace)
	reported := atomic.LoadInt32(line.reportedShoreSpace)
	// if reported shore space under 50% of available shorespace, report
	if reported == 0 || line.shoreCap/reported >= 2 {
		if space <= 0 {
			return false, 0
		}
		atomic.StoreInt32(line.reportedShoreSpace, space)
		return true, space - reported
	}
	return false, 0
}

func (line *ConveyorLine) increaseShoreSpace() {
	atomic.AddInt32(line.shoreSpace, 1)
}

func (line *ConveyorLine) availableShipSpace() int32 {
	return atomic.LoadInt32(line.shipSpace)
}

func (line *ConveyorLine) addAvailableShipSpace(space int32) {
	atomic.AddInt32(line.shipSpace, space)
}

func (line *ConveyorLine) decreaseAvailableShipSpace() {
	atomic.AddInt32(line.shipSpace, -1)
}
