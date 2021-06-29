package terminal

import (
	"context"
	"sync"

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
	sendSpace uint16
	// readyToSend is used to notify sending components that there is free space.
	readyToSend chan struct{}
	// wakeSender is used to wake a sender in case the sendSpace was zero and the
	// sender is waiting for available space.
	wakeSender chan struct{}

	// recvQueue holds the containers that are waiting to be processed.
	recvQueue chan *container.Container
	// recvQueueSpace indicates the amount of free slots in the recvQueue.
	recvQueueSpace uint16
	// reportedSpace indicates the amount of free slots that the other end knows
	// about.
	reportedSpace uint16

	// spaceLock locks  of space to report.
	spaceLock sync.Mutex
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
		sendSpace:        queueSize,
		readyToSend:      make(chan struct{}),
		wakeSender:       make(chan struct{}, 1),
		recvQueue:        make(chan *container.Container, queueSize),
		recvQueueSpace:   queueSize,
		reportedSpace:    queueSize,
		forceSpaceReport: make(chan struct{}),
	}

	// Start worker.
	module.StartWorker("dfq sender", dfq.sender)

	return dfq
}

// decrementRecvSpace decreases the recv queue space and reported recv space by
// one and returns if the receive space should be reported.
func (dfq *DuplexFlowQueue) decrementRecvSpace() (shouldReportRecvSpace bool) {
	dfq.spaceLock.Lock()
	defer dfq.spaceLock.Unlock()

	dfq.recvQueueSpace--
	dfq.reportedSpace--
	return dfq.reportedSpace < uint16(cap(dfq.recvQueue)/forceReportFraction)
}

// decrementSendSpace decreases the send space by 1 and returns it.
func (dfq *DuplexFlowQueue) decrementSendSpace() uint16 {
	dfq.spaceLock.Lock()
	defer dfq.spaceLock.Unlock()

	dfq.sendSpace--
	return dfq.sendSpace
}

func (dfq *DuplexFlowQueue) addToSendSpace(n uint16) {
	dfq.spaceLock.Lock()
	defer dfq.spaceLock.Unlock()

	// Add new space to send space.
	dfq.sendSpace += n

	// Wake the sender if the send space was zero.
	if dfq.sendSpace == n {
		select {
		case dfq.wakeSender <- struct{}{}:
		default:
		}
	}
}

// reportableRecvSpace returns how much free space can be reported to the other
// end. The returned number must be communicated to the other end and must not
// be ignored.
func (dfq *DuplexFlowQueue) reportableRecvSpace() uint16 {
	dfq.spaceLock.Lock()
	defer dfq.spaceLock.Unlock()

	// Calculate reportable receive space and add it to the reported space.
	toReport := dfq.recvQueueSpace - dfq.reportedSpace
	dfq.reportedSpace += toReport

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
					varint.Pack16(dfq.reportableRecvSpace()),
					MsgTypeNone.Pack(),
				))

				// Decrease the send space and set flag if depleted.
				if dfq.decrementSendSpace() <= 0 {
					sendSpaceDepleted = true
				}

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
			c.Prepend(varint.Pack16(dfq.reportableRecvSpace()))
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
				varint.Pack16(dfq.reportableRecvSpace()),
				MsgTypeNone.Pack(),
			))

			// Decrease the send space and set flag if depleted.
			if dfq.decrementSendSpace() <= 0 {
				sendSpaceDepleted = true
			}

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
	dfq.spaceLock.Lock()
	defer dfq.spaceLock.Unlock()

	if dfq.sendSpace > 0 {
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
		dfq.addToSendSpace(addSpace)
	}

	select {
	case dfq.recvQueue <- c:
		// If the recv queue accepted the Container, decrement the recv space.
		shouldReportRecvSpace := dfq.decrementRecvSpace()
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
