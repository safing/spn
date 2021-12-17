package docks

import (
	"testing"

	"github.com/safing/portmaster/core/pmtesting"
	"github.com/safing/spn/conf"
)

func TestMain(m *testing.M) {
	runningTests = true
	conf.EnablePublicHub(true)
	pmtesting.TestMain(m, module)
}
