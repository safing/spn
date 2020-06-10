package conf

import (
	"github.com/tevino/abool"
)

var (
	enableHub = abool.New()
)

func HubMode() bool {
	return enableHub.IsSet()
}

func EnableHubMode(enable bool) {
	enableHub.SetTo(enable)
}
