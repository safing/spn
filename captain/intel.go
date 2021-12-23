package captain

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/safing/portbase/updater"
	"github.com/safing/portmaster/updates"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/navigator"
)

var (
	intelResource           *updater.File
	intelResourcePath       = "intel/spn/main-intel.json"
	intelResourceMapName    = "main"
	intelResourceUpdateLock sync.Mutex
)

func registerIntelUpdateHook() error {
	return module.RegisterEventHook(
		updates.ModuleName,
		updates.ResourceUpdateEvent,
		"update SPN intel",
		updateSPNIntel,
	)
}

func updateSPNIntel(ctx context.Context, _ interface{}) (err error) {
	intelResourceUpdateLock.Lock()
	defer intelResourceUpdateLock.Unlock()

	// Only update SPN intel when using the matching map.
	if conf.MainMapName != intelResourceMapName {
		return nil
	}

	// Check if there is something to do.
	if intelResource != nil && !intelResource.UpgradeAvailable() {
		return nil
	}

	// Get intel file and load it from disk.
	intelResource, err = updates.GetFile(intelResourcePath)
	if err != nil {
		return fmt.Errorf("failed to get SPN intel update: %w", err)
	}
	intelData, err := ioutil.ReadFile(intelResource.Path())
	if err != nil {
		return fmt.Errorf("failed to load SPN intel update: %w", err)
	}

	// Parse and apply intel data.
	intel, err := hub.ParseIntel(intelData)
	if err != nil {
		return fmt.Errorf("failed to parse SPN intel update: %w", err)
	}
	return navigator.Main.UpdateIntel(intel)
}

var requiredResources = []string{
	"intel/geoip/geoipv4.mmdb.gz",
	"intel/geoip/geoipv6.mmdb.gz",
}

func loadRequiredResources() error {
	for _, res := range requiredResources {
		_, err := updates.GetFile(res)
		if err != nil {
			return fmt.Errorf("failed to get required resource %s: %w", res, err)
		}
	}
	return nil
}
