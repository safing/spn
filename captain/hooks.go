package captain

import (
	"github.com/safing/portbase/log"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/navigator"
)

func initDockHooks() {
	docks.RegisterCraneUpdateHook(handleCraneUpdate)
}

func handleCraneUpdate(crane *docks.Crane) {
	// Do nothing if the connection is not public.
	if !crane.Public() {
		return
	}

	// Do nothing if we're not a public hub.
	if !conf.PublicHub() {
		return
	}

	// Update Hub status.
	updateConnectionStatus()
}

func updateConnectionStatus() {
	// export new connection status from controllers
	cranes := docks.GetAllAssignedCranes()
	lanes := make([]*hub.Lane, 0, len(cranes))
	for _, crane := range cranes {
		if crane.Public() {
			lanes = append(lanes, &hub.Lane{
				ID:       crane.ConnectedHub.ID,
				Capacity: 0, // TODO
				Latency:  0, // TODO
			})
		}
	}
	// Sort Lanes for comparing.
	hub.SortLanes(lanes)

	defer func() {
		log.Infof("spn/captain: current lanes: %v", publicIdentity.Hub.Status.Lanes)
	}()

	// update status
	changed, err := publicIdentity.MaintainStatus(lanes, false)
	if err != nil {
		log.Warningf("spn/captain: failed to update public hub status: %s", err)
		return
	}

	// Propagate changes.
	if changed {
		// Update hub in map.
		navigator.Main.UpdateHub(publicIdentity.Hub)
		log.Debug("spn/captain: updated own hub on map after status change")

		// Export status data.
		statusData, err := publicIdentity.ExportStatus()
		if err != nil {
			log.Warningf("spn/captain: failed to export public hub status: %s", err)
			return
		}

		// Forward status data to other connected Hubs.
		gossipRelayMsg("", GossipHubStatusMsg, statusData)
	}

	// TODO: Detect if we lost all connections and trigger a restart, if one is pending.
}
