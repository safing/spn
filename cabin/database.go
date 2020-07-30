package cabin

import (
	"fmt"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/record"
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

	return EnsureIdentity(r)
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
		return new, nil
	}

	// or adjust type
	new, ok := r.(*Identity)
	if !ok {
		return nil, fmt.Errorf("record not of type *Identity, but %T", r)
	}
	return new, nil
}

// SaveIdentity saves the identity to the database. If no key is set, the supplied is used.
func SaveIdentity(id *Identity, key string) error {
	if !id.KeyIsSet() {
		id.SetKey(key)
	}

	return db.Put(id)
}
