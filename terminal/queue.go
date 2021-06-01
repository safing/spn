package terminal

import (
	"sync"
	"sync/atomic"

	"github.com/safing/portbase/container"
)

type DuplexFlowQueue struct {
	lock sync.Mutex

	// sendQueue holds the containers that are waiting to be sent.
	sendQueue chan *container.Container
	// sendSpace indicates the amount free slots in the recvQueue on the other end.
	sendSpace *int32
	// wakeSender is used to wake a sender in case the sendSpace was zero and the
	// sender is waiting for available space.
	wakeSender chan struct{}

	// recvQueue holds the containers that are waiting to be processed.
	recvQueue chan *container.Container
	// recvSpace indicates the amount of free slots in the recvQueue.
	recvSpace *int32
	// reportedSpace indicates the amount of free slots that the other end knows about.
	reportedSpace *int32
}

func (dfq *DuplexFlowQueue) GetSpaceToReport() uint32 {
	spaceLock.Lock()
	defer spaceLock.Unlock()

	if dfq.space > dfq.reportedSpace {

		spaceToReport := space - reportedSpace
		atomic.AddInt32(dfq.reportedSpace, spaceToReport)
	}
	return 0
}

// Send adds the given container to the send queue.
func (dfq *DuplexFlowQueue) Send(c *container.Container) {

}

func (dfq *DuplexFlowQueue) RetrieveForSending() *container.Container {

}

func (dfq *DuplexFlowQueue) SubmitForReceiving(c *container.Container) error {
	// Get new space indicator.
	addSpace, err := c.GetNextN16()
	if err != nil {
		return err
	}

	// Add reported space.
	atomic.AddInt32(dfq.sendSpace, uint32(addSpace))

	select {
	case dfq.recvQueue <- c:
		atomic.AddInt32(dfq.recvSpace, -1)
		return nil
	default:
		return ErrQueueFull
	}
}

func (dfq *DuplexFlowQueue) Receive() *container.Container {

}
