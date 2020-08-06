package navigator

import (
	"context"
	"errors"
	"path"
	"time"

	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/hub"
)

var (
	module *modules.Module
)

func init() {
	module = modules.Register("navigator", prep, start, nil, "base", "geoip", "netenv")
}

func prep() error {
	return nil
}

func start() error {
	module.StartServiceWorker("hub subscription", 0, hubSubFeeder)

	InitializeNavigator()
	hub.SetNavigatorAccess(GetHub)

	return nil
}

func hubSubFeeder(ctx context.Context) error {
	// TODO: fix bug and remove workaround
	time.Sleep(1 * time.Second)

	sub, err := db.Subscribe(query.New(hub.PublicHubs))
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			sub.Cancel()
			return nil
		case r := <-sub.Feed:
			if r == nil {
				return errors.New("subscription ended")
			}

			if r.Meta().IsDeleted() {
				RemovePublicHub(path.Base(r.Key()))
				continue
			}

			h, err := hub.EnsureHub(r)
			if err != nil {
				log.Warningf("spn/captain: hub ingestion hook received invalid Hub: %s", err)
				continue
			}

			UpdateHub(h)
			continue
		}
	}
}
