package captain

import (
	"github.com/safing/portmaster/updates"
)

func init() {
	updates.UpgradeCore = false
	updates.MandatoryUpdates = []string{}
}
