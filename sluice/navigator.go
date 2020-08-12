package sluice

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/access"
	"github.com/safing/spn/api"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/navigator"
)

var (
	tunnelBuildLock sync.Mutex
)

func buildTunnel(r *Request) (*api.Call, error) {
	// only build one tunnel at a time - for now
	tunnelBuildLock.Lock()
	defer tunnelBuildLock.Unlock()

	// get nearest ports
	col, err := navigator.FindNearestPorts([]net.IP{r.Info.Dst})
	if err != nil {
		return nil, fmt.Errorf("failed to find nearest hubs: %s", err)
	}
	if col.Len() == 0 {
		return nil, fmt.Errorf("no hubs found near %s", r.Info.Dst)
	}
	log.Tracef("spn/sluice: found %d near hubs, first: %s", col.Len(), col.All[0].Port.Hub)

	// find best path
	ports, err := navigator.FindPathToPorts([]*navigator.Port{col.All[0].Port})
	if err != nil {
		return nil, fmt.Errorf("failed to find path to port: %s", err)
	}
	if !ports[0].HasActiveRoute() {
		return nil, errors.New("first port in route has no active client")
	}
	log.Tracef("spn/sluice: found route with %d (additional) hops and with exit %s", len(ports)-1, ports[len(ports)-1].Hub)

	// get access code
	accessCode, err := access.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get access code: %s", err)
	}

	// build path
	currentPort := ports[0]
	for i := 1; i < len(ports); i++ {
		// hop
		newAPI, err := currentPort.ActiveAPI.Hop(conf.CurrentVersion, ports[i].Hub)
		if err != nil {
			return nil, fmt.Errorf("failed to hop to %s: %s", ports[i], err)
		}
		log.Tracef("spn/sluice: hopped to %s", ports[i].Hub)

		// authenticate
		err = newAPI.UserAuth(accessCode)
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate to %s: %s", ports[i], err)
		}
		log.Tracef("spn/sluice: authenticated to %s", ports[i].Hub)

		// set for next hop
		currentPort = ports[i]
		currentPort.ActiveAPI = newAPI
	}

	// init tunnel
	log.Tracef("spn/sluice: initiating tunnel at %s", currentPort)
	tunnel, err := currentPort.ActiveAPI.Tunnel(r.Domain, r.Info.Dst, r.Info.Protocol, r.Info.DstPort)
	if err != nil {
		return nil, fmt.Errorf("failed to init tunnel: %s", err)
	}

	log.Tracef("spn/sluice: tunnel to %s:%d complete", r.Info.Dst, r.Info.DstPort)

	return tunnel, nil
}
