package bottle

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/tinker"
)

var (
	rack            = make(map[string]*Bottle)
	rackLock        sync.RWMutex
	myBottle        *Bottle
	lastBottleCheck time.Time
)

type Bottle struct {
	sync.Mutex

	PortID    []byte            `json:"i" bson:"i"` // public key
	Signature *tinker.Signature `json:"s" bson:"s"`

	// Additional Information
	Local *LocalBottle `json:"local,omitempty" bson:"local,omitempty"`

	// Node Information
	PortName       string `json:"n" bson:"n"`
	PortAssociaton string `json:"a" bson:"a"`

	// Network Location and Access
	IPv4      net.IP   `json:"ip4" bson:"ip4"`
	IPv6      net.IP   `json:"ip6" bson:"ip6"`
	ShipTypes []string `json:"t" bson:"t"`
	Ports     []int16  `json:"p" bson:"p"`

	// Routing Information
	Keys        map[int]*BottleKey `json:"k" bson:"k"`
	Connections []BottleConnection `json:"c,omitempty" bson:"c,omitempty"`
	Load        int                `json:"l" bson:"l"`

	FirstSeen  int64 `json:",omitempty" bson:",omitempty"`
	LastUpdate int64 `json:",omitempty" bson:",omitempty"`
}

func (b *Bottle) String() string {
	b.Lock()
	defer b.Unlock()

	var local string
	if b.Local != nil {
		local = "local "
	}
	return fmt.Sprintf("<Bottle %s @ %s%s %s routes=%v keys=%v>", b.PortName, local, b.IPv4, b.IPv6, b.Connections, b.Keys)
}

func (b *Bottle) Public() *Bottle {
	return b.PublicWithMinValidity(time.Now())
}

func (b *Bottle) PublicWithMinValidity(keysMinValid time.Time) *Bottle {
	b.Lock()
	defer b.Unlock()

	// make a copy
	new := &Bottle{
		PortID:         b.PortID,
		Signature:      b.Signature,
		PortName:       b.PortName,
		PortAssociaton: b.PortAssociaton,
		IPv4:           b.IPv4,
		IPv6:           b.IPv6,
		ShipTypes:      b.ShipTypes,
		Ports:          b.Ports,
		Keys:           make(map[int]*BottleKey),
		Connections:    b.Connections,
		Load:           b.Load,
	}

	// clean keys
	minValid := keysMinValid.Unix()
	for keyID, ephKey := range b.Keys {
		if ephKey.Key != nil && ephKey.Expires >= minValid {
			new.Keys[keyID] = ephKey.Public()
		}
	}

	return new
}

func (b *Bottle) AddConnection(newBottle *Bottle, cost int) {
	b.Lock()
	defer b.Unlock()

	for _, connection := range b.Connections {
		if newBottle.PortName == connection.PortName {
			log.Warningf("port17/bottle: tried to add already existing connection to bottle")
			return
		}
	}

	b.Connections = append(b.Connections, BottleConnection{
		PortName: newBottle.PortName,
		Cost:     cost,
	})
}

func (b *Bottle) RemoveConnection(portName string) {
	b.Lock()
	defer b.Unlock()

	for key, connection := range b.Connections {
		if connection.PortName == portName {
			b.Connections = append(b.Connections[:key], b.Connections[key+1:]...)
			break
		}
	}
}

func (b *Bottle) Equal(otherBottle *Bottle) bool {
	b.Lock()
	defer b.Unlock()
	otherBottle.Lock()
	defer otherBottle.Unlock()
	// TODO: check on more things here

	// ID
	if b.PortName != otherBottle.PortName {
		// log.Tracef("port17/bottle: equality: PortName mismatch")
		return false
	}
	if !bytes.Equal(b.PortID, otherBottle.PortID) {
		// log.Tracef("port17/bottle: equality: PortID mismatch")
		return false
	}

	// addresses
	if !b.IPv4.Equal(otherBottle.IPv4) {
		// log.Tracef("port17/bottle: equality: IPv4 mismatch")
		return false
	}
	if !b.IPv6.Equal(otherBottle.IPv6) {
		// log.Tracef("port17/bottle: equality: IPv6 mismatch")
		return false
	}

	// connections
	if len(b.Connections) != len(otherBottle.Connections) {
		// log.Tracef("port17/bottle: equality: connections length mismatch")
		return false
	}
	for i := 0; i < len(b.Connections); i++ {
		if b.Connections[i].PortName != otherBottle.Connections[i].PortName {
			// log.Tracef("port17/bottle: equality: connection PortName mismatch")
			return false
		}
		if b.Connections[i].Cost != otherBottle.Connections[i].Cost {
			// log.Tracef("port17/bottle: equality: connection Cost mismatch")
			return false
		}
	}

	// keys
	if len(b.Keys) != len(otherBottle.Keys) {
		// log.Tracef("port17/bottle: equality: keys length mismatch")
		return false
	}
	for keyID, bKey := range b.Keys {
		otherKey, ok := otherBottle.Keys[keyID]
		if !ok {
			// log.Tracef("port17/bottle: equality: key ID mismatch")
			return false
		}
		if !bytes.Equal(bKey.Key.PublicKey, otherKey.Key.PublicKey) {
			// log.Tracef("port17/bottle: equality: keys public key mismatch")
			return false
		}
	}

	// all same
	return true
}
