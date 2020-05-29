package bottle

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/safing/portbase/database"

	"github.com/safing/portbase/database/record"

	"github.com/safing/portbase/log"
	"github.com/safing/tinker"
)

var (
	myBottle        *Bottle
	lastBottleCheck time.Time

	AllBottles    = "cache:spn/bottles/"
	PublicBottles = AllBottles + "public/"
	LocalBottles  = AllBottles + "local/"

	db = database.NewInterface(nil)
)

type Bottle struct {
	record.Base
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

func GetPublicBottle(name string) (*Bottle, error) {
	return Get(PublicBottles + name)
}

func GetLocalBottle(name string) (*Bottle, error) {
	return Get(LocalBottles + name)
}

func EnsureBottle(r record.Record) (*Bottle, error) {
	// unwrap
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &Bottle{}
		err := record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		return new, nil
	}

	// or adjust type
	new, ok := r.(*Bottle)
	if !ok {
		return nil, fmt.Errorf("record not of type *Bottle, but %T", r)
	}
	return new, nil
}

func Get(key string) (*Bottle, error) {
	r, err := db.Get(key)
	if err != nil {
		if err == database.ErrNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get bottle from database: %s", err)
	}

	return EnsureBottle(r)
}

func DeletePublicBottle(name string) error {
	return db.Delete(PublicBottles + name)
}

func (b *Bottle) Save() error {
	if !b.KeyIsSet() {
		// FIXME: local bottles?
		b.SetKey(PublicBottles + b.PortName)
	}

	return db.Put(b)
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
