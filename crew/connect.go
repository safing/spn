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
// TODO: Find a nice way to parallelize route creation.
var connectLock sync.Mutex

// HandleSluiceRequest handles a sluice request to build a tunnel.
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

// Tunnel represents the local information and endpoint of a data tunnel.
type Tunnel struct {
	connInfo *network.Connection
	conn     net.Conn
}

func (t *Tunnel) handle(ctx context.Context) (err error) {
	// Find possible routes.
	routes, err := navigator.Main.FindRoutes(
		t.connInfo.Entity.IP,
		t.connInfo.TunnelOpts,
		10,
	)
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
	var dstTerminal terminal.OpTerminal
	for tries, route = range routes.All {
		dstPin, dstTerminal, err = establishRoute(route)
		if err == nil {
			break
		}
	}
	navigator.Main.PushPinChanges()

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
	_, tErr := NewConnectOp(dstTerminal, request, t.conn)
	if tErr != nil {
		tErr = tErr.Wrap("failed to initialize tunnel")

		t.connInfo.Lock()
		defer t.connInfo.Unlock()
		t.connInfo.Failed(tErr.Error(), "")
		t.connInfo.Save()

		// TODO: try with another route?
		return tErr
	}

	t.connInfo.Lock()
	defer t.connInfo.Unlock()
	addTunnelContextToConnection(t.connInfo, route)
	t.connInfo.Save()

	log.Infof("spn/crew: connected to %s via %s", request, dstPin.Hub)
	return nil
}

type hopCheck struct {
	pin       *navigator.Pin
	route     *navigator.Route
	expansion *docks.ExpansionTerminal
	authOp    *access.AuthorizeOp
}

func establishRoute(route *navigator.Route) (dstPin *navigator.Pin, dstTerminal terminal.OpTerminal, err error) {
	connectLock.Lock()
	defer connectLock.Unlock()

	// Check for path length.
	if len(route.Path) < 1 {
		return nil, nil, errors.New("path too short")
	}

	// Get home hub.
	previousHop, homeTerminal := navigator.Main.GetHome()
	if previousHop == nil || homeTerminal == nil {
		return nil, nil, navigator.ErrHomeHubUnset
	}
	// Convert to interface for later use.
	var previousTerminal terminal.OpTerminal = homeTerminal

	// Check if first hub in path is the home hub.
	if route.Path[0].HubID != previousHop.Hub.ID {
		return nil, nil, errors.New("path start does not match home hub")
	}

	// Check if path only exists of home hub.
	if len(route.Path) == 1 {
		return previousHop, previousTerminal, nil
	}

	// TODO: Check what needs locking.

	// Build path and save created paths.
	hopChecks := make([]*hopCheck, 0, len(route.Path)-1)
	for i, hop := range route.Path[1:] {
		// Check if we already have a connection to the Hub.
		activeTerminal := hop.Pin().GetActiveTerminal()
		if activeTerminal != nil {
			previousHop = hop.Pin()
			previousTerminal = activeTerminal
			continue
		}

		// Expand to next Hub.
		expansion, authOp, tErr := expand(previousTerminal, previousHop, hop.Pin())
		if tErr != nil {
			return nil, nil, tErr.Wrap("failed to expand to %s", hop.Pin())
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
				return nil, nil, tErr.Wrap("failed to authenticate to %s", check.pin.Hub)
			}
		case <-time.After(3 * time.Second):
			return nil, nil, terminal.ErrTimeout.With("timed out waiting for auth to %s", check.pin.Hub)
		}

		// Add terminal extension to the map.
		check.pin.SetActiveTerminal(&navigator.PinConnection{
			Terminal: check.expansion,
			Route:    check.route,
		})
		log.Infof("spn/crew: added conn to %s: %s", check.pin, check.route)
	}

	// Return last hop.
	return previousHop, previousTerminal, nil
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

// TunnelContext holds additional information about the tunnel to be added to a
// connection.
type TunnelContext struct {
	Path       []*TunnelContextHop
	PathCost   float32
	RoutingAlg string
}

// TunnelContextHop holds hop data for TunnelContext.
type TunnelContextHop struct {
	ID   string
	Name string
	IPv4 *TunnelContextHopIPInfo `json:",omitempty"`
	IPv6 *TunnelContextHopIPInfo `json:",omitempty"`
}

// TunnelContextHopIPInfo holds hop IP data for TunnelContextHop.
type TunnelContextHopIPInfo struct {
	IP      net.IP
	Country string
	ASN     uint
	ASOwner string
}

func addTunnelContextToConnection(connInfo *network.Connection, route *navigator.Route) {
	// Create and add basic info.
	tunnelCtx := &TunnelContext{
		Path:       make([]*TunnelContextHop, len(route.Path)),
		PathCost:   route.TotalCost,
		RoutingAlg: route.Algorithm,
	}
	connInfo.TunnelContext = tunnelCtx

	// Add path info.
	for i, hop := range route.Path {
		// Add hub info.
		hopCtx := &TunnelContextHop{
			ID:   hop.HubID,
			Name: hop.Pin().Hub.Info.Name,
		}
		tunnelCtx.Path[i] = hopCtx
		// Add hub IPv4 info.
		if hop.Pin().Hub.Info.IPv4 != nil {
			hopCtx.IPv4 = &TunnelContextHopIPInfo{
				IP: hop.Pin().Hub.Info.IPv4,
			}
			if hop.Pin().LocationV4 != nil {
				hopCtx.IPv4.Country = hop.Pin().LocationV4.Country.ISOCode
				hopCtx.IPv4.ASN = hop.Pin().LocationV4.AutonomousSystemNumber
				hopCtx.IPv4.ASOwner = hop.Pin().LocationV4.AutonomousSystemOrganization
			}
		}
		// Add hub IPv6 info.
		if hop.Pin().Hub.Info.IPv6 != nil {
			hopCtx.IPv6 = &TunnelContextHopIPInfo{
				IP: hop.Pin().Hub.Info.IPv6,
			}
			if hop.Pin().LocationV6 != nil {
				hopCtx.IPv6.Country = hop.Pin().LocationV6.Country.ISOCode
				hopCtx.IPv6.ASN = hop.Pin().LocationV6.AutonomousSystemNumber
				hopCtx.IPv6.ASOwner = hop.Pin().LocationV6.AutonomousSystemOrganization
			}
		}
	}
}
