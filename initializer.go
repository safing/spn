package port17

import (
	"fmt"

	"github.com/Safing/safing-core/formats/dsd"
	"github.com/Safing/safing-core/formats/varint"
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/tinker"
)

// Initializer holds information for initializing different parts of the Port17 protocol.
type Initializer struct {
	portVersion  uint8  `json:"-", bson:"-"`
	LineID       uint32 `json:"l,omitempty", bson:"l,omitempty"`
	DestPortName string `json:"p,omitempty", bson:"p,omitempty"`

	TinkerVersion uint8    `json:"v,omitempty", bson:"v,omitempty"`
	TinkerTools   []string `json:"t,omitempty", bson:"t,omitempty"`
	KeyexIDs      []int    `json:"x,omitempty", bson:"x,omitempty"`
}

// NewInitializer creates a new Initializer with meta-information pre-filled. All of the information for the protocol must still be filled by the caller.
func NewInitializer() *Initializer {
	return &Initializer{
		portVersion:   1,
		TinkerVersion: 1,
		TinkerTools:   tinker.RecommendedNetwork,
	}
}

func NewInitializerFromBottle(destBottle *bottle.Bottle) (*Initializer, error) {
	init := NewInitializer()
	keyID, _ := destBottle.GetValidKey()
	if keyID < 0 {
		return nil, fmt.Errorf("destination bottle (%s) has not valid keys", destBottle.PortName)
	}
	init.KeyexIDs = []int{keyID}
	return init, nil
}

// UnpackInitializer unpacks a Initializer structure received over the network.
func UnpackInitializer(data []byte) (*Initializer, error) {
	portVersion, n, err := varint.Unpack8(data)
	if err != nil {
		return nil, err
	}

	if portVersion != 1 {
		return nil, fmt.Errorf("unsupported port17 version: %d", portVersion)
	}

	loaded, err := dsd.Load(data[n:], &Initializer{})
	if err != nil {
		return nil, fmt.Errorf("data structure mismatch")
	}

	init, ok := loaded.(*Initializer)
	if !ok {
		return nil, fmt.Errorf("data structure mismatch")
	}
	init.portVersion = portVersion

	return init, nil
}

// Pack packs an Initializer for transmission over the network.
func (init *Initializer) Pack() ([]byte, error) {
	data, err := dsd.Dump(init, dsd.JSON)
	if err != nil {
		return nil, err
	}
	data = append(varint.Pack8(init.portVersion), data...)
	return data, nil
}
