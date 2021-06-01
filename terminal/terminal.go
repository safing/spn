package terminal

import (
	"sync/atomic"
	"github.com/safing/portbase/container"
	"github.com/tevino/abool"
)


/*

Init Message Format:

- Version
- Letter

Encrypted Message:

- Version
- 

Message Format (Encrypted):

- AddAvailableSpace
- MsgType
- AssignmentID
- AssignmentData
	- Init: CmdID, Data
	- Data: Data
	- Error: string

*/


const (
	// MsgTypeNone is used for metadata only messages.
	// For example to add to the available sending space.
	MsgTypeNone uint8 = iota

	// MsgTypeInit is used to create a new assignment.
	MsgTypeInit

	// MsgTypeData signifies a data packet and is used in both directions.
	MsgTypeData
	
	// MsgTypeError signifies that there was an error during execution of the assignment.
	// Only sent by the server.
	MsgTypeError

	// MsgTypeDone signifies that the assignment was completed successfully.
	// Normally only sent by the server.
	// If sent by the client, it cancels the operation silently.
	MsgTypeDone
)

const (
	// ErrMalformedData is returned when the request data was malformed and could not be parsed.
	ErrMalformedData = errors.New("malformed data")

	// ErrUnknownAssignment is returned when a requested assignment cannot be found.
	ErrUnknownAssignment = errors.New("unknown assignment")

	// ErrUnknownAssignment is returned when a requested command cannot be found.
	ErrUnknownCommand = errors.New("unknown command")

	// ErrPermissinDenied is returned when calling a command with insufficient permissions.
	ErrPermissinDenied = erros.New("permission denied")

	// ErrQueueFull is returned when a full queue is encountered.
	ErrQueueFull = errors.New("queue full")
)

const (
	defaultMsgQueueSize = 100
)

type Terminal struct {
	ID uint64

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

	// assignments holds references to all assignments that require persistence.
	assignments map[uint64]*Assignment
	// nextAssignmedID holds the next assignmentID
	nextAssignmedID uint64

	// Encryption


	// Grouping
	// Terminals can be grouped together in order to multiplex a connection.

	groupNeighborUp *Terminal
	groupNeighborDown *Terminal

	Permission TerminalPermission
}

type TerminalManager struct {
	t *Terminal

	isUser bool
	isAdmin bool
}

// N
func NewBaseTerminal()

func NewBranchTerminal(id uint32, ) (*Terminal, error) {
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
