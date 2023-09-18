package ships

import (
	"context"
	"fmt"
	"net"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/spn/hub"
)

// Launch launches a new ship to the given Hub.
func Launch(ctx context.Context, h *hub.Hub, transport *hub.Transport, ip net.IP) (Ship, error) {
	var transports []*hub.Transport
	var ips []net.IP

	// choose transports
	if transport != nil {
		transports = []*hub.Transport{transport}
	} else {
		if h.Info == nil {
			return nil, hub.ErrMissingInfo
		}
		transports = h.Info.ParsedTransports()
		if len(transports) == 0 {
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
		ips = make([]net.IP, 0, 3)
		// If IPs have been verified, check if we can use a virtual network address.
		var vnetForced bool
		if h.VerifiedIPs {
			vnet := GetVirtualNetworkConfig()
			if vnet != nil {
				virtIP := vnet.Mapping[h.ID]
				if virtIP != nil {
					ips = append(ips, virtIP)
					if vnet.Force {
						vnetForced = true
						log.Infof("spn/ships: forcing virtual network address %s for %s", virtIP, h)
					} else {
						log.Infof("spn/ships: using virtual network address %s for %s", virtIP, h)
					}
				}
			}
		}
		// Add Hub's IPs if no virtual address was forced.
		if !vnetForced {
			// prioritize IPv4
			if h.Info.IPv4 != nil {
				ips = append(ips, h.Info.IPv4)
			}
			if h.Info.IPv6 != nil && netenv.IPv6Enabled() {
				ips = append(ips, h.Info.IPv6)
			}
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

func connectTo(ctx context.Context, h *hub.Hub, transport *hub.Transport, ip net.IP) (Ship, error) {
	builder := GetBuilder(transport.Protocol)
	if builder == nil {
		return nil, fmt.Errorf("protocol %s not supported", transport.Protocol)
	}

	ship, err := builder.LaunchShip(ctx, transport, ip)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s using %s (%s): %w", h, transport, ip, err)
	}

	return ship, nil
}
