package captain

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/safing/portbase/updater"
	"github.com/safing/portmaster/updates"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/navigator"
)

var (
	intelResource           *updater.File
	intelResourcePath       = "intel/spn/main-intel.dsd"
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
