package ships

import (
	"context"
	"net"
	"time"

	"github.com/safing/spn/hub"
)

// TCPShip is a ship that uses TCP.
type TCPShip struct {
	ShipBase
}

// TCPPier is a pier that uses TCP.
type TCPPier struct {
	PierBase
}

func init() {
	Register("tcp", &Builder{
		LaunchShip:    launchTCPShip,
		EstablishPier: establishTCPPier,
	})
}

func launchTCPShip(ctx context.Context, transport *hub.Transport, ip net.IP) (Ship, error) {
	dialer := &net.Dialer{
		Timeout: 3 * time.Second,
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), portToA(transport.Port)))
	if err != nil {
		return nil, err
	}

	ship := &TCPShip{}
	ship.initBase(ctx, transport, true, conn)
	return ship, nil
}

func establishTCPPier(ctx context.Context, transport *hub.Transport, dockingRequests chan *DockingRequest) (Pier, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: int(transport.Port),
	})
	if err != nil {
		return nil, err
	}

	pier := &TCPPier{}
	pier.initBase(
		ctx,
		transport,
		listener,
		pier.dockShip,
		dockingRequests,
	)
	return pier, nil
}

func (pier *TCPPier) dockShip() (Ship, error) {
	conn, err := pier.listener.Accept()
	if err != nil {
		return nil, err
	}

	ship := &TCPShip{}
	ship.initBase(pier.ctx, pier.transport, false, conn)
	return ship, nil
}
