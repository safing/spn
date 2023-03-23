package navigator

import (
	"bytes"
	"errors"
	"fmt"
	mrand "math/rand"
	"net"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/safing/portbase/api"
	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/portmaster/network/netutils"
)

func registerRouteAPIEndpoints() error {
	if err := api.RegisterEndpoint(api.Endpoint{
		Path:        `spn/map/{map:[A-Za-z0-9]{1,255}}/route/to/{destination:[a-z0-9_\.:-]{1,255}}`,
		Read:        api.PermitUser,
		BelongsTo:   module,
		ActionFunc:  handleRouteCalculationRequest,
		Name:        "Calculate Route through SPN",
		Description: "Returns a textual representation of the routing process.",
	}); err != nil {
		return err
	}

	return nil
}

func handleRouteCalculationRequest(ar *api.Request) (msg string, err error) {
	// Get map.
	m, ok := getMapForAPI(ar.URLVars["map"])
	if !ok {
		return "", errors.New("map not found")
	}

	var introText string

	// Parse destination.
	var locationV4, locationV6 *geoip.Location
	var dstIP net.IP
	destination := ar.URLVars["destination"]
	matchFor := DestinationHub
	opts := m.defaultOptions()
	switch {
	case destination == "":
		// Destination is required.
		return "", errors.New("no destination provided")

	case destination == "home":
		// Simulate finding home hub.
		locations, ok := netenv.GetInternetLocation()
		if !ok || len(locations.All) == 0 {
			return "", errors.New("failed to locate own device for finding home hub")
		}
		introText = fmt.Sprintf("looking for home hub near %s and %s", locations.BestV4(), locations.BestV6())
		locationV4 = locations.BestV4().LocationOrNil()
		locationV6 = locations.BestV6().LocationOrNil()
		matchFor = HomeHub

	case net.ParseIP(destination) != nil:
		dstIP = net.ParseIP(destination)
		if ip4 := dstIP.To4(); ip4 != nil {
			locationV4, err = geoip.GetLocation(dstIP)
			if err != nil {
				return "", fmt.Errorf("failed to get geoip location for %s: %w", dstIP, err)
			}
			introText = fmt.Sprintf("looking for route to %s at %s", dstIP, formatLocation(locationV4))
		} else {
			locationV6, err = geoip.GetLocation(dstIP)
			if err != nil {
				return "", fmt.Errorf("failed to get geoip location for %s: %w", dstIP, err)
			}
			introText = fmt.Sprintf("looking for route to %s at %s", dstIP, formatLocation(locationV6))
		}

	case netutils.IsValidFqdn(destination):
		fallthrough
	case netutils.IsValidFqdn(destination + "."):

		// Resolve name to IPs.
		ips, err := net.DefaultResolver.LookupIP(ar.Context(), "ip", destination)
		if err != nil {
			return "", fmt.Errorf("failed to lookup IP address of %s: %w", destination, err)
		}

		// Shuffle IPs.
		if len(ips) >= 2 {
			r := mrand.New(mrand.NewSource(time.Now().UnixNano()))
			r.Shuffle(len(ips), func(i, j int) {
				ips[i], ips[j] = ips[j], ips[i]
			})
		}

		// Get IP location.
		dstIP = ips[0]
		if ip4 := dstIP.To4(); ip4 != nil {
			locationV4, err = geoip.GetLocation(dstIP)
			if err != nil {
				return "", fmt.Errorf("failed to get geoip location for %s: %w", dstIP, err)
			}
			introText = fmt.Sprintf("looking for route to %s at %s\n(ignoring %d additional IPs returned by DNS)", dstIP, formatLocation(locationV4), len(ips)-1)
		} else {
			locationV6, err = geoip.GetLocation(dstIP)
			if err != nil {
				return "", fmt.Errorf("failed to get geoip location for %s: %w", dstIP, err)
			}
			introText = fmt.Sprintf("looking for route to %s at %s\n(ignoring %d additional IPs returned by DNS)", dstIP, formatLocation(locationV6), len(ips)-1)
		}

	default:
		return "", errors.New("invalid destination provided")
	}

	// Start formatting output.
	lines := []string{
		"Routing simulation: " + introText,
		"Please not that this routing simulation does match the behavior of regular routing to 100%.",
		"",
	}

	// Print options.
	// ==================

	lines = append(lines, "Routing Options:")
	lines = append(lines, "Algorithm: "+opts.RoutingProfile)
	lines = append(lines, fmt.Sprintf("Require Verified Owners: %s", opts.RequireVerifiedOwners))
	lines = append(lines, fmt.Sprintf("Require Trusted Exit: %v", opts.RequireTrustedDestinationHubs))
	lines = append(lines, "Hub Policy: ")
	for _, ep := range opts.HubPolicies {
		lines = append(lines, ep.String())
	}
	lines = append(lines, "")

	// Find nearest hubs.
	// ==================

	// Start operating in map.
	m.RLock()
	defer m.RUnlock()
	// Check if map is populated.
	if m.isEmpty() {
		return "", ErrEmptyMap
	}

	// Find nearest hubs.
	nbPins, err := m.findNearestPins(locationV4, locationV6, opts, matchFor, true)
	if err != nil {
		return "", fmt.Errorf("failed to search for nearby pins: %w", err)
	}

	// Print found exits to table.
	lines = append(lines, "Considered Exits (cheapest 10% are shuffled)")
	buf := bytes.NewBuffer(nil)
	tabWriter := tabwriter.NewWriter(buf, 8, 4, 3, ' ', 0)
	fmt.Fprint(tabWriter, "Hub Name\tCost\tLocation\n")
	for _, nbPin := range nbPins.pins {
		fmt.Fprintf(tabWriter,
			"%s\t%.0f\t%s\n",
			nbPin.pin.Hub.Name(),
			nbPin.cost,
			formatMultiLocation(nbPin.pin.LocationV4, nbPin.pin.LocationV6),
		)
	}
	_ = tabWriter.Flush()
	lines = append(lines, buf.String())

	// Print too expensive exits to table.
	lines = append(lines, "Too Expensive Exits:")
	buf = bytes.NewBuffer(nil)
	tabWriter = tabwriter.NewWriter(buf, 8, 4, 3, ' ', 0)
	fmt.Fprint(tabWriter, "Hub Name\tCost\tLocation\n")
	for _, nbPin := range nbPins.debug.tooExpensive {
		fmt.Fprintf(tabWriter,
			"%s\t%.0f\t%s\n",
			nbPin.pin.Hub.Name(),
			nbPin.cost,
			formatMultiLocation(nbPin.pin.LocationV4, nbPin.pin.LocationV6),
		)
	}
	_ = tabWriter.Flush()
	lines = append(lines, buf.String())

	// Print disregarded exits to table.
	lines = append(lines, "Disregarded Exits:")
	buf = bytes.NewBuffer(nil)
	tabWriter = tabwriter.NewWriter(buf, 8, 4, 3, ' ', 0)
	fmt.Fprint(tabWriter, "Hub Name\tReason\tStates\n")
	for _, nbPin := range nbPins.debug.disregarded {
		fmt.Fprintf(tabWriter,
			"%s\t%s\t%s\n",
			nbPin.pin.Hub.Name(),
			nbPin.reason,
			nbPin.pin.State,
		)
	}
	_ = tabWriter.Flush()
	lines = append(lines, buf.String())

	// Find routes.
	// ============

	// Unless we looked for a home node.
	if destination == "home" {
		return strings.Join(lines, "\n"), nil
	}

	// Find routes.
	routes, err := m.findRoutes(nbPins, opts)
	if err != nil {
		return "", fmt.Errorf("failed to find routes: %w", err)
	}

	// Print found routes to table.
	lines = append(lines, "Considered Routes (cheapest 10% are shuffled)")
	buf = bytes.NewBuffer(nil)
	tabWriter = tabwriter.NewWriter(buf, 8, 4, 3, ' ', 0)
	fmt.Fprint(tabWriter, "Cost\tPath\n")
	for _, route := range routes.All {
		fmt.Fprintf(tabWriter,
			"%.0f\t%s\n",
			route.TotalCost,
			formatRoute(route, dstIP),
		)
	}
	_ = tabWriter.Flush()
	lines = append(lines, buf.String())

	return strings.Join(lines, "\n"), nil
}

func formatLocation(loc *geoip.Location) string {
	return fmt.Sprintf(
		"%s (AS%d %s)",
		loc.Country.ISOCode,
		loc.AutonomousSystemNumber,
		loc.AutonomousSystemOrganization,
	)
}

func formatMultiLocation(a, b *geoip.Location) string {
	switch {
	case a != nil:
		return formatLocation(a)
	case b != nil:
		return formatLocation(b)
	default:
		return ""
	}
}

func formatRoute(r *Route, dst net.IP) string {
	s := make([]string, 0, len(r.Path)+1)
	for i, hop := range r.Path {
		if i == 0 {
			s = append(s, hop.pin.Hub.Name())
		} else {
			s = append(s, fmt.Sprintf(">> %.2fc >> %s", hop.Cost, hop.pin.Hub.Name()))
		}
	}
	s = append(s, fmt.Sprintf(">> %.2fc >> %s", r.DstCost, dst))
	return strings.Join(s, " ")
}
