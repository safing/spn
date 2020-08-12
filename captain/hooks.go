package captain

import (
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/navigator"
)

func initDockHooks() {
	docks.RegisterCraneHooks(
		handleHubAnnouncement,
		handleHubStatus,
		handleConnectionPublish,
		handleDiscontinuedConnection,
	)
}

func handleHubAnnouncement(
	controller *docks.CraneController,
	connectedHub *hub.Hub,
	c *container.Container,
) error {
	// import
	err := hub.ImportAnnouncement(c.CompileData(), hub.ScopePublic)
	if err != nil {
		log.Warningf("received invalid announcement from %s: %s", connectedHub.ID, err)
	}

	// forward to other connected Hubs
	if conf.PublicHub() {
		for _, ctrl := range docks.GetAllControllers() {
			if ctrl.Crane.ID != controller.Crane.ID {
				ctrl.SendHubAnnouncement(c.CompileData())
			}
		}
	}

	return nil
}

func handleHubStatus(
	controller *docks.CraneController,
	connectedHub *hub.Hub,
	c *container.Container,
) error {
	// import
	err := hub.ImportStatus(c.CompileData(), hub.ScopePublic)
	if err != nil {
		log.Warningf("received invalid status from %s: %s", connectedHub.ID, err)
	}

	// forward to other connected Hubs
	if conf.PublicHub() {
		for _, ctrl := range docks.GetAllControllers() {
			if ctrl.Crane.ID != controller.Crane.ID {
				ctrl.SendHubStatus(c.CompileData())
			}
		}
	}

	return nil
}

func handleConnectionPublish(
	controller *docks.CraneController,
	connectedHub *hub.Hub,
	c *container.Container,
) error {
	// do nothing if we're not a public hub
	if !conf.PublicHub() {
		return nil
	}

	// assign crane
	docks.AssignCrane(connectedHub.ID, controller.Crane)

	// update status
	updateConnectionStatus()

	return nil
}

func handleDiscontinuedConnection(
	controller *docks.CraneController,
	connectedHub *hub.Hub,
	c *container.Container,
) error {
	// do nothing if identity is unknown - there is no higher level logic initiated by us
	if connectedHub == nil {
		return nil
	}

	// shutdown any active API
	port := navigator.GetPublicPort(connectedHub.ID)
	if port != nil && port.ActiveAPI != nil {
		port.ActiveAPI.Shutdown()
	}

	// do nothing if we're not a public hub
	if !conf.PublicHub() {
		return nil
	}

	// only update if the connection was published
	if controller.Crane.Status() != docks.CraneStatusPublished {
		return nil
	}

	// update status
	updateConnectionStatus()

	// TODO: prepone restart if we loose all connections (ie. all connected hubs are restarting and no client are connected)

	return nil
}

func updateConnectionStatus() {
	// export new connection status from controllers
	controllers := docks.GetAllControllers()
	connections := make([]*hub.HubConnection, 0, len(controllers))
	for _, controller := range controllers {
		if controller.Crane.ConnectedHub != nil {
			connections = append(connections, &hub.HubConnection{
				ID:       controller.Crane.ConnectedHub.ID,
				Capacity: 0, // TODO
				Latency:  0, // TODO
			})
		}
	}
	// sort connections for comparing
	hub.SortConnections(connections)

	defer func() {
		log.Infof("spn/captain: current connections: %v", publicIdentity.Hub().Status.Connections)
	}()

	// update status
	changed, err := publicIdentity.MaintainStatus(connections)
	if err != nil {
		log.Warningf("spn/captain: failed to update public hub status: %s", err)
		return
	}

	// export if changed
	if changed {
		msg, err := publicIdentity.ExportStatus()
		if err != nil {
			log.Warningf("spn/captain: failed to export public hub status: %s", err)
			return
		}

		// forward to other connected Hubs
		for _, controller := range docks.GetAllControllers() {
			controller.SendHubStatus(msg)
		}
	}
}
