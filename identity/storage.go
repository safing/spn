package identity

import (
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/safing/portbase/rng"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/bottle"
	"github.com/safing/spn/mode"
)

var (
	identity       *bottle.Bottle
	publicIdentity *bottle.Bottle
	exportIdentity []byte
	identityLock   sync.Mutex

	identityKey = "core:spn/identity"
)

func initIdentity() {
	if mode.Node() {

		var err error
		identity, err = bottle.Get(identityKey)
		if err == nil {
			log.Infof("port17/identity: loaded identity: %s", identity)
			_, changed := maintainIdentity()
			if !changed {
				// also publish if not changed
				publishIdentity()
			}
			return
		}

		log.Warningf("port17/identity: failed to load identity: %s", err)
		log.Info("port17/identity: generating new identity...")

		new, err := NewIdentity()
		if err != nil {
			log.Warningf("port17/identity: failed to generate new identity: %s", err)
			return
		}
		UpdateIdentity(new)

	}
}

// UpdateIdentity updates an identity and saves it to the database.
func UpdateIdentity(newIdentity *bottle.Bottle) {
	identityLock.Lock()
	defer identityLock.Unlock()
	// identity.Lock()
	// defer identity.Unlock()

	identity = newIdentity
	publicIdentity = nil
	exportIdentity = nil
	log.Infof("port17/identity: updated ID: %s", newIdentity)
	err := identity.Save()
	if err != nil {
		log.Warningf("spn/identity: failed to save identity: %s", err)
	}
	go publishIdentity()
}

// GetIdentity returns the identity of the node.
func Get() *bottle.Bottle {
	identityLock.Lock()
	defer identityLock.Unlock()
	return identity
}

// Public returns the identity of the node with all private/sensitive information stripped.
func Public() *bottle.Bottle {
	identityLock.Lock()
	defer identityLock.Unlock()
	if identity == nil {
		return nil
	}
	if publicIdentity == nil {
		publicIdentity = identity.PublicWithMinValidity(time.Now().Add(minAdvertiseValidity))
	}
	return publicIdentity
}

// Export returns an already signed and packed version of the public identity.
func Export() ([]byte, error) {
	identityLock.Lock()
	defer identityLock.Unlock()
	if identity == nil {
		return nil, errors.New("identity not yet available, try later")
	}
	if len(exportIdentity) == 0 {
		var err error
		exportIdentity, err = identity.Export(time.Now().Add(minAdvertiseValidity))
		if err != nil {
			return nil, err
		}
	}
	return exportIdentity, nil
}

func NewIdentity() (*bottle.Bottle, error) {

	// Name
	// Specified?
	portName := nodeName
	// Hostname?
	if portName == "" {
		var err error
		portName, err = os.Hostname()
		if err != nil {
			log.Warningf("port17/identity: could not get hostname")
			portName = ""
		}
	}
	// Then random
	if portName == "" {
		id, err := rng.Bytes(3)
		if err != nil {
			log.Warningf("port17/identity: could not get hostname")
			portName = ""
		} else {
			portName = hex.EncodeToString(id)
		}
	}
	if portName == "" {
		return nil, errors.New("no node name specified, could not get hostname or generate random name")
	}

	new := &bottle.Bottle{
		PortName: portName,
	}
	checkEphermalKeys(new, time.Now())
	checkAddresses(new)

	new.SetKey(identityKey)
	return new, nil
}
