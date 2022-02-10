package docks

import (
	"testing"

	"github.com/safing/portmaster/core/pmtesting"
	"github.com/safing/spn/access"
	"github.com/safing/spn/conf"
)

func TestMain(m *testing.M) {
	runningTests = true
	conf.EnablePublicHub(true) // Make hub config available.
	access.EnableTestMode()    // Register test zone instead of real ones.
	pmtesting.TestMain(m, module)
}
