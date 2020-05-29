package bottle

import (
	"bytes"
	"time"

	ds "github.com/ipfs/go-datastore"

	"github.com/safing/portbase/log"
)

// ComparePublicBottle compares a public bottle in the bottlerack and returns if it should be handled further.
func ComparePublicBottle(newBottle *Bottle) (okAndContinue bool, storedBottle *Bottle) {

	// log.Tracef("bottlerack: comparing public bottle %s", newBottle.PortName)

	storedBottle, err := GetPublicBottle(newBottle.PortName)
	if err != nil {
		// save if not found -> new
		if err == ds.ErrNotFound {
			log.Infof("bottlerack: received new public bottle %s", newBottle.PortName)
			newBottle.FirstSeen = time.Now().Unix()
			return true, nil
		}
		// else warn
		log.Warningf("port17/bottlerack: could not load bottle with name \"%s\": %s", newBottle.PortName, err)
		return false, nil
	}

	if !bytes.Equal(storedBottle.PortID, newBottle.PortID) {
		log.Warningf("port17/bottlerack: bottle with ID \"%x\" tried to snatch name \"%s\"", newBottle.PortID, newBottle.PortName)
		return false, nil
	}

	// check for changes
	if !storedBottle.Equal(newBottle) {
		// log.Tracef("port17/bottlerack: bottles not equal: %s != %s", storedBottle, newBottle)
		return true, storedBottle
	}

	return false, nil

}

// UpdateLocalBottle updates a local bottle in the bottlerack and returns if it should be handled further.
func UpdateLocalBottle(newBottle *Bottle) (okAndContinue bool) {
	log.Tracef("bottlerack: updating local bottle %s (not yet)", newBottle.PortName)
	return false
}
