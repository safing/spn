package conf

import (
	"github.com/tevino/abool"
)

var (
	publicHub = abool.New()
	client    = abool.New()
)

func PublicHub() bool {
	return publicHub.IsSet()
}

func EnablePublicHub(enable bool) {
	publicHub.SetTo(enable)
}

func Client() bool {
	return client.IsSet()
}

func EnableClient(enable bool) {
	client.SetTo(enable)
}
