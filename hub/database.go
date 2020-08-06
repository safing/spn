package hub

import (
	"errors"
	"fmt"
	"sync"
	"time"

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

	// RawMsgsScope is for storing raw msgs. The path spec for this scope is cache:spn/rawMsgs/<scope>/<msgType>/<ID>
	RawMsgsScope = "cache:spn/rawMsgs/"

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
func GetHub(scope Scope, id string) (*Hub, error) {
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
		return checkAndReturn(new), nil
	}

	// or adjust type
	new, ok := r.(*Hub)
	if !ok {
		return nil, fmt.Errorf("record not of type *Hub, but %T", r)
	}

	// ensure status
	return checkAndReturn(new), nil
}

func checkAndReturn(h *Hub) *Hub {
	if h.Status == nil {
		h.Status = &HubStatus{}
	}
	return h
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
func RemoveHub(scope Scope, id string) error {
	key, ok := makeHubDBKey(scope, id)
	if !ok {
		return errors.New("invalid scope")
	}
	return db.Delete(key)
}

func makeHubDBKey(scope Scope, id string) (key string, ok bool) {
	switch scope {
	case ScopeLocal:
		return LocalHubs + id, true
	case ScopePublic:
		return PublicHubs + id, true
	case ScopeTest:
		return AllHubs + "test/" + id, true
	default:
		return "", false
	}
}

// HubMsg stores raw Hub messages.
type HubMsg struct {
	record.Base
	sync.Mutex

	ID    string
	Scope Scope
	Type  string
	Data  []byte

	Received int64
}

// SaveRawHubMsg saves a raw (and signed) message received by another Hub.
func SaveRawHubMsg(id string, scope Scope, msgType string, data []byte) error {
	// create wrapper record
	msg := &HubMsg{
		ID:       id,
		Scope:    scope,
		Type:     msgType,
		Data:     data,
		Received: time.Now().Unix(),
	}
	// set key
	msg.SetKey(fmt.Sprintf(
		"%s%s/%s/%s",
		RawMsgsScope,
		msg.Scope,
		msg.Type,
		msg.ID,
	))
	// save
	return db.PutNew(msg)
}

// EnsureHubMsg makes sure a database record is a HubMsg.
func EnsureHubMsg(r record.Record) (*HubMsg, error) {
	// unwrap
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &HubMsg{}
		err := record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		return new, nil
	}

	// or adjust type
	new, ok := r.(*HubMsg)
	if !ok {
		return nil, fmt.Errorf("record not of type *Hub, but %T", r)
	}
	return new, nil
}
