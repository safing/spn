package captain

import (
	"time"

	"github.com/safing/portmaster/updates"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/docks"
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
	// Delay updating status for a better chance to combine multiple changes.
	statusUpdateTask.Schedule(time.Now().Add(maintainStatusUpdateDelay))

	// Check if we lost all connections and trigger a pending restart if we did.
	for _, crane := range docks.GetAllAssignedCranes() {
		if crane.Public() && !crane.Stopped() {
			// There is at least one public and active crane, so don't restart now.
			return
		}
	}
	updates.TriggerRestartIfPending()
}
