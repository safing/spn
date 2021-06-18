package terminal

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/safing/portbase/formats/varint"

	"github.com/safing/portbase/container"
)

const (
	defaultQueueSize    = 100
	forceReportFraction = 4
)

type DuplexFlowQueue struct {
	// ti is the interface to the Terminal that is using the DFQ.
	ti TerminalInterface

	// upstream is the channel to put containers into to send them upstream.
	submitUpstream func(*container.Container)

	// sendQueue holds the containers that are waiting to be sent.
	sendQueue chan *container.Container
	// sendSpace indicates the amount free slots in the recvQueue on the other end.
	sendSpace *int32
	// readyToSend is used to notify sending components that there is free space.
	readyToSend chan struct{}
	// wakeSender is used to wake a sender in case the sendSpace was zero and the
	// sender is waiting for available space.
	wakeSender chan struct{}

	// recvQueue holds the containers that are waiting to be processed.
	recvQueue chan *container.Container
	// reportedSpace indicates the amount of free slots that the other end knows
	// about.
	reportedSpace *int32
	// spaceReportLock locks the calculation of space to report.
	spaceReportLock sync.Mutex
	// forceSpaceReport forces the sender to send a space report.
	forceSpaceReport chan struct{}
}

func NewDuplexFlowQueue(
	ti TerminalInterface,
	queueSize uint16,
	submitUpstream func(*container.Container),
) *DuplexFlowQueue {
	dfq := &DuplexFlowQueue{
		ti:               ti,
		submitUpstream:   submitUpstream,
		sendQueue:        make(chan *container.Container, queueSize),
		sendSpace:        new(int32),
		readyToSend:      make(chan struct{}),
		wakeSender:       make(chan struct{}, 1),
		recvQueue:        make(chan *container.Container, queueSize),
		reportedSpace:    new(int32),
		forceSpaceReport: make(chan struct{}),
	}
	atomic.StoreInt32(dfq.sendSpace, int32(queueSize))
	atomic.StoreInt32(dfq.reportedSpace, int32(queueSize))

	// Start worker.
	module.StartWorker("dfq sender", dfq.sender)

	return dfq
}

// decrementReportedRecvSpace decreases the reported recv space by 1 and
// returns if the receive space should be reported.
func (dfq *DuplexFlowQueue) decrementReportedRecvSpace() (shouldReportRecvSpace bool) {
	return atomic.AddInt32(dfq.reportedSpace, -1) < int32(cap(dfq.recvQueue)/forceReportFraction)
}

// decrementSendSpace decreases the send space by 1 and returns it.
func (dfq *DuplexFlowQueue) decrementSendSpace() int32 {
	return atomic.AddInt32(dfq.sendSpace, -1)
}

func (dfq *DuplexFlowQueue) addToSendSpace(n int32) {
	// Add new space to send space and check if it was zero.
	if atomic.AddInt32(dfq.sendSpace, n) == n {
		// Wake the sender if the send space was zero.
		select {
		case dfq.wakeSender <- struct{}{}:
		default:
		}
	}
}

// reportableRecvSpace returns how much free space can be reported to the other
// end. The returned number must be communicated to the other end and must not
// be ignored.
func (dfq *DuplexFlowQueue) reportableRecvSpace() int32 {
	// Changes to the recvQueue during calculation are no problem.
	// We don't want to report space twice though!
	dfq.spaceReportLock.Lock()
	defer dfq.spaceReportLock.Unlock()

	// Calculate reportable receive space and add it to the reported space.
	reportedSpace := atomic.LoadInt32(dfq.reportedSpace)
	toReport := int32(cap(dfq.recvQueue)-len(dfq.recvQueue)) - reportedSpace
	atomic.AddInt32(dfq.reportedSpace, toReport)

	return toReport
}

func (dfq *DuplexFlowQueue) sender(_ context.Context) error {
	// The upstreamSender is started by the terminal module, but is tied to the
	// flow owner instead. Make sure that the flow owner's module depends on the
	// terminal module so that it is shut down earlier.

	// Notify upstream when were shutting down.
	defer dfq.submitUpstream(nil)

	var sendSpaceDepleted bool

sending:
	for {
		// If the send queue is depleted, wait to be woken.
		if sendSpaceDepleted {
			select {
			case <-dfq.wakeSender:
				sendSpaceDepleted = false
			case <-dfq.forceSpaceReport:
				// Forced reporting of space.
				// We do not need to check if there is enough sending space, as there is
				// no data included.
				dfq.submitUpstream(container.New(
					varint.Pack32(dfq.ti.ID()),
					varint.Pack64(uint64(dfq.reportableRecvSpace())),
					MsgTypeNone.Pack(),
				))

				continue sending
			case <-dfq.ti.Ctx().Done():
				return nil
			}
		}

		// Get Container from send queue.

		select {
		case dfq.readyToSend <- struct{}{}:
			// Notify that we are ready to send.

		case c := <-dfq.sendQueue:
			// Send Container from queue.

			// If nil, the queue is being shut down.
			if c == nil {
				return nil
			}

			// Prepend available receiving space and flow ID.
			c.Prepend(varint.Pack64(uint64(dfq.reportableRecvSpace())))
			c.Prepend(varint.Pack32(dfq.ti.ID()))

			// Submit for sending upstream.
			dfq.submitUpstream(c)

			// Decrease the send space and set flag if depleted.
			if dfq.decrementSendSpace() <= 0 {
				sendSpaceDepleted = true
			}

		case <-dfq.forceSpaceReport:
			// Forced reporting of space.
			// We do not need to check if there is enough sending space, as there is
			// no data included.
			dfq.submitUpstream(container.New(
				varint.Pack32(dfq.ti.ID()),
				varint.Pack64(uint64(dfq.reportableRecvSpace())),
				MsgTypeNone.Pack(),
			))

		case <-dfq.ti.Ctx().Done():
			return nil
		}
	}
}

var ready = make(chan struct{})

func init() {
	close(ready)
}

// Send adds the given container to the send queue.
func (dfq *DuplexFlowQueue) ReadyToSend() <-chan struct{} {
	if atomic.LoadInt32(dfq.sendSpace) > 0 {
		return ready
	}
	return dfq.readyToSend
}

// Send adds the given container to the send queue.
func (dfq *DuplexFlowQueue) Send(c *container.Container) Error {
	select {
	case dfq.sendQueue <- c:
		return ErrNil
	case <-dfq.ti.Ctx().Done():
		return ErrAbandoning
	}
}

// Receive receives a container from the recv queue.
func (dfq *DuplexFlowQueue) Receive() <-chan *container.Container {
	return dfq.recvQueue
}

// Deliver submits a container for receiving from upstream.
func (dfq *DuplexFlowQueue) Deliver(c *container.Container) Error {
	// Interpret a nil container as a call to End().
	if c == nil {
		dfq.ti.End("", ErrNil)
		return ErrNil
	}

	// Get and add new reported space.
	addSpace, err := c.GetNextN16()
	if err != nil {
		return ErrMalformedData
	}
	if addSpace > 0 {
		dfq.addToSendSpace(int32(addSpace))
	}

	select {
	case dfq.recvQueue <- c:
		// If the recv queue accepted the Container, decrement the recv space.
		shouldReportRecvSpace := dfq.decrementReportedRecvSpace()
		// If the reported recv space is nearing its end, force a report, if the
		// sender worker is idle.
		if shouldReportRecvSpace {
			select {
			case dfq.forceSpaceReport <- struct{}{}:
			default:
			}
		}

		return ErrNil
	default:
		// If the recv queue is full, return an error.
		// The whole point of the flow queue is to guarantee that this never happens.
		return ErrQueueOverflow
	}
}