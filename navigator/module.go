package navigator

import (
	"errors"
	"time"

	"github.com/safing/portbase/modules"
	"github.com/safing/spn/hub"
)

var (
	// ErrHomeHubUnset is returned when the Home Hub is required and not set.
	ErrHomeHubUnset = errors.New("map has no Home Hub set")

	// ErrEmptyMap is returned when the Map is empty.
	ErrEmptyMap = errors.New("map is empty")
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
	Main.InitializeFromDatabase(hub.PublicHubs)
	err := Main.RegisterHubUpdateHook(hub.PublicHubs)
	if err != nil {
		return err
	}

	// TODO: delete superseded hubs after x amount of time

	module.NewTask("update states", Main.updateStates).
		Repeat(1 * time.Hour).
		Schedule(time.Now().Add(3 * time.Minute))

	return nil
}
