package hub

import (
	"errors"
	"fmt"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/record"
)

var (
	// AllHubs is the database scope for saving Hubs
	AllHubs = "cache:spn/hubs/"

	// LocalHubs is the database scope for local hubs
	LocalHubs = AllHubs + "local/"

	// PublicHubs is the database scope for public hubs
	PublicHubs = AllHubs + "public/"

	db = database.NewInterface(nil)

	getFromNavigator func(id string) *Hub
)

// SetNavigatorAccess sets a shortcut function to access hubs from the navigator instead of having go through the database.
// This also reduces the number of object in RAM and better caches parsed attributes.
func SetNavigatorAccess(fn func(id string) *Hub) {
	if getFromNavigator == nil {
		getFromNavigator = fn
	}
}

// GetHub get a Hub from the database - or the navigator, if configured.
func GetHub(scope uint8, id string) (*Hub, error) {
	if getFromNavigator != nil {
		hub := getFromNavigator(id)
		if hub != nil {
			return hub, nil
		}
	}

	key, ok := makeHubDBKey(scope, id)
	if !ok {
		return nil, errors.New("invalid scope")
	}

	r, err := db.Get(key)
	if err != nil {
		return nil, err
	}

	hub, err := EnsureHub(r)
	if err != nil {
		return nil, err
	}

	return hub, nil
}

// EnsureHub makes sure a database record is a Hub.
func EnsureHub(r record.Record) (*Hub, error) {
	// unwrap
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &Hub{}
		err := record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		return new, nil
	}

	// or adjust type
	new, ok := r.(*Hub)
	if !ok {
		return nil, fmt.Errorf("record not of type *Hub, but %T", r)
	}
	return new, nil
}

// Save saves to Hub to the correct scope in the database.
func (hub *Hub) Save() error {
	if !hub.KeyIsSet() {
		key, ok := makeHubDBKey(hub.Scope, hub.ID)
		if !ok {
			return errors.New("invalid scope")
		}
		hub.SetKey(key)
	}

	return db.Put(hub)
}

// RemoveHub deletes a Hub from the database.
func RemoveHub(scope uint8, id string) error {
	key, ok := makeHubDBKey(scope, id)
	if !ok {
		return errors.New("invalid scope")
	}
	return db.Delete(key)
}

func makeHubDBKey(scope uint8, id string) (key string, ok bool) {
	switch scope {
	case ScopeLocal:
		return LocalHubs + id, true
	case ScopePublic:
		return PublicHubs + id, true
	default:
		return "", false
	}
}
