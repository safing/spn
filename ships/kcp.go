package ships

import (
	"context"
	"net"

	"github.com/safing/spn/hub"
	kcp "github.com/xtaci/kcp-go/v5"
)

// KCPShip is a ship that uses KCP.
type KCPShip struct {
	ShipBase
}

// KCPPier is a pier that uses KCP.
type KCPPier struct {
	PierBase
}

func init() {
	Register("kcp", &Builder{
		LaunchShip:    launchKCPShip,
		EstablishPier: establishKCPPier,
	})
}

func launchKCPShip(ctx context.Context, transport *hub.Transport, ip net.IP) (Ship, error) {
	conn, err := kcp.Dial(net.JoinHostPort(ip.String(), portToA(transport.Port)))
	if err != nil {
		return nil, err
	}

	ship := &KCPShip{}
	ship.initBase(ctx, transport, true, conn)
	return ship, nil
}

func establishKCPPier(ctx context.Context, transport *hub.Transport, dockingRequests chan *DockingRequest) (Pier, error) {
	listener, err := kcp.Listen(net.JoinHostPort("", portToA(transport.Port)))
	if err != nil {
		return nil, err
	}

	pier := &KCPPier{}
	pier.initBase(
		ctx,
		transport,
		listener,
		pier.dockShip,
		dockingRequests,
	)
	return pier, nil
}

func (pier *KCPPier) dockShip() (Ship, error) {
	conn, err := pier.listener.Accept()
	if err != nil {
		return nil, err
	}

	ship := &KCPShip{}
	ship.initBase(pier.ctx, pier.transport, false, conn)
	return ship, nil
}
