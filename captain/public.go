package captain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/safing/spn/navigator"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/hub"
)

var (
	publicIdentity    *cabin.Identity
	publicIdentityKey = "core:spn/public/identity"

	publicIdentityUpdateTask *modules.Task
)

func loadPublicIdentity() (err error) {
	var changed bool

	publicIdentity, changed, err = cabin.LoadIdentity(publicIdentityKey)
	switch err {
	case nil:
		// load was successful
		log.Infof("spn/captain: loaded public hub identity %s", publicIdentity.Hub.ID)
	case database.ErrNotFound:
		// does not exist, create new
		publicIdentity, err = cabin.CreateIdentity(module.Ctx, hub.ScopePublic)
		if err != nil {
			return fmt.Errorf("failed to create new identity: %w", err)
		}
		publicIdentity.SetKey(publicIdentityKey)
		changed = true

		log.Infof("spn/captain: created new public hub identity %s", publicIdentity.ID)
	default:
		// loading error, abort
		return fmt.Errorf("failed to load public identity: %w", err)
	}

	// Save to database if the identity changed.
	if changed {
		err = publicIdentity.Save()
		if err != nil {
			return fmt.Errorf("failed to save new/updated identity to database: %w", err)
		}
	}

	// Always update the navigator in any case in order to sync the reference to the active struct of the identity.
	navigator.Main.UpdateHub(publicIdentity.Hub)
	ok := navigator.Main.SetHome(publicIdentity.ID)
	if !ok {
		return errors.New("failed to set self as home hub")
	}
	return nil
}

func prepPublicIdentityMgmt() error {
	publicIdentityUpdateTask = module.NewTask(
		"maintain public identity",
		maintainPublicIdentity,
	)

	module.NewTask(
		"maintain public status",
		maintainPublicStatus,
	).Repeat(1 * time.Hour)

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
	changed, err := publicIdentity.MaintainAnnouncement(false)
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
	gossipRelayMsg("", GossipHubAnnouncementMsg, announcementData)

	// manage docks in order to react to possibly changed transports
	if managePiersTask != nil {
		managePiersTask.Queue()
	}

	return nil
}

func maintainPublicStatus(ctx context.Context, task *modules.Task) error {
	changed, err := publicIdentity.MaintainStatus(nil, false)
	if err != nil {
		return fmt.Errorf("failed to maintain status: %w", err)
	}

	if !changed {
		return nil
	}

	// export status
	statusData, err := publicIdentity.ExportStatus()
	if err != nil {
		return fmt.Errorf("failed to export status: %w", err)
	}

	// forward to other connected Hubs
	gossipRelayMsg("", GossipHubStatusMsg, statusData)

	return nil
}
