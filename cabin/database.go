package cabin

import (
	"github.com/safing/portbase/database"
	"github.com/safing/spn/hub"
)

var (
	db = database.NewInterface(nil)
)

// LoadIdentity loads an identify with the given key.
func LoadIdentity(key string) (*hub.Hub, error) {
	r, err := db.Get(key)
	if err != nil {
		return nil, err
	}

	h, err := hub.EnsureHub(r)
	if err != nil {
		return nil, err
	}

	return h, nil
}

// SaveIdentity saves the identity to the database. If no key is set, the supplied is used.
func SaveIdentity(h *hub.Hub, key string) error {
	if !h.KeyIsSet() {
		h.SetKey(key)
	}

	return db.Put(h)
}
