package terminal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/safing/portbase/formats/varint"

	"github.com/safing/portbase/container"
)

const (
	DefaultQueueSize    = 100
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

	// Never report values below zero.
	// This can happen, as dfq.reportedSpace is decreased after a container is
	// submitted to dfq.recvQueue by dfq.Deliver(). This race condition can only
	// lower the space to report, not increase it. A simple check here solved
	// this problem and keeps performance high.
	if toReport <= 0 {
		return 0
	}

	// Add space to report to dfq.reportedSpace and return it.
	atomic.AddInt32(dfq.reportedSpace, toReport)
	return toReport
}

// FlowHandler handles all flow queue internals and must be started as a worker
// in the module where it is used.
func (dfq *DuplexFlowQueue) FlowHandler(_ context.Context) error {
	// The upstreamSender is started by the terminal module, but is tied to the
	// flow owner instead. Make sure that the flow owner's module depends on the
	// terminal module so that it is shut down earlier.

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
					varint.Pack64(uint64(dfq.reportableRecvSpace())),
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
			c.Prepend(varint.Pack64(uint64(dfq.reportableRecvSpace())))

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
				varint.Pack64(uint64(dfq.reportableRecvSpace())),
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
	if atomic.LoadInt32(dfq.sendSpace) > 0 {
		return ready
	}
	return dfq.readyToSend
}

// Send adds the given container to the send queue.
func (dfq *DuplexFlowQueue) Send(c *container.Container) *Error {
	select {
	case dfq.sendQueue <- c:
		return nil
	case <-dfq.ti.Ctx().Done():
		return ErrStopping
	}
}

// SendRaw sends the given raw data without any further processing.
func (dfq *DuplexFlowQueue) SendRaw(c *container.Container) *Error {
	dfq.submitUpstream(c)
	return nil
}

// Receive receives a container from the recv queue.
func (dfq *DuplexFlowQueue) Receive() <-chan *container.Container {
	return dfq.recvQueue
}

// Deliver submits a container for receiving from upstream.
func (dfq *DuplexFlowQueue) Deliver(c *container.Container) *Error {
	// Ignore nil containers.
	if c == nil {
		return ErrStopping
	}

	// Get and add new reported space.
	addSpace, err := c.GetNextN16()
	if err != nil {
		return ErrMalformedData.With("failed to parse reported space: %w", err)
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

		return nil
	default:
		// If the recv queue is full, return an error.
		// The whole point of the flow queue is to guarantee that this never happens.
		return ErrQueueOverflow
	}
}

// FlowStats returns a k=v formatted string of internal stats.
func (dfq *DuplexFlowQueue) FlowStats() string {
	return fmt.Sprintf(
		"sq=%d rq=%d sends=%d reps=%d",
		len(dfq.sendQueue),
		len(dfq.recvQueue),
		atomic.LoadInt32(dfq.sendSpace),
		atomic.LoadInt32(dfq.reportedSpace),
	)
}
