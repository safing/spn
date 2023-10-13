package captain

import (
	"context"
	"errors"
	"fmt"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/intel"
	"github.com/safing/portmaster/network/netutils"
	"github.com/safing/portmaster/profile/endpoints"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
)

var (
	dockingRequests = make(chan ships.Ship, 100)
	piers           []ships.Pier
)

func startPiers() error {
	// Get and check transports.
	transports := publicIdentity.Hub.Info.Transports
	if len(transports) == 0 {
		return errors.New("no transports defined")
	}

	piers = make([]ships.Pier, 0, len(transports))
	for _, t := range transports {
		// Parse transport.
		transport, err := hub.ParseTransport(t)
		if err != nil {
			return fmt.Errorf("cannot build pier for invalid transport %q: %w", t, err)
		}

		// Establish pier / listener.
		pier, err := ships.EstablishPier(transport, dockingRequests)
		if err != nil {
			return fmt.Errorf("failed to establish pier for transport %q: %w", t, err)
		}

		piers = append(piers, pier)
		log.Infof("spn/captain: pier for transport %q built", t)
	}

	// Start worker to handle docking requests.
	module.StartServiceWorker("docking request handler", 0, dockingRequestHandler)

	return nil
}

func stopPiers() {
	for _, pier := range piers {
		pier.Abolish()
	}
}

func dockingRequestHandler(ctx context.Context) error {
	// Sink all waiting ships when this worker ends.
	// But don't be destructive so the service worker could recover.
	defer func() {
		for {
			select {
			case ship := <-dockingRequests:
				if ship != nil {
					ship.Sink()
				}
			default:
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ship := <-dockingRequests:
			// Ignore nil ships.
			if ship == nil {
				continue
			}

			if err := checkDockingPermission(ctx, ship); err != nil {
				log.Warningf("spn/captain: denied ship from %s to dock at pier %s: %s", ship.RemoteAddr(), ship.Transport().String(), err)
			} else {
				handleDockingRequest(ship)
			}
		}
	}
}

func checkDockingPermission(ctx context.Context, ship ships.Ship) error {
	remoteIP, remotePort, err := netutils.IPPortFromAddr(ship.RemoteAddr())
	if err != nil {
		return fmt.Errorf("failed to parse remote IP: %w", err)
	}

	// Create entity.
	entity := (&intel.Entity{
		IP:       remoteIP,
		Protocol: uint8(netutils.ProtocolFromNetwork(ship.RemoteAddr().Network())),
		Port:     remotePort,
	}).Init(ship.Transport().Port)
	entity.FetchData(ctx)

	// Check against policy.
	result, reason := publicIdentity.Hub.GetInfo().EntryPolicy().Match(ctx, entity)
	if result == endpoints.Denied {
		return fmt.Errorf("entry policy violated: %s", reason)
	}

	return nil
}

func handleDockingRequest(ship ships.Ship) {
	log.Infof("spn/captain: pemitting %s to dock", ship)

	crane, err := docks.NewCrane(ship, nil, publicIdentity)
	if err != nil {
		log.Warningf("spn/captain: failed to commission crane for %s: %s", ship, err)
		return
	}

	module.StartWorker("start crane", func(ctx context.Context) error {
		_ = crane.Start(ctx)
		// Crane handles errors internally.
		return nil
	})
}
