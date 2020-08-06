package cabin

import (
	"errors"
	"fmt"

	"github.com/safing/spn/hub"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/record"
	"github.com/safing/portbase/log"
)

var (
	db = database.NewInterface(nil)
)

// LoadIdentity loads an identify with the given key.
func LoadIdentity(key string) (*Identity, error) {
	r, err := db.Get(key)
	if err != nil {
		return nil, err
	}
	id, err := EnsureIdentity(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identity: %w", err)
	}

	// load Hub
	h, err := hub.GetHub(id.Scope, id.ID)
	if err != nil {
		log.Warningf("spn/cabin: re-initializing hub for identity %s", id.ID)
		recipient, err := id.Signet.AsRecipient()
		if err != nil {
			return nil, fmt.Errorf("failed to get recipient from identity signet: %w", err)
		}
		id.initializeIdentityHub(recipient)
	} else {
		id.hub = h
	}

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

// EnsureIdentity makes sure a database record is an Identity.
func EnsureIdentity(r record.Record) (*Identity, error) {
	// unwrap
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &Identity{}
		err := record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		return new.loadIdentityHub()
	}

	// or adjust type
	new, ok := r.(*Identity)
	if !ok {
		return nil, fmt.Errorf("record not of type *Identity, but %T", r)
	}
	return new.loadIdentityHub()
}

func (id *Identity) loadIdentityHub() (*Identity, error) {
	h, err := hub.GetHub(id.Scope, id.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity Hub: %w", err)
	}

	id.hub = h
	return id, nil
}

// Save saves the Identity to the database.
func (id *Identity) Save() error {
	if !id.KeyIsSet() {
		return errors.New("no key set")
	}

	return db.Put(id)
}
