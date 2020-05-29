package identity

import (
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/utils"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/spn/bottle"
	"github.com/safing/tinker"
)

func manager() {
	time.Sleep(1 * time.Minute)
	for {
		// TODO: listen for config changes
		if ok, _ := maintainIdentity(); ok {
			time.Sleep(10 * time.Minute)
		} else {
			time.Sleep(1 * time.Second)
		}
	}
}

func maintainIdentity() (ok, changed bool) {
	identityLock.Lock()
	defer identityLock.Unlock()

	if identity == nil {
		return
	}
	ok = true

	identity.Lock()
	defer identity.Unlock()

	// TODO:
	// changed = checkNodeName(identity)
	changed = checkEphermalKeys(identity, time.Now())
	changed = checkAddresses(identity)

	if changed {
		go UpdateIdentity(identity)
	}

	return
}

func getFirstUnusedKeyID(keys map[int]*bottle.BottleKey) (i int) {
	i = 1
	for {
		_, ok := keys[i]
		if !ok {
			return
		}
		i++
	}
}

var (
	// valid for 36 hrs (1.5 days)
	validity = 36 * time.Hour
	// minimum hours left of validity for advertising
	minAdvertiseValidity = 12 * time.Hour

	// renew after 24 hrs (1 day)
	renewAfter = 24 * time.Hour
	// available for 48 hrs (2 days)
	burnAfter = 48 * time.Hour
	// reuse ID after 96 hrs (4 days)
	reuseAfter = 96 * time.Hour
)

func checkEphermalKeys(identity *bottle.Bottle, now time.Time) (changed bool) {

	// TODO: get key types and strength from config
	provideKeyTypes := utils.DuplicateStrings([]string{"ECDH-X25519"})
	provideKeyStrength := 0

	validUntil := now.Add(validity)
	renewThreshhold := validUntil.Add(-renewAfter).Unix()
	burnThreshhold := validUntil.Add(-burnAfter).Unix()
	idReuseThreshhold := validUntil.Add(-reuseAfter).Unix()

	// create Keys map
	if identity.Keys == nil {
		identity.Keys = make(map[int]*bottle.BottleKey)
	}

	// first check for valid keys
	for _, ephKey := range identity.Keys {
		if ephKey.Expires >= renewThreshhold {
			// we already provide that key type, remove
			provideKeyTypes = utils.RemoveFromStringSlice(provideKeyTypes, ephKey.Key.Algorithm)
		}
	}

	// handle existing keys: expiry, renewal
	for keyID, ephKey := range identity.Keys {
		switch {
		case ephKey.Expires < idReuseThreshhold:
			delete(identity.Keys, keyID)
		case ephKey.Expires < burnThreshhold:
			ephKey.Key = nil
		case ephKey.Expires < renewThreshhold:
			if utils.StringInSlice(provideKeyTypes, ephKey.Key.Algorithm) {
				// generate new key
				newKey, err := tinker.GenerateEphermalKey(ephKey.Key.Algorithm, provideKeyStrength)
				if err != nil {
					log.Errorf("port17/identity: failed to renew ephermal key of type %s: %s", ephKey.Key.Algorithm, err)
				} else {
					identity.Keys[getFirstUnusedKeyID(identity.Keys)] = &bottle.BottleKey{
						Key:     newKey,
						Expires: validUntil.Unix(),
					}
				}
				// delete from provideKeyTypes
				provideKeyTypes = utils.RemoveFromStringSlice(provideKeyTypes, ephKey.Key.Algorithm)
				changed = true
			}
		}
	}

	// create new keys
	for _, alg := range provideKeyTypes {
		newKey, err := tinker.GenerateEphermalKey(alg, provideKeyStrength)
		if err != nil {
			log.Errorf("port17/identity: failed to generate initial ephermal key of type %s: %s", alg, err)
		} else {
			identity.Keys[getFirstUnusedKeyID(identity.Keys)] = &bottle.BottleKey{
				Key:     newKey,
				Expires: validUntil.Unix(),
			}
			changed = true
		}
	}

	return
}

func checkAddresses(identity *bottle.Bottle) (changed bool) {

	// TODO: first check config
	// TODO: then check env

	// then try to autoconfigure
	// TODO: disable this via config
	v4IPs, v6IPs, err := netenv.GetAssignedGlobalAddresses()
	if err != nil {
		log.Warningf("port17/identity: failed to get addresses, aborting node IP autoconfig: %s", err)
		return
	}
	if len(v4IPs) == 0 && len(v6IPs) == 0 {
		log.Warningf("port17/identity: could not detect any global ip addresses on system, aborting node IP autoconfig")
		return
	}

	// first invalidate IPs if not found on system
	var foundIPv4 bool
	if identity.IPv4 != nil {
		for _, ip := range v4IPs {
			if ip.Equal(identity.IPv4) {
				foundIPv4 = true
				break
			}
		}
		if !foundIPv4 {
			log.Infof("port17/identity: removing unassigned IPv4 address (%s) from node identity", identity.IPv4)
			identity.IPv4 = nil
			changed = true
		}
	}

	var foundIPv6 bool
	if identity.IPv6 != nil {
		for _, ip := range v6IPs {
			if ip.Equal(identity.IPv6) {
				foundIPv6 = true
				break
			}
		}
		if !foundIPv6 {
			log.Infof("port17/identity: removing unassigned IPv6 address (%s) from node identity", identity.IPv6)
			identity.IPv6 = nil
			changed = true
		}
	}

	// then assign IP address, if only one is present
	if !foundIPv4 {
		switch len(v4IPs) {
		case 0:
			// no IP, ignore
		case 1:
			// one IP, assign!
			identity.IPv4 = v4IPs[0]
			changed = true
			log.Infof("port17/identity: selecting %s as IPv4 address for node identity", v4IPs[0])
		default:
			// more than one IP, what should we do?
			log.Warningf("port17/identity: found more than one viable IPv4 addresses, please configure IPv4 address manually. Found IPv4 addresses: %v", v4IPs)
		}
	}

	if !foundIPv6 {
		switch len(v6IPs) {
		case 0:
			// no IP, ignore
		case 1:
			// one IP, assign!
			identity.IPv6 = v6IPs[0]
			changed = true
			log.Infof("port17/identity: selecting %s as IPv6 address for node identity", v6IPs[0])
		default:
			// more than one IP, what should we do?
			log.Warningf("port17/identity: found more than one viable IPv6 addresses, please configure IPv6 address manually. Found IPv6 addresses: %v", v6IPs)
		}
	}

	if identity.IPv4 == nil && identity.IPv6 == nil {
		log.Errorf("port17/identity: unable to autoconfigure node identity IP addresses, please check logs")
	}

	return

}

func publishIdentity() {
	if hooksActive.IsSet() {
		publishHook()
	}
}
