package navigator

import (
	"errors"
	"time"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/conf"
)

var (
	// ErrHomeHubUnset is returned when the Home Hub is required and not set.
	ErrHomeHubUnset = errors.New("map has no Home Hub set")

	// ErrEmptyMap is returned when the Map is empty.
	ErrEmptyMap = errors.New("map is empty")
)

var (
	module *modules.Module
	Main   *Map

	devMode config.BoolOption
)

func init() {
	module = modules.Register("navigator", prep, start, stop, "base", "geoip", "netenv")
}

func prep() error {
	return registerAPIEndpoints()
}

func start() error {
	Main = NewMap(conf.MainMapName, true)
	devMode = config.Concurrent.GetAsBool(config.CfgDevModeKey, false)

	err := registerMapDatabase()
	if err != nil {
		return err
	}

	Main.InitializeFromDatabase()
	err = Main.RegisterHubUpdateHook()
	if err != nil {
		return err
	}

	// TODO: delete superseded hubs after x amount of time

	module.NewTask("update states", Main.updateStates).
		Repeat(1 * time.Hour).
		Schedule(time.Now().Add(3 * time.Minute))

	if conf.PublicHub() {
		// Only measure Hubs on public Hubs.
		module.NewTask("measure hubs", Main.measureHubs).
			Repeat(5 * time.Minute).
			Schedule(time.Now().Add(1 * time.Minute))

		// Only register metrics on Hubs, as they only make sense there.
		registerMetrics()

	}

	return nil
}

func stop() error {
	withdrawMapDatabase()

	Main.CancelHubUpdateHook()
	Main.SaveMeasuredHubs()
	Main.Close()

	return nil
}
