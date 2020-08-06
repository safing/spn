package captain

import (
	"context"
	"fmt"
	"time"

	"github.com/safing/spn/navigator"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
)

var (
	publicIdentity    *cabin.Identity
	publicIdentityKey = "core:spn/public/identity"

	publicIdentityUpdateTask *modules.Task
)

func loadPublicIdentity() (err error) {
	publicIdentity, err = cabin.LoadIdentity(publicIdentityKey)
	switch err {
	case nil:
		// load was successful
		log.Infof("spn/captain: loaded public hub identity %s", publicIdentity.Hub().ID)
	case database.ErrNotFound:
		// does not exist, create new
		publicIdentity, err = cabin.CreateIdentity(module.Ctx, hub.ScopePublic)
		if err != nil {
			return fmt.Errorf("failed to create new identity: %w", err)
		}

		// save to database
		publicIdentity.SetKey(publicIdentityKey)
		err = publicIdentity.Save()
		if err != nil {
			return fmt.Errorf("failed to save new identity to database: %w", err)
		}
		err = publicIdentity.Hub().Save()
		if err != nil {
			return fmt.Errorf("failed to save identity hub to database: %w", err)
		}

		log.Infof("spn/captain: created new public hub identity %s", publicIdentity.ID)
	default:
		// loading error, abort
		return fmt.Errorf("failed to load public identity: %w", err)
	}

	// export verification
	_, err = publicIdentity.ExportAnnouncement()
	if err != nil {
		return fmt.Errorf("faile to create announcement export cache: %s", err)
	}
	_, err = publicIdentity.ExportStatus()
	if err != nil {
		return fmt.Errorf("faile to create status export cache: %s", err)
	}

	// Always update the navigator in any case in order to sync the reference to the active struct of the identity.
	navigator.UpdateHub(publicIdentity.Hub())
	return nil
}

func prepPublicIdentityMgmt() error {
	publicIdentityUpdateTask = module.NewTask(
		"maintain public identity",
		maintainPublicIdentity,
	)

	return module.RegisterEventHook(
		"config",
		"config change",
		"update public identity from config",
		func(_ context.Context, _ interface{}) error {
			// trigger update in 5 minutes
			publicIdentityUpdateTask.Schedule(time.Now().Add(5 * time.Minute))
			return nil
		},
	)
}

func maintainPublicIdentity(ctx context.Context, task *modules.Task) error {
	changed, err := publicIdentity.MaintainAnnouncement()
	if err != nil {
		return fmt.Errorf("failed to maintain announcement: %w", err)
	}

	if !changed {
		return nil
	}

	// export announcement
	announcementData, err := publicIdentity.ExportAnnouncement()
	if err != nil {
		return fmt.Errorf("failed to export announcement: %w", err)
	}

	// forward to other connected Hubs
	for _, controller := range docks.GetAllControllers() {
		controller.SendHubAnnouncement(announcementData)
	}

	// manage docks in order to react to possibly changed transports
	if managePiersTask != nil {
		managePiersTask.Queue()
	}

	return nil
}
