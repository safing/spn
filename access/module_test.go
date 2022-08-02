package access

import (
	"testing"

	"github.com/safing/portmaster/core/pmtesting"
	"github.com/safing/spn/conf"
)

func TestMain(m *testing.M) {
	conf.EnableClient(true)
	pmtesting.TestMain(m, module)
}
