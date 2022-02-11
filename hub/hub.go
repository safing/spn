package hub

import (
	"net"
	"sync"
	"time"

	"github.com/mitchellh/copystructure"

	"github.com/safing/jess"
	"github.com/safing/portbase/database/record"
)

// Scope is the network scope a Hub can be in.
type Scope uint8

const (
	// ScopeInvalid defines an invalid scope.
	ScopeInvalid Scope = 0

	// ScopeLocal identifies local Hubs.
	ScopeLocal Scope = 1

	// ScopePublic identifies public Hubs.
	ScopePublic Scope = 2

	// ScopeTest identifies Hubs for testing.
	ScopeTest Scope = 0xFF
)

const (
	obsoleteValidAfter   = 30 * 24 * time.Hour
	obsoleteInvalidAfter = 7 * 24 * time.Hour
)

// MsgType defines the message type.
type MsgType string

// Message Types.
const (
	MsgTypeAnnouncement = "announcement"
	MsgTypeStatus       = "status"
)

// Hub represents a network node in the SPN.
type Hub struct { //nolint:maligned
	sync.Mutex
	record.Base

	ID        string
	PublicKey *jess.Signet
	Map       string

	Info   *Announcement
	Status *Status

	Measurements            *Measurements
	measurementsInitialized bool

	FirstSeen     time.Time
	VerifiedIPs   bool
	InvalidInfo   bool
	InvalidStatus bool
}

// Announcement is the main message type to publish Hub Information. This only changes if updated manually.
type Announcement struct {

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

// Copy returns a deep copy of the Announcement.
func (a *Announcement) Copy() (*Announcement, error) {
	// TODO: Improve this.
	copied, err := copystructure.Copy(a)
	if err != nil {
		return nil, err
	}
	return copied.(*Announcement), nil //nolint:forcetypeassert // Can only be an *Announcement.
}

// GetInfo returns the hub info.
func (h *Hub) GetInfo() *Announcement {
	h.Lock()
	defer h.Unlock()

	return h.Info
}

// GetStatus returns the hub status.
func (h *Hub) GetStatus() *Status {
	h.Lock()
	defer h.Unlock()

	return h.Status
}

// GetMeasurements returns the hub measurements.
// This method should always be used instead of direct access.
func (h *Hub) GetMeasurements() *Measurements {
	h.Lock()
	defer h.Unlock()

	return h.GetMeasurementsWithLockedHub()
}

// GetMeasurementsWithLockedHub returns the hub measurements.
// The caller must hold the lock to Hub.
// This method should always be used instead of direct access.
func (h *Hub) GetMeasurementsWithLockedHub() *Measurements {
	if !h.measurementsInitialized {
		h.Measurements = getSharedMeasurements(h.ID, h.Measurements)
		h.Measurements.check()
		h.measurementsInitialized = true
	}

	return h.Measurements
}

// Verified return whether the Hub has been verified.
func (h *Hub) Verified() bool {
	h.Lock()
	defer h.Unlock()

	return h.VerifiedIPs
}

// String returns a human-readable representation of the Hub.
func (h *Hub) String() string {
	h.Lock()
	defer h.Unlock()

	return "<Hub " + h.getName() + ">"
}

// StringWithoutLocking returns a human-readable representation of the Hub without locking it.
func (h *Hub) StringWithoutLocking() string {
	return "<Hub " + h.getName() + ">"
}

// Name returns a human-readable version of a Hub's name. This name will likely consist of two parts: the given name and the ending of the ID to make it unique.
func (h *Hub) Name() string {
	h.Lock()
	defer h.Unlock()

	return h.getName()
}

func (h *Hub) getName() string {
	// Check for a short ID that is sometimes used for testing.
	if len(h.ID) < 8 {
		return h.ID
	}

	shortenedID := h.ID[len(h.ID)-8:len(h.ID)-4] +
		"-" +
		h.ID[len(h.ID)-4:]

	// Be more careful, as the Hub name is user input.
	switch {
	case h.Info.Name == "":
		return shortenedID
	case len(h.Info.Name) > 16:
		return h.Info.Name[:16] + " " + shortenedID
	default:
		return h.Info.Name + " " + shortenedID
	}
}

// Obsolete returns if the Hub is obsolete and may be deleted.
func (h *Hub) Obsolete() bool {
	h.Lock()
	defer h.Unlock()

	// Check if Hub is valid.
	var valid bool
	switch {
	case h.InvalidInfo:
	case h.InvalidStatus:
	case h.Status.Version == VersionOffline:
		// Treat offline as invalid.
	default:
		valid = true
	}

	// Check when Hub was last seen.
	lastSeen := h.FirstSeen
	if h.Status.Timestamp != 0 {
		lastSeen = time.Unix(h.Status.Timestamp, 0)
	}

	// Check if Hub is obsolete.
	if valid {
		return time.Now().Add(-obsoleteValidAfter).After(lastSeen)
	}
	return time.Now().Add(-obsoleteInvalidAfter).After(lastSeen)
}

// Equal returns whether the given Announcements are equal.
func (a *Announcement) Equal(b *Announcement) bool {
	switch {
	case a == nil || b == nil:
		return false
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

// validateFormatting check if all values conform to the basic format.
func (a *Announcement) validateFormatting() (err error) {
	if err = checkStringFormat("ID", a.ID, 255); err != nil {
		return err
	}
	if err = checkStringFormat("Name", a.Name, 32); err != nil {
		return err
	}
	if err = checkStringFormat("Group", a.Group, 32); err != nil {
		return err
	}
	if err = checkStringFormat("ContactAddress", a.ContactAddress, 255); err != nil {
		return err
	}
	if err = checkStringFormat("ContactService", a.ContactService, 255); err != nil {
		return err
	}
	if err = checkStringSliceFormat("Hosters", a.Hosters, 255, 255); err != nil {
		return err
	}
	if err = checkStringFormat("Datacenter", a.Datacenter, 255); err != nil {
		return err
	}
	if err = checkIPFormat("IPv4", a.IPv4); err != nil {
		return err
	}
	if err = checkIPFormat("IPv6", a.IPv6); err != nil {
		return err
	}
	if err = checkStringSliceFormat("Transports", a.Transports, 255, 255); err != nil {
		return err
	}
	if err = checkStringSliceFormat("Entry", a.Entry, 255, 255); err != nil {
		return err
	}
	if err = checkStringSliceFormat("Exit", a.Exit, 255, 255); err != nil {
		return err
	}
	return nil
}

// String returns the string representation of the scope.
func (s Scope) String() string {
	switch s {
	case ScopeInvalid:
		return "invalid"
	case ScopeLocal:
		return "local"
	case ScopePublic:
		return "public"
	case ScopeTest:
		return "test"
	default:
		return "unknown"
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
