package crew

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/network"
	"github.com/safing/portmaster/network/packet"
	"github.com/safing/spn/access"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/navigator"
	"github.com/safing/spn/terminal"
)

// connectLock locks all routing operations to mitigate racy stuff for now.
// FIXME: Find a nice way to parallelize route creation.
var connectLock sync.Mutex

func HandleSluiceRequest(connInfo *network.Connection, conn net.Conn) {
	if conn == nil {
		log.Debugf("spn/crew: closing tunnel for %s before starting because of shutdown", connInfo)

		// This is called within the connInfo lock.
		connInfo.Failed("tunnel entry closed", "")
		connInfo.SaveWhenFinished()
		return
	}

	t := &Tunnel{
		connInfo: connInfo,
		conn:     conn,
	}
	module.StartWorker("tunnel handler", t.handle)
}

type Tunnel struct {
	connInfo *network.Connection
	conn     net.Conn
}

func (t *Tunnel) handle(ctx context.Context) (err error) {
	// Find possible routes.
	routes, err := navigator.Main.FindRoutes(t.connInfo.Entity.IP, nil, 10)
	if err != nil {
		log.Warningf("spn/crew: failed to find route for %s: %s", t.connInfo, err)

		// TODO: Clean this up.
		t.connInfo.Lock()
		defer t.connInfo.Unlock()
		t.connInfo.Failed(fmt.Sprintf("failed to find route: %s", err), "")
		t.connInfo.Save()

		return nil
	}

	// Try routes until one succeeds.
	var tries int
	var route *navigator.Route
	var dstPin *navigator.Pin
	for tries, route = range routes.All {
		dstPin, err = establishRoute(route)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Warningf("spn/crew: failed to establish route for %s - tried %d routes: %s", t.connInfo, tries+1, err)

		// TODO: Clean this up.
		t.connInfo.Lock()
		defer t.connInfo.Unlock()
		t.connInfo.Failed(fmt.Sprintf("failed to establish route - tried %d routes: %s", tries+1, err), "")
		t.connInfo.Save()

		return nil
	}
	log.Infof("spn/crew: established route to %s with %d failed tries", dstPin.Hub, tries)

	// Create request and connect.
	request := &ConnectRequest{
		Domain:   t.connInfo.Entity.Domain,
		IP:       t.connInfo.Entity.IP,
		Protocol: packet.IPProtocol(t.connInfo.Entity.Protocol),
		Port:     t.connInfo.Entity.Port,
	}
	_, tErr := NewConnectOp(dstPin.Connection.Terminal, request, t.conn)
	if tErr != nil {
		tErr = tErr.Wrap("failed to initialize tunnel")

		// TODO: Clean this up.
		t.connInfo.Lock()
		defer t.connInfo.Unlock()
		t.connInfo.Failed(tErr.Error(), "")
		t.connInfo.Save()

		// FIXME: try with another route?
		return tErr
	}

	log.Infof("spn/crew: connected to %s via %s", request, dstPin.Hub)
	return nil
}

type hopCheck struct {
	pin       *navigator.Pin
	route     *navigator.Route
	expansion *docks.ExpansionTerminal
	authOp    *access.AuthorizeOp
}

func establishRoute(route *navigator.Route) (dstPin *navigator.Pin, err error) {
	connectLock.Lock()
	defer connectLock.Unlock()

	// Check for path length.
	if len(route.Path) <= 1 {
		return nil, errors.New("path too short")
	}

	// Get home hub.
	var previousHop *navigator.Pin
	var previousTerminal terminal.OpTerminal
	previousHop, previousTerminal = navigator.Main.GetHome()

	// Check if first hub in path is the home hub.
	if route.Path[0].HubID != previousHop.Hub.ID {
		return nil, errors.New("path start does not match home hub")
	}

	// FIXME: Check what needs locking.

	// Build path and save created paths.
	hopChecks := make([]*hopCheck, 0, len(route.Path)-1)
	for i, hop := range route.Path[1:] {
		// Check if we already have a connection to the Hub.
		if hop.Pin().Connection != nil && !hop.Pin().Connection.Terminal.IsAbandoned() {
			previousHop = hop.Pin()
			previousTerminal = hop.Pin().Connection.Terminal
			continue
		}

		// Expand to next Hub.
		expansion, authOp, tErr := expand(previousTerminal, previousHop, hop.Pin())
		if tErr != nil {
			return nil, tErr.Wrap("failed to expand to %s", hop.Pin())
		}

		// Add for checking results later.
		hopChecks = append(hopChecks, &hopCheck{
			pin:       hop.Pin(),
			route:     route.CopyUpTo(i + 2),
			expansion: expansion,
			authOp:    authOp,
		})

		// Save previous pin for next loop or end.
		previousHop = hop.Pin()
		previousTerminal = expansion
	}

	// Check results.
	for _, check := range hopChecks {
		// Wait for authOp result.
		select {
		case tErr := <-check.authOp.Ended:
			if !tErr.Is(terminal.ErrExplicitAck) {
				return nil, tErr.Wrap("failed to authenticate to %s", check.pin.Hub)
			}
		case <-time.After(3 * time.Second):
			return nil, terminal.ErrTimeout.With("timed out waiting for auth to %s", check.pin.Hub)
		}

		// Add terminal extension to the map.
		check.pin.Connection = &navigator.PinConnection{
			Terminal: check.expansion,
			Route:    check.route,
		}
		log.Errorf("spn/crew: added conn to %s: %s", check.pin, check.route)
	}

	// Return last hop.
	return previousHop, nil
}

func expand(fromTerminal terminal.OpTerminal, from, to *navigator.Pin) (expansion *docks.ExpansionTerminal, authOp *access.AuthorizeOp, tErr *terminal.Error) {
	expansion, tErr = docks.ExpandTo(fromTerminal, to.Hub.ID, to.Hub)
	if tErr != nil {
		return nil, nil, tErr.Wrap("failed to expand to %s", to.Hub)
	}

	authOp, tErr = access.AuthorizeToTerminal(expansion)
	if tErr != nil {
		expansion.Abandon(nil)
		return nil, nil, tErr.Wrap("failed to authorize")
	}

	log.Infof("spn/crew: expanded to %s (from %s)", to.Hub, from.Hub)
	return expansion, authOp, nil
}
