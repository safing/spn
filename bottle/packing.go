package bottle

import (
	"errors"
	"time"

	"github.com/Safing/safing-core/formats/dsd"
)

// Pack packs a bottle.
func (b *Bottle) Pack() ([]byte, error) {
	b.Lock()
	defer b.Unlock()
	return dsd.Dump(b, dsd.JSON)
}

// Export packs and signs a public version of the bottle, ready for distribution
func (b *Bottle) Export(keysMinValid time.Time) ([]byte, error) {
	// locking is done by PublicWithMinValidity
	public := b.PublicWithMinValidity(keysMinValid)
	// TODO: sign
	return public.Pack()
}

// LoadUntrustedBottle loads an untrusted Bottle, does some checks and verifies the signature.
func LoadUntrustedBottle(data []byte) (*Bottle, error) {
	b, err := LoadTrustedBottle(data)
	if err != nil {
		return nil, err
	}

	// do some checks
	if b.Local != nil {
		if len(b.PortID) > 0 {
			return nil, errors.New("port17/bottle: failed to load local bottle, should not have PortID")
		}
		if len(b.Local.MaskedIdentifier) == 0 {
			return nil, errors.New("port17/bottle: failed to load local bottle, missing Local.MaskedIdentifier")
		}
		if len(b.Local.ReachableFrom) == 0 {
			return nil, errors.New("port17/bottle: failed to load local bottle, should not have Local.ReachableFrom")
		}
	}

	if len(b.Keys) == 0 {
		return nil, errors.New("port17/bottle: failed to load bottle, missing ephemeral keys")
	}

	// reset internal values
	b.FirstSeen = 0
	b.LastUpdate = 0

	// TODO: check signature

	return b, nil
}

// LoadTrustedBottle laods a Bottle without doing any checks.
func LoadTrustedBottle(data []byte) (*Bottle, error) {
	b := &Bottle{}
	_, err := dsd.Load(data, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}
