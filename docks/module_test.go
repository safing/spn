package docks

import (
	"testing"

	"github.com/safing/portmaster/core/pmtesting"
	"github.com/safing/spn/conf"
)

func TestMain(m *testing.M) {
	conf.EnablePublicHub(false)
	pmtesting.TestMain(m, module)
}
