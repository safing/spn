package crew

import (
	"context"
	"sync"

	"github.com/safing/portmaster/intel"
	"github.com/safing/portmaster/profile/endpoints"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

var (
	connectingHubLock sync.Mutex
	connectingHub     *hub.Hub
)

// EnableConnecting enables connecting from this Hub.
func EnableConnecting(my *hub.Hub) {
	connectingHubLock.Lock()
	defer connectingHubLock.Unlock()

	connectingHub = my
}

func checkExitPolicy(request *ConnectRequest) *terminal.Error {
	connectingHubLock.Lock()
	defer connectingHubLock.Unlock()

	// Check if connect requests are allowed.
	if connectingHub == nil {
		return terminal.ErrPermissinDenied.With("connect requests disabled")
	}

	// Create entity.
	entity := &intel.Entity{
		Protocol: uint8(request.Protocol),
		Port:     request.Port,
		Domain:   request.Domain,
	}
	entity.SetIP(request.IP)
	entity.FetchData(context.TODO())

	// Check against policy.
	result, reason := connectingHub.GetInfo().ExitPolicy().Match(context.TODO(), entity)
	if result == endpoints.Denied {
		return terminal.ErrPermissinDenied.With("connect request for %s violates the exit policy: %s", request, reason)
	}

	return nil
}
