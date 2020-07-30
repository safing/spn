package cabin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/safing/portbase/database/record"

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
	record.Base

	Hub    *hub.Hub
	Signet *jess.Signet

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
	id.Signet = signet
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

func (id *Identity) signingEnvelope() *jess.Envelope {
	env := jess.NewUnconfiguredEnvelope()
	env.SuiteID = jess.SuiteSignV1
	env.Senders = []*jess.Signet{id.Signet}

	return env
}

// ExportAnnouncement serializes and signs the Announcement.
func (id *Identity) ExportAnnouncement() ([]byte, error) {
	id.Lock()
	defer id.Unlock()

	data, err := id.Hub.Info.Export(id.signingEnvelope())
	if err != nil {
		return nil, fmt.Errorf("failed to export: %w", err)
	}

	err = hub.ImportAnnouncement(data, id.Hub.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to pass import check: %w", err)
	}

	return data, nil
}

// ExportStatus serializes and signs the Status.
func (id *Identity) ExportStatus() ([]byte, error) {
	id.Lock()
	defer id.Unlock()

	data, err := id.Hub.Status.Export(id.signingEnvelope())
	if err != nil {
		return nil, fmt.Errorf("failed to export: %w", err)
	}

	err = hub.ImportStatus(data, id.Hub.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to pass import check: %w", err)
	}

	return data, nil
}

// SignHubMsg signs a data blob with the identity's private key.
func (id *Identity) SignHubMsg(data []byte) ([]byte, error) {
	return hub.SignHubMsg(data, id.signingEnvelope(), false)
}

// GetSignet returns the private exchange key with the given ID.
func (id *Identity) GetSignet(keyID string, recipient bool) (*jess.Signet, error) {
	if recipient {
		return nil, errors.New("cabin.Identity only serves private keys")
	}

	id.Lock()
	defer id.Unlock()

	key, ok := id.ExchKeys[keyID]
	if !ok {
		return nil, errors.New("the requested key does not exist")
	}
	if time.Now().After(key.Expires) || key.key == nil {
		return nil, errors.New("the requested key has expired")
	}

	return key.key, nil
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
