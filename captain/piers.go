package captain

import (
	"context"
	"fmt"
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/portmaster/intel"
	"github.com/safing/portmaster/network/netutils"
	"github.com/safing/portmaster/profile/endpoints"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
)

var (
	managePiersTask *modules.Task
	pierMgmtLock    sync.Mutex
	pierMgmtCycleID int

	dockingRequests = make(chan *ships.DockingRequest, 10)
)

func startPierMgmt() error {
	managePiersTask = module.NewTask(
		"manage piers",
		managePiers,
	)

	module.StartServiceWorker("docking request handler", 0, dockingRequestHandler)

	err := managePiers(module.Ctx, managePiersTask)
	if err != nil {
		log.Warningf("spn/captain: failed to initialize piers: %s", err)
	}

	return nil
}

func managePiers(ctx context.Context, task *modules.Task) error {
	pierMgmtLock.Lock()
	defer pierMgmtLock.Unlock()

	// TODO: do proper management (this is a workaround for now)
	if pierMgmtCycleID > 0 {
		return nil
	}
	pierMgmtCycleID = 1

	for _, t := range publicIdentity.Hub.Info.Transports {
		transport, err := hub.ParseTransport(t)
		if err != nil {
			log.Warningf("spn/captain: cannot build pier for invalid transport %q: %s", t, err)
			continue
		}

		// create listener
		pier, err := ships.EstablishPier(transport, dockingRequests)
		if err != nil {
			log.Warningf("spn/captin: failed to establish pier for transport %q: %s", t, err)
			continue
		}
		log.Infof("spn/captain: pier for transport %q built", t)

		// start accepting connections
		module.StartWorker("pier docking", pier.Docking)
	}

	// TODO:
	// task.Schedule(5 * time.Minute)

	return nil
}

func dockingRequestHandler(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case r := <-dockingRequests:
			switch {
			case r.Err != nil:
				// TODO: Restart pier?
				// TODO: Do actual pier management.
				log.Errorf("spn/captain: pier %s failed: %s", r.Pier.Transport(), r.Err)
			case r.Ship != nil:
				if err := checkDockingPermission(ctx, r.Ship); err != nil {
					log.Warningf("spn/captain: denied ship from %s to dock at pier %s: %s", r.Ship.RemoteAddr(), r.Pier.Transport(), err)
				} else {
					handleDockingRequest(r.Ship)
				}
			default:
				log.Warningf("spn/captain: received invalid docking request without ship for pier %s", r.Pier.Transport())
			}
		}
	}
}

func checkDockingPermission(ctx context.Context, ship ships.Ship) error {
	remoteIP, err := netutils.IPFromAddr(ship.RemoteAddr())
	if err != nil {
		return fmt.Errorf("failed to parse remote IP: %w", err)
	}

	// Create entity.
	entity := &intel.Entity{}
	entity.SetIP(remoteIP)
	entity.FetchData(ctx)

	// TODO: Do we want to handle protocol and port too?

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
