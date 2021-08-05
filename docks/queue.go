package docks

import (
	"sync"

	"github.com/safing/portbase/container"
)

type BlockingSubmitQueue struct {
	lock sync.Mutex
	next *BlockingSubmitter
	last *BlockingSubmitter
	pool *sync.Pool
}

type BlockingSubmitter struct {
	submit chan *container.Container
	next   *BlockingSubmitter
}

func NewBlockingSubmitQueue(size int) *BlockingSubmitQueue {
	bsq := &BlockingSubmitQueue{
		pool: &sync.Pool{
			New: func() interface{} {
				return &BlockingSubmitter{
					submit: make(chan *container.Container),
				}
			},
		},
	}
	// Initialize empty-but-ready list.
	bsq.next = bsq.pool.New().(*BlockingSubmitter)

	return bsq
}

func (bsq *BlockingSubmitQueue) submit(c *container.Container) {
	var submitter *BlockingSubmitter

	// Get submitter.
	bsq.lock.Lock()
	if bsq.last == nil {
		// The list is empty, and bsq.next is prepared for first time use.
		submitter = bsq.next // Get first list entry.
		bsq.last = submitter // Add it as the last entry of the list.
	} else {
		// The list has at least one entry.
		submitter = bsq.pool.New().(*BlockingSubmitter)
		bsq.last.next = submitter // Add it as the next item of the current last list item.
		bsq.last = submitter      // Add new submitter as last list item.
	}
	bsq.lock.Unlock()

	// Submit and wait.
	submitter.submit <- c

	// Put back into pool.
	bsq.pool.Put(submitter)
}

func (bsq *BlockingSubmitQueue) read() <-chan *container.Container {
	// Get next submitter.
	bsq.lock.Lock()
	submitter := bsq.next     // Get first list entry.
	bsq.next = submitter.next // Shift list to next entry.
	submitter.next = nil      // Reset for next use.

	// If we reached the end of the list, reset it to empty-but-ready state.
	if bsq.next == nil {
		bsq.last = nil
		bsq.next = bsq.pool.New().(*BlockingSubmitter)
	}
	bsq.lock.Unlock()

	return submitter.submit
}

func (bsq *BlockingSubmitQueue) destroy() {
	// TODO
}
