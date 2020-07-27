package conf

import (
	"github.com/tevino/abool"
)

var (
	publicHub = abool.New()
)

func PublicHub() bool {
	return publicHub.IsSet()
}

func EnablePublicHub(enable bool) {
	publicHub.SetTo(enable)
}
