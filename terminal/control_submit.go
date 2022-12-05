package terminal

import (
	"context"
	"runtime"
	"time"
)

const (
	// MaxSubmitControlSize is the maximum size for submit control channels.
	MaxSubmitControlSize = 1000
)

// SubmitControl defines the submit control interface.
type SubmitControl interface {
	Submit(msg *Msg, timeout time.Duration) *Error
	Recv() <-chan SubmitControlItem
}

// SubmitControlItem defines the submit control item interface.
type SubmitControlItem interface {
	Accept() *Msg
}

// SubmitControlType represents a submit control type.
type SubmitControlType uint8

// Submit Control Types.
const (
	SubmitControlDefault SubmitControlType = 0
	SubmitControlPlain   SubmitControlType = 1
	SubmitControlFair    SubmitControlType = 2

	defaultSubmitControl = SubmitControlPlain
)

// DefaultSize returns the default flow control size.
func (sct SubmitControlType) DefaultSize() uint32 {
	if sct == SubmitControlDefault {
		sct = defaultSubmitControl
	}

	switch sct {
	case SubmitControlPlain:
		return 100
	case SubmitControlFair:
		return 100
	case SubmitControlDefault:
		fallthrough
	default:
		return 0
	}
}

// PlainChannel is a submit control using a plain channel.
type PlainChannel struct {
	ctx   context.Context
	queue chan SubmitControlItem
}

// PlainChannelItem is an item for the PlainChannel.
type PlainChannelItem struct {
	msg *Msg
}

// NewPlainChannel returns a new PlainChannel.
func NewPlainChannel(ctx context.Context, size int) *PlainChannel {
	return &PlainChannel{
		ctx:   ctx,
		queue: make(chan SubmitControlItem, size),
	}
}

// Submit submits data to the channel.
func (pc *PlainChannel) Submit(msg *Msg, timeout time.Duration) *Error {
	// Prepare submit timeout.
	var submitTimeout <-chan time.Time
	if timeout > 0 {
		submitTimeout = time.After(timeout)
	}

	// Submit message to buffer, if space is available.
	msg.PauseUnit()
	select {
	case pc.queue <- &PlainChannelItem{msg}:
		return nil
	case <-submitTimeout:
		msg.FinishUnit()
		return ErrTimeout.With("plain channel submit timeout")
	case <-pc.ctx.Done():
		msg.FinishUnit()
		return ErrStopping
	}
}

// Recv returns a receive-channel to receive an item from the submit control channel.
func (pc *PlainChannel) Recv() <-chan SubmitControlItem {
	return pc.queue
}

// Accept is called by the channel owner when an item from the channel is
// accepted to receive the data.
func (pci *PlainChannelItem) Accept() *Msg {
	return pci.msg
}

// FairChannel is a submit control using a fairly queued channel.
type FairChannel struct {
	ctx   context.Context
	queue chan SubmitControlItem
}

// FairChannelItem is an item for the FairChannel.
type FairChannelItem struct {
	msg  *Msg
	read chan struct{}
}

// NewFairChannel returns a new FairChannel.
func NewFairChannel(ctx context.Context, size int) *FairChannel {
	return &FairChannel{
		ctx:   ctx,
		queue: make(chan SubmitControlItem, size),
	}
}

// Submit submits data to the channel.
func (fc *FairChannel) Submit(msg *Msg, timeout time.Duration) *Error {
	item := &FairChannelItem{
		msg:  msg,
		read: make(chan struct{}),
	}

	// Prepare submit timeout.
	var submitTimeout <-chan time.Time
	if timeout > 0 {
		submitTimeout = time.After(timeout)
	}

	// Submit message to buffer, if space is available.
	msg.PauseUnit()
	select {
	case fc.queue <- item:
		runtime.Gosched()
		// Continue
	case <-submitTimeout:
		msg.FinishUnit()
		return ErrTimeout.With("fair channel submit timeout")
	case <-fc.ctx.Done():
		msg.FinishUnit()
		return ErrStopping
	}

	// Wait for message to be read.
	select {
	case <-item.read:
		runtime.Gosched()
		return nil
	case <-submitTimeout:
		msg.FinishUnit()
		return ErrTimeout.With("fair channel submit confirmation timeout")
	case <-fc.ctx.Done():
		msg.FinishUnit()
		return ErrStopping
	}
}

// Recv returns a receive-channel to receive an item from the submit control channel.
func (fc *FairChannel) Recv() <-chan SubmitControlItem {
	return fc.queue
}

// Accept is called by the channel owner when an item from the channel is
// accepted to receive the data.
func (fci *FairChannelItem) Accept() *Msg {
	close(fci.read)
	return fci.msg
}
