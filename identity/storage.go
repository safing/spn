package identity

import (
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/Safing/safing-core/crypto/random"
	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/meta"
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/port17/bottlerack"
	"github.com/Safing/safing-core/port17/mode"
)

var (
	identity       *bottle.Bottle
	publicIdentity *bottle.Bottle
	exportIdentity []byte
	identityLock   sync.Mutex

	dbNamespace = bottlerack.DatabaseNamespace.ChildString("Mine")
)

func init() {
	go func() {
		time.Sleep(1 * time.Second)
		if mode.Node() {
			defer func() {
				go manager()
			}()

			var err error
			identity, err = bottlerack.LoadBottle(dbNamespace.ChildString("nodeIdentity"))
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
	}()

	// go func() {
	// 	time.Sleep(5 * time.Second)
	// 	fmt.Println("===== TAKING TOO LONG FOR SHUTDOWN - PRINTING STACK TRACES =====")
	// 	pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	// 	os.Exit(1)
	// }()
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
	bottlerack.SaveBottle(dbNamespace.ChildString("nodeIdentity"), newIdentity)
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
	portName := meta.NodeName()
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
		id, err := random.Bytes(3)
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

	return new, nil
}
