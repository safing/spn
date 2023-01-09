package conf

import (
	"github.com/tevino/abool"
)

var (
	publicHub = abool.New()
	client    = abool.New()
	desktop   = abool.New()
)

// PublicHub returns whether this is a public Hub.
func PublicHub() bool {
	return publicHub.IsSet()
}

// EnablePublicHub enables the public hub mode.
func EnablePublicHub(enable bool) {
	publicHub.SetTo(enable)
}

// Client returns whether this is a client.
func Client() bool {
	return client.IsSet()
}

// EnableClient enables the client mode.
func EnableClient(enable bool) {
	client.SetTo(enable)
}

// DesktopClient returns whether this is a desktop client.
func DesktopClient() bool {
	return desktop.IsSet()
}

// EnableDesktop enables desktop client mode.
func EnableDesktop(enable bool) {
	desktop.SetTo(enable)
}
