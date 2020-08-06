package ships

import (
	"context"
	"fmt"
	"net"

	"github.com/safing/spn/hub"
	"github.com/tevino/abool"
)

// Pier represents a network connection listener.
type Pier interface {
	// String returns a human readable informational summary about the ship.
	String() string

	// Transport returns the transport used for this ship.
	Transport() *hub.Transport

	// Docking is the blocking (!) procedure that docks new ships and sends docking requests. This should be run as a worker by the caller.
	Docking(ctx context.Context) error

	// Addr returns the underlying network address used by the listener.
	Addr() net.Addr

	// Abolish closes the underlying listener and cleans up any related resources.
	Abolish()
}

// DockingRequest is a uniform request that Piers emit when a new ship arrives.
type DockingRequest struct {
	Pier Pier
	Ship Ship
	Err  error
}

// EstablishPier is shorthand function to get the transport's builder and establish a pier.
func EstablishPier(ctx context.Context, transport *hub.Transport, dockingRequests chan *DockingRequest) (Pier, error) {
	builder := GetBuilder(transport.Protocol)
	if builder == nil {
		return nil, fmt.Errorf("protocol %s not supported", transport.Protocol)
	}

	pier, err := builder.EstablishPier(ctx, transport, dockingRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to establish pier on %s: %w", transport, err)
	}

	return pier, nil
}

// PierBase implements common functions to comply with the Pier interface.
type PierBase struct {
	ctx             context.Context
	transport       *hub.Transport
	listener        net.Listener
	abolishing      *abool.AtomicBool
	dockShip        func() (Ship, error)
	dockingRequests chan *DockingRequest
}

func (pier *PierBase) initBase(
	ctx context.Context,
	transport *hub.Transport,
	listener net.Listener,
	dockShip func() (Ship, error),
	dockingRequests chan *DockingRequest,
) {
	// populate
	pier.ctx = ctx
	pier.transport = transport
	pier.listener = listener
	pier.dockShip = dockShip
	pier.dockingRequests = dockingRequests

	// init
	pier.abolishing = abool.New()
}

// String returns a human readable informational summary about the ship.
func (pier *PierBase) String() string {
	return fmt.Sprintf("<Pier %s>", pier.transport)
}

// Transport returns the transport used for this ship.
func (pier *PierBase) Transport() *hub.Transport {
	return pier.transport
}

// Docking is the blocking (!) procedure that docks new ships and sends docking requests. This should be run as a worker by the caller.
func (pier *PierBase) Docking(ctx context.Context) error {
	for {
		ship, err := pier.dockShip()
		if err != nil {
			if pier.abolishing.SetToIf(false, true) {
				_ = pier.listener.Close()
				pier.dockingRequests <- &DockingRequest{
					Pier: pier,
					Err:  err,
				}
			}
			return nil
		}

		pier.dockingRequests <- &DockingRequest{
			Pier: pier,
			Ship: ship,
		}
	}
}

// Addr returns the underlying network address used by the listener.
func (pier *PierBase) Addr() net.Addr {
	return pier.listener.Addr()
}

// Abolish closes the underlying listener and cleans up any related resources.
func (pier *PierBase) Abolish() {
	if pier.abolishing.SetToIf(false, true) {
		_ = pier.listener.Close()
	}
}
