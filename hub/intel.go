package hub

import (
	"errors"
	"fmt"
	"net"

	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portmaster/profile/endpoints"
)

// Intel holds a collection of various security related data collections on Hubs.
type Intel struct {
	// BootstrapHubs is list of transports that also contain an IP and the Hub's ID.
	BootstrapHubs []string
	// TrustedHubs is a list of Hub IDs that are specially designated for more sensitive tasls, such as handling unencrypted traffic.
	TrustedHubs []string

	// AdviseOnlyTrustedHubs advises to only use trusted Hubs regardless of intended purpose.
	AdviseOnlyTrustedHubs bool
	// AdviseOnlyTrustedHomeHubs advises to only use trusted Hubs for Home Hubs.
	AdviseOnlyTrustedHomeHubs bool
	// AdviseOnlyTrustedDestinationHubs advises to only use trusted Hubs for Destination Hubs.
	AdviseOnlyTrustedDestinationHubs bool

	// Hub Advisories advise on the usage of Hubs and take the form of Endpoint Lists that match on both IPv4 and IPv6 addresses and their related data.

	// HubAdvisory always affects all Hubs.
	HubAdvisory []string
	// HomeHubAdvisory is only taken into account when selecting a Home Hub.
	HomeHubAdvisory []string
	// DestinationHubAdvisory is only taken into account when selecting a Destination Hub.
	DestinationHubAdvisory []string

	parsed *ParsedIntel
}

// ParsedIntel holds a collection of parsed intel data.
type ParsedIntel struct {
	// HubAdvisory always affects all Hubs.
	HubAdvisory endpoints.Endpoints

	// HomeHubAdvisory is only taken into account when selecting a Home Hub.
	HomeHubAdvisory endpoints.Endpoints

	// DestinationHubAdvisory is only taken into account when selecting a Destination Hub.
	DestinationHubAdvisory endpoints.Endpoints
}

// Parsed returns the collection of parsed intel data.
func (i *Intel) Parsed() *ParsedIntel {
	return i.parsed
}

// ParseIntel parses Hub intelligence data.
func ParseIntel(data []byte) (*Intel, error) {
	// Load data into struct.
	intel := &Intel{}
	err := dsd.LoadAsFormat(data, dsd.JSON, intel)
	if err != nil {
		return nil, fmt.Errorf("failed to parse data: %w", err)
	}

	// Parse all endpoint lists.
	err = intel.ParseAdvisories()
	if err != nil {
		return nil, err
	}

	return intel, nil
}

// ParseAdvisories parses all advisory endpoint lists.
func (i *Intel) ParseAdvisories() (err error) {
	i.parsed = &ParsedIntel{}

	i.parsed.HubAdvisory, err = endpoints.ParseEndpoints(i.HubAdvisory)
	if err != nil {
		return fmt.Errorf("failed to parse HubAdvisory list: %w", err)
	}

	i.parsed.HomeHubAdvisory, err = endpoints.ParseEndpoints(i.HomeHubAdvisory)
	if err != nil {
		return fmt.Errorf("failed to parse HomeHubAdvisory list: %w", err)
	}

	i.parsed.DestinationHubAdvisory, err = endpoints.ParseEndpoints(i.DestinationHubAdvisory)
	if err != nil {
		return fmt.Errorf("failed to parse DestinationHubAdvisory list: %w", err)
	}

	return nil
}

func ParseBootstrapHub(bootstrapTransport string, mapName string) (*Hub, error) {
	// Parse transport and check Hub ID.
	t, err := ParseTransport(bootstrapTransport)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transport: %s", err)
	}
	if t.Option == "" {
		return nil, errors.New("missing hub ID in URL fragment")
	}
	if _, err := lhash.FromBase58(t.Option); err != nil {
		return nil, fmt.Errorf("hub ID is invalid: %w", err)
	}

	// Parse IP address from transport.
	ip := net.ParseIP(t.Domain)
	if ip == nil {
		return nil, errors.New("invalid IP address (domains are not supported for bootstrapping)")
	}

	// Clean up transport for hub info.
	id := t.Option
	t.Domain = ""
	t.Option = ""

	// Create bootstrap hub.
	bootstrapHub := &Hub{
		ID:  id,
		Map: mapName,
		Info: &Announcement{
			ID:         id,
			Transports: []string{t.String()},
		},
		Status: &Status{},
	}

	// Set IP address.
	if ip4 := ip.To4(); ip4 != nil {
		bootstrapHub.Info.IPv4 = ip4
	} else {
		bootstrapHub.Info.IPv6 = ip
	}

	return bootstrapHub, nil
}
