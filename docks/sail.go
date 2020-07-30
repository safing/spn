package docks

import (
	"context"
	"fmt"
	"net"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
)

func LaunchShip(ctx context.Context, h *hub.Hub, transport *hub.Transport, ip net.IP) (ships.Ship, error) {
	var transports []*hub.Transport
	var ips []net.IP

	// choose transports
	if transport != nil {
		transports = []*hub.Transport{transport}
	} else {
		if h.Info == nil {
			return nil, hub.ErrMissingInfo
		}
		for _, definition := range h.Info.Transports {
			t, err := hub.ParseTransport(definition)
			if err != nil {
				log.Warningf("failed to parse transport definition %s of %s: %s", definition, h, err)
			} else {
				transports = append(transports, t)
			}
		}
		if len(h.Info.Transports) == 0 {
			return nil, hub.ErrMissingTransports
		}
	}

	// choose IPs
	if ip != nil {
		ips = []net.IP{ip}
	} else {
		if h.Info == nil {
			return nil, hub.ErrMissingInfo
		}
		// prioritize IPv6
		if h.Info.IPv6 != nil {
			ips = append(ips, h.Info.IPv6)
		}
		if h.Info.IPv4 != nil {
			ips = append(ips, h.Info.IPv4)
		}
		if len(ips) == 0 {
			return nil, hub.ErrMissingIPs
		}
	}

	// connect
	var firstErr error
	for _, ip := range ips {
		for _, tr := range transports {
			ship, err := connectTo(ctx, h, tr, ip)
			if err == nil {
				return ship, nil // return on success
			} else if firstErr == nil {
				firstErr = err // save first error
			}
		}
	}

	return nil, firstErr
}

func connectTo(ctx context.Context, h *hub.Hub, transport *hub.Transport, ip net.IP) (ships.Ship, error) {
	builder := ships.GetBuilder(transport.Protocol)
	if builder == nil {
		return nil, fmt.Errorf("protocol %s not supported", transport.Protocol)
	}

	ship, err := builder.LaunchShip(ctx, transport, ip)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s using %s (%s): %w", h, transport, ip, err)
	}

	return ship, nil
}

func EstablishRoute(id *cabin.Identity, dst *hub.Hub) {

	if GetAssignedCrane(dst.ID) != nil {
		log.Infof("spn/docks: tried to establish route to %s, but one already exists", dst.ID)
		return
	}

	log.Infof("spn/docks: establishing new route to %s", dst.ID)

	ship, err := LaunchShip(context.TODO(), dst, nil, nil)
	if err != nil {
		log.Warningf("spn/docks: unable to establish route to %s: failed to launch ship: %s", dst.ID, err)
		return
	}

	crane, err := NewCrane(ship, id, dst)
	if err != nil {
		log.Warningf("spn/docks: unable to establish route to %s: failed to build crane: %s", dst.ID, err)
		return
	}

	err = crane.Start()
	if err != nil {
		log.Warningf("spn/docks: unable to establish route to %s: failed to start crane: %s", dst.ID, err)
		return
	}
	if crane.stopped.IsSet() {
		return
	}

	err = crane.Controller.PublishConnection()
	if err != nil {
		log.Warningf("spn/docks: failed to initiate connection publishing: %s", err)
		return
	}
}
