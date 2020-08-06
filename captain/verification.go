package captain

import (
	"net"

	"github.com/safing/spn/hub"
)

func init() {
	hub.SetHubIPValidationFn(verifyIPAddress)
}

func verifyIPAddress(h *hub.Hub, ip net.IP) error {

	// ship := ships.SetSail(b., address)
	// FIXME: actually do some checking here

	return nil
}
