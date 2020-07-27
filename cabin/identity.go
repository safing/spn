package cabin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/safing/jess/tools"

	"github.com/safing/jess"
	"github.com/safing/spn/hub"
)

const (
	// DefaultIDKeyScheme is the default jess tool for creating ID keys
	DefaultIDKeyScheme = "Ed25519"

	// DefaultIDKeySecurityLevel is the default security level for creating ID keys
	DefaultIDKeySecurityLevel = 256 // Ed25519 security level is fixed, setting is ignored
)

// Identity holds the identity of a Hub.
type Identity struct {
	Hub *hub.Hub
	Key *jess.Signet

	ExchKeys map[string]*ExchKey
}

// Lock locks the Identity through the Hub lock.
func (id *Identity) Lock() {
	id.Hub.Lock()
}

// Unlock unlocks the Identity through the Hub lock.
func (id *Identity) Unlock() {
	id.Hub.Unlock()
}

// ExchKey holds the private information of a HubKey.
type ExchKey struct {
	Created time.Time
	Expires time.Time
	key     *jess.Signet
	tool    *tools.Tool
}

// CreateIdentity creates a new identity.
func CreateIdentity(ctx context.Context, scope hub.Scope) (*Identity, error) {
	id := &Identity{
		Hub: &hub.Hub{
			Scope:     scope,
			Info:      &hub.HubAnnouncement{},
			Status:    &hub.HubStatus{},
			FirstSeen: time.Now().UTC(),
		},
		ExchKeys: make(map[string]*ExchKey),
	}

	// create signet
	signet, recipient, err := hub.CreateHubSignet(DefaultIDKeyScheme, DefaultIDKeySecurityLevel)
	if err != nil {
		return nil, err
	}
	id.Key = signet
	id.Hub.ID = signet.ID
	id.Hub.PublicKey = recipient

	// initial maintenance routine
	_, err = id.MaintainAnnouncement()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize announcement: %w", err)
	}
	_, err = id.MaintainStatus(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize status: %w", err)
	}

	return id, nil
}

// MaintainAnnouncement maintains the Hub's Announcenemt and returns whether there was a change that should be communicated to other Hubs.
func (id *Identity) MaintainAnnouncement() (changed bool, err error) {
	id.Lock()
	defer id.Unlock()

	// update hub information
	var newHubInfo *hub.HubAnnouncement

	switch id.Hub.Scope {
	case hub.ScopePublic:
		newHubInfo = getPublicHubInfo()
		newHubInfo.ID = id.Hub.ID
		newHubInfo.Timestamp = id.Hub.Info.Timestamp
	default:
		return false, nil
	}

	if newHubInfo.Equal(id.Hub.Info) {
		return false, nil
	}

	// update info and timestamp
	id.Hub.Info = newHubInfo
	id.Hub.Info.Timestamp = time.Now().Unix()
	return true, nil
}

// MaintainStatus maintains the Hub's Status and returns whether there was a change that should be communicated to other Hubs.
func (id *Identity) MaintainStatus(connections []*hub.HubConnection) (changed bool, err error) {
	id.Lock()
	defer id.Unlock()

	// update keys
	keysChanged, err := id.maintainExchKeys(time.Now())
	if err != nil {
		return false, fmt.Errorf("failed to maintain keys: %w", err)
	}
	if keysChanged {
		changed = true
	}

	// update connections
	if !hub.ConnectionsEqual(id.Hub.Status.Connections, connections) {
		id.Hub.Status.Connections = connections
		changed = true
	}

	// update timestamp
	if changed {
		id.Hub.Status.Timestamp = time.Now().Unix()
	}

	return changed, nil
}

// ExportAnnouncement serializes and signs the Announcement.
func (id *Identity) ExportAnnouncement() ([]byte, error) {
	return id.Hub.Info.Export(&jess.Envelope{
		SuiteID: jess.SuiteSignV1,
		Senders: []*jess.Signet{id.Key},
	})
}

// ExportStatus serializes and signs the Status.
func (id *Identity) ExportStatus() ([]byte, error) {
	return id.Hub.Status.Export(&jess.Envelope{
		SuiteID: jess.SuiteSignV1,
		Senders: []*jess.Signet{id.Key},
	})
}

func (ek *ExchKey) toHubKey() (*hub.HubKey, error) {
	if ek.key == nil {
		return nil, errors.New("no key")
	}

	// export public key
	rcpt, err := ek.key.AsRecipient()
	if err != nil {
		return nil, err
	}
	err = rcpt.StoreKey()
	if err != nil {
		return nil, err
	}

	// repackage
	return &hub.HubKey{
		Scheme:  rcpt.Scheme,
		Key:     rcpt.Key,
		Expires: ek.Expires.Unix(),
	}, nil
}
