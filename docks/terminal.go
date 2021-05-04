package docks

import (
	"sync/atomic"
	"github.com/safing/portbase/container"
	"github.com/tevino/abool"
)


/*





*/


// Terminal Message Types.
const (
	// Informational
	TerminalMsgTypeInfo          uint8 = 1
	TerminalMsgTypeLoad          uint8 = 2
	TerminalMsgTypeStats         uint8 = 3
	TerminalMsgTypePublicHubFeed uint8 = 4

	// Diagnostics
	TerminalMsgTypeEcho      uint8 = 16
	TerminalMsgTypeSpeedtest uint8 = 17

	// User Access
	TerminalMsgTypeUserAuth uint8 = 32

	// Tunneling
	TerminalMsgTypeHop    uint8 = 40
	TerminalMsgTypeTunnel uint8 = 41
	TerminalMsgTypePing   uint8 = 42

	// Admin/Mod Access
	TerminalMsgTypeAdminAuth uint8 = 128

	// Mgmt
	TerminalMsgTypeEstablishRoute uint8 = 144
	TerminalMsgTypeShutdown       uint8 = 145
)

type Terminal struct {
	ID uint32

	// queue holds containers that need processing and is the terminal "space".
	queue chan *container.Container
	// space indicates the amount of free slots in the queue.
	space         *int32
	// reportedSpace indicates the reported amount of free slots in the queue.
	reportedSpace *int32

	// send holds containers that are waiting for shipping.
	send chan *container.Container
	// sendSpace indicates the amount of free slots in the send queue.
	sendSpace *int32
	// wakeSender wakes the sender in case the sending space was depleted.
	wakeSender chan struct{}

	// abandoned indicates if the Terminal has been abandoned. Whoever abandoned
	// the terminal already took care of notifying everyone, so a silent fail is
	// normally the best response.
	abandoned *abool.AtomicBool
}

type TerminalManager struct {
	t *Terminal

	isUser bool
	isAdmin bool
}

func NewTerminal(id uint32) (*Terminal, error) {
	t := &Terminal{
		ID: id,
		queue: make(chan *container.Container, 100),
		reportedSpace: new(int(0)),
		send: make(chan *container.Container, 100),
		sendSpace: new(int(0)),
		wakeSender: make(chan struct{}, 1)
		abandoned: abool.NewBool(false),
	}
	t.space = &cap(t.queue)

	return t, nil
}

func (t *Terminal) QueueContainer(c *container.Container) (reportSpace uint32) {
	select {
	case t.queue <- c:
			// Decrease space and reportedSpace
	space = atomic.AddInt32(t.space, -1)
	reportedSpace := atomic.AddInt32(t.reportedSpace, -1)

	// If the reported space is under 20% of the capacity, report available space.
	if reportedSpace == 0 || cap(queue)/reportedSpace >= 20 {
		reportSpace = space - reportedSpace
		atomic.AddInt32(t.reportedSpace, reportSpace)
		return reportSpace
	}

	default:
		// The queue should never be full, there is something amiss.
		t.Abandon(
			fmt.Errorf(
				"queue with cap=%d overflowed with space=%s and reportedSpace=%d",
				cap(t.queue),
				atomic.LoadInt32(t.space),
				atomic.LoadInt32(t.reportedSpace),
			),
			"queue overflow",
		)
	}

	return 0
}

func (t *Terminal) SpaceToReport() {}

func (t *Terminl) Abandon(internalError error, externalMsg string) {
	if t.abandoned.SetToIf(false, true) {
		// FIXME
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


func (t )

func NewConveyorLine(crane *Crane, lineID uint32) (*ConveyorLine, error) {


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
		toShore:            make(chan *container.Container),
		fromShore:          make(chan *container.Container),
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
