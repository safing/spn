package access

import (
	"testing"

	"github.com/safing/portmaster/core/pmtesting"
)

func TestMain(m *testing.M) {
	TestMode()
	pmtesting.TestMain(m, module)
}
