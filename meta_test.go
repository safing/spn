package port17

import (
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/tinker"
)

func newPortIdentity(name string) (*bottle.Bottle, error) {
	// Keys
	ephKey, err := tinker.GenerateEphermalKey("ECDH-X25519", 0)
	if err != nil {
		return nil, err
	}

	return &bottle.Bottle{
		PortName: name,
		Keys: map[int]*bottle.BottleKey{
			1: &bottle.BottleKey{
				Key: ephKey,
			},
		},
	}, nil
}
