package hub

import (
	"net"
	"sync"
	"time"

	"github.com/safing/jess"
	"github.com/safing/portbase/database/record"
)

// Scope is the network scope a Hub can be in.
type Scope uint8

const (
	// ScopeInvalid defines an invalid scope
	ScopeInvalid Scope = 0

	// ScopeLocal identifies local Hubs
	ScopeLocal Scope = 1

	// ScopePublic identifies public Hubs
	ScopePublic Scope = 2
)

// Hub represents a network node in the SPN.
type Hub struct {
	sync.Mutex
	record.Base

	ID        string
	PublicKey *jess.Signet

	Scope  Scope
	Info   *HubAnnouncement
	Status *HubStatus

	// activeRoute

	FirstSeen time.Time
}

// HubAnnouncement is the main message type to publish Hub Information. This only changes if updated manually.
type HubAnnouncement struct {

	// Primary Key
	// hash of public key
	// must be checked if it matches the public key
	ID string // via jess.LabeledHash

	// PublicKey *jess.Signet
	// PublicKey // if not part of signature
	// Signature *jess.Letter
	Timestamp int64 // Unix timestamp in seconds

	// Node Information
	Name           string // name of the node
	Group          string // person or organisation, who is in control of the node (should be same for all nodes of this person or organisation)
	ContactAddress string // contact possibility  (recommended, but optional)
	ContactService string // type of service of the contact address, if not email

	// currently unused, but collected for later use
	Hosters    []string // hoster supply chain (reseller, hosting provider, datacenter operator, ...)
	Datacenter string   // datacenter will be bullshit checked
	// Format: CC-COMPANY-INTERNALCODE
	// Eg: DE-Hetzner-FSN1-DC5

	// Network Location and Access
	// If node is behind NAT (or similar), IP addresses must be configured
	IPv4       net.IP // must be global and accessible
	IPv6       net.IP // must be global and accessible
	Transports []string
	// {
	//   "spn:17",
	//   "smtp:25", // also support "smtp://:25
	//   "smtp:587",
	//   "imap:143",
	//   "http:80",
	//   "http://example.com:80", // HTTP (based): use full path for request
	//   "https:443",
	//   "ws:80",
	//   "wss://example.com:443/spn",
	// } // protocols with metadata

	// Policies - default permit
	Entry []string
	// {"+ ", "- *"}
	Exit []string
	// {"- * TCP/25", "- US"}
}

// String returns a human-readable representation of a Hub.
func (h *Hub) String() string {
	return "<Hub " + h.ID + ">"
}

// Equal returns whether the given Announcements are equal.
func (a *HubAnnouncement) Equal(b *HubAnnouncement) bool {
	switch {
	case a.ID != b.ID:
		return false
	case a.Timestamp != b.Timestamp:
		return false
	case a.Name != b.Name:
		return false
	case a.ContactAddress != b.ContactAddress:
		return false
	case a.ContactService != b.ContactService:
		return false
	case !equalStringSlice(a.Hosters, b.Hosters):
		return false
	case a.Datacenter != b.Datacenter:
		return false
	case !a.IPv4.Equal(b.IPv4):
		return false
	case !a.IPv6.Equal(b.IPv6):
		return false
	case !equalStringSlice(a.Transports, b.Transports):
		return false
	case !equalStringSlice(a.Entry, b.Entry):
		return false
	case !equalStringSlice(a.Exit, b.Exit):
		return false
	default:
		return true
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
