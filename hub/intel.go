package hub

import (
	"fmt"

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
	var intel *Intel
	_, err := dsd.Load(data, intel)
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
