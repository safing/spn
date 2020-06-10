package hub

import (
	"net"
	"sync"

	"github.com/safing/jess"
	"github.com/safing/portbase/database/record"
)

const (
	// ScopeLocal identifies local Hubs
	ScopeLocal = 1

	// ScopePublic identifies public Hubs
	ScopePublic = 2
)

// Hub represents a network node in the SPN.
type Hub struct {
	sync.Mutex
	record.Base

	ID        string
	PublicKey *jess.Signet

	Scope  uint8
	Info   *HubAnnouncement
	Status *HubStatus

	// activeRoute

	FirstSeen int64
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
	Timestamp int64

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
