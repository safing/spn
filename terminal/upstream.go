package terminal

import "time"

// Upstream defines the the interface for upstream (parent) components.
type Upstream interface {
	Send(msg *Msg, timeout time.Duration) *Error
}

// UpstreamFromSendFunc creates an upstream proxy from a send function.
func UpstreamFromSendFunc(send func(msg *Msg, timeout time.Duration) *Error) *UpstreamProxy {
	return &UpstreamProxy{
		send: send,
	}
}

// UpstreamProxy is a helper to be able to satisfy the Upstream interface.
type UpstreamProxy struct {
	send func(msg *Msg, timeout time.Duration) *Error
}

// Send is used to send a message through this upstream.
func (up *UpstreamProxy) Send(msg *Msg, timeout time.Duration) *Error {
	return up.send(msg, timeout)
}
