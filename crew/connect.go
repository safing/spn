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
	"github.com/safing/portmaster/profile/endpoints"
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
	module.StartWorker("tunnel handler", t.connectWorker)
}

// Tunnel represents the local information and endpoint of a data tunnel.
type Tunnel struct {
	connInfo *network.Connection
	conn     net.Conn

	dstPin      *navigator.Pin
	dstTerminal terminal.Terminal
	route       *navigator.Route
	failedTries int
	stickied    bool
}

func (t *Tunnel) connectWorker(ctx context.Context) (err error) {
	// Get tracing logger.
	ctx, tracer := log.AddTracer(ctx)
	defer tracer.Submit()

	// Check the status of the Home Hub.
	home, homeTerminal := navigator.Main.GetHome()
	if home == nil || homeTerminal == nil || homeTerminal.IsBeingAbandoned() {
		reportConnectError(terminal.ErrUnknownError.With("home terminal is abandoned"))

		t.connInfo.Lock()
		defer t.connInfo.Unlock()
		t.connInfo.Failed("SPN not ready for tunneling", "")
		t.connInfo.Save()

		tracer.Infof("spn/crew: not tunneling %s, as the SPN is not ready", t.connInfo)
		return nil
	}

	// Create path through the SPN.
	err = t.establish(ctx)
	if err != nil {
		log.Warningf("spn/crew: failed to establish route for %s: %s", t.connInfo, err)

		// TODO: Clean this up.
		t.connInfo.Lock()
		defer t.connInfo.Unlock()
		t.connInfo.Failed(fmt.Sprintf("failed to establish route: %s", err), "")
		t.connInfo.Save()

		tracer.Warningf("spn/crew: failed to establish route for %s: %s", t.connInfo, err)
		return nil
	}

	// Connect via established tunnel.
	_, tErr := NewConnectOp(t)
	if tErr != nil {
		tErr = tErr.Wrap("failed to initialize tunnel")
		reportConnectError(tErr)

		t.connInfo.Lock()
		defer t.connInfo.Unlock()
		t.connInfo.Failed(tErr.Error(), "")
		t.connInfo.Save()

		// TODO: try with another route?
		tracer.Warningf("spn/crew: failed to initialize tunnel for %s: %s", t.connInfo, err)
		return tErr
	}

	t.connInfo.Lock()
	defer t.connInfo.Unlock()
	addTunnelContextToConnection(t)
	t.connInfo.Save()

	tracer.Infof("spn/crew: connected %s via %s", t.connInfo, t.dstPin.Hub)
	return nil
}

func (t *Tunnel) establish(ctx context.Context) (err error) {
	var routes *navigator.Routes

	// Check if the destination sticks to a Hub.
	sticksTo := getStickiedHub(t.connInfo)
	switch {
	case sticksTo == nil:
		// Continue.

	case sticksTo.Avoid:
		log.Tracer(ctx).Tracef("spn/crew: avoiding %s", sticksTo.Pin.Hub)

		// Build avoid policy.
		avoidPolicy := make([]endpoints.Endpoint, 0, 2)
		// Exclude countries of the hub to be avoided.
		// This helps to select a destination hub that is more different than say,
		// the other hub in the same datacenter as the one to be avoided.
		if sticksTo.Pin.LocationV4 != nil &&
			sticksTo.Pin.LocationV4.Country.ISOCode != "" {
			avoidPolicy = append(avoidPolicy, &endpoints.EndpointCountry{
				Country: sticksTo.Pin.LocationV4.Country.ISOCode,
			})
			log.Tracer(ctx).Tracef("spn/crew: avoiding country %s via IPv4 location", sticksTo.Pin.LocationV4.Country.ISOCode)
		}
		if sticksTo.Pin.LocationV6 != nil &&
			sticksTo.Pin.LocationV6.Country.ISOCode != "" {
			avoidPolicy = append(avoidPolicy, &endpoints.EndpointCountry{
				Country: sticksTo.Pin.LocationV6.Country.ISOCode,
			})
			log.Tracer(ctx).Tracef("spn/crew: avoiding country %s via IPv6 location", sticksTo.Pin.LocationV6.Country.ISOCode)
		}

		// Append to policies
		t.connInfo.TunnelOpts.HubPolicies = append(t.connInfo.TunnelOpts.HubPolicies, avoidPolicy)

	default:
		log.Tracer(ctx).Tracef("spn/crew: using stickied %s", sticksTo.Pin.Hub)

		// Check if the stickied Hub has an active terminal.
		dstTerminal := sticksTo.Pin.GetActiveTerminal()
		if dstTerminal != nil {
			t.dstPin = sticksTo.Pin
			t.dstTerminal = dstTerminal
			t.route = sticksTo.Route
			t.stickied = true
			return nil
		}

		// If not, attempt to find a route to the stickied hub.
		routes, err = navigator.Main.FindRouteToHub(
			sticksTo.Pin.Hub.ID,
			t.connInfo.TunnelOpts,
			navigator.DefaultMaxFindMatches,
		)
		if err != nil {
			log.Tracer(ctx).Tracef("spn/crew: failed to find route to stickied %s: %s", sticksTo.Pin.Hub, err)
			routes = nil
		} else {
			t.stickied = true
		}
	}

	// Find possible routes to destination.
	if routes == nil {
		log.Tracer(ctx).Trace("spn/crew: finding routes...")
		routes, err = navigator.Main.FindRoutes(
			t.connInfo.Entity.IP,
			t.connInfo.TunnelOpts,
			navigator.DefaultMaxFindMatches,
		)
		if err != nil {
			return fmt.Errorf("failed to find routes to %s: %w", t.connInfo.Entity.IP, err)
		}
	}

	// Check if routes are okay (again).
	if len(routes.All) == 0 {
		return fmt.Errorf("no routes to %s", t.connInfo.Entity.IP)
	}

	// Try routes until one succeeds.
	log.Tracer(ctx).Trace("spn/crew: establishing route...")
	var dstPin *navigator.Pin
	var dstTerminal terminal.Terminal
	for tries, route := range routes.All {
		dstPin, dstTerminal, err = establishRoute(route)
		if err != nil {
			continue
		}

		// Assign route data to tunnel.
		t.dstPin = dstPin
		t.dstTerminal = dstTerminal
		t.route = route
		t.failedTries = tries

		// Push changes to Pins and return.
		navigator.Main.PushPinChanges()
		return nil
	}

	return fmt.Errorf("failed to establish a route to %s: %w", t.connInfo.Entity.IP, err)
}

type hopCheck struct {
	pin       *navigator.Pin
	route     *navigator.Route
	expansion *docks.ExpansionTerminal
	authOp    *access.AuthorizeOp
}

func establishRoute(route *navigator.Route) (dstPin *navigator.Pin, dstTerminal terminal.Terminal, err error) {
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
	var previousTerminal terminal.Terminal = homeTerminal

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
		case tErr := <-check.authOp.Result:
			if !tErr.Is(terminal.ErrExplicitAck) {
				return nil, nil, tErr.Wrap("failed to authenticate to %s", check.pin.Hub)
			}
		case <-time.After(3 * time.Second):
			return nil, nil, terminal.ErrTimeout.With("waiting for auth to %s", check.pin.Hub)
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

func expand(fromTerminal terminal.Terminal, from, to *navigator.Pin) (expansion *docks.ExpansionTerminal, authOp *access.AuthorizeOp, tErr *terminal.Error) {
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

	tunnel *Tunnel
}

// GetExitNodeID returns the ID of the exit node.
// It returns an empty string in case no path exists.
func (tc *TunnelContext) GetExitNodeID() string {
	if len(tc.Path) == 0 {
		return ""
	}

	return tc.Path[len(tc.Path)-1].ID
}

// StopTunnel stops the tunnel.
func (tc *TunnelContext) StopTunnel() error {
	if tc.tunnel != nil && tc.tunnel.conn != nil {
		return tc.tunnel.conn.Close()
	}
	return nil
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

func addTunnelContextToConnection(t *Tunnel) {
	// Create and add basic info.
	tunnelCtx := &TunnelContext{
		Path:       make([]*TunnelContextHop, len(t.route.Path)),
		PathCost:   t.route.TotalCost,
		RoutingAlg: t.route.Algorithm,
		tunnel:     t,
	}
	t.connInfo.TunnelContext = tunnelCtx

	// Add path info.
	for i, hop := range t.route.Path {
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
