package captain

import (
	"context"
	"sync"

	"github.com/safing/spn/docks"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
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

	for _, t := range publicIdentity.Hub().Info.Transports {
		transport, err := hub.ParseTransport(t)
		if err != nil {
			log.Warningf("spn/captain: cannot build pier for invalid transport %q: %s", t, err)
			continue
		}

		// create listener
		pier, err := ships.EstablishPier(module.Ctx, transport, dockingRequests)
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
			if checkDockingPermission(r.Ship) {
				handleDockingRequest(r.Ship)
			}
		}
	}
}

func checkDockingPermission(ship ships.Ship) (ok bool) {
	// TODO: check docking policies (hub entry policy)
	return true
}

func handleDockingRequest(ship ships.Ship) {
	log.Infof("spn/captain: pemitting %s to dock", ship)

	crane, err := docks.NewCrane(ship, publicIdentity, nil)
	if err != nil {
		log.Warningf("spn/captain: failed to comission crane for %s: %s", ship, err)
		return
	}

	err = crane.Start()
	if err != nil {
		log.Warningf("spn/captain: failed to start crane for %s: %s", ship, err)
	}
}
