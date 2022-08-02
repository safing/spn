package navigator

import (
	"errors"
	"time"

	"github.com/safing/portbase/config"
	"github.com/safing/portbase/modules"
	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/spn/conf"
)

var (
	// ErrHomeHubUnset is returned when the Home Hub is required and not set.
	ErrHomeHubUnset = errors.New("map has no Home Hub set")

	// ErrEmptyMap is returned when the Map is empty.
	ErrEmptyMap = errors.New("map is empty")

	// ErrHubNotFound is returned when the Hub was not found on the Map.
	ErrHubNotFound = errors.New("hub not found")
)

var (
	module *modules.Module

	// Main is the primary map used.
	Main *Map

	devMode config.BoolOption
)

func init() {
	module = modules.Register("navigator", prep, start, stop, "terminal", "geoip", "netenv")
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

	// Wait for geoip databases to be ready.
	// Try again if not yet ready, as this is critical.
	// The "wait" parameter times out after 1 second.
	// Allow 30 seconds for both databases to load.
geoInitCheck:
	for i := 0; i < 30; i++ {
		switch {
		case !geoip.IsInitialized(false, true): // First, IPv4.
		case !geoip.IsInitialized(true, true): // Then, IPv6.
		default:
			break geoInitCheck
		}
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
		err := registerMetrics()
		if err != nil {
			return err
		}
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
