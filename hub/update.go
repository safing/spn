package hub

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/network/netutils"

	"github.com/safing/jess"
	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/database"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/spn/conf"
)

var (
	updateLock sync.Mutex

	// hubMsgRequirements defines which security attributes message need to have.
	hubMsgRequirements = jess.NewRequirements().
				Remove(jess.RecipientAuthentication). // Recipient don't need a private key.
				Remove(jess.Confidentiality).         // Message contents are out in the open.
				Remove(jess.Integrity)                // Only applies to decryption.
	// SenderAuthentication provides pre-decryption integrity. That is all we need.

	validateHubIP func(hub *Hub, ip net.IP) error

	clockSkewTolerance = 1 * time.Hour
)

// SetHubIPValidationFn sets the function that is used to validate the IP of a Hub.
func SetHubIPValidationFn(fn func(hub *Hub, ip net.IP) error) {
	if validateHubIP == nil {
		validateHubIP = fn
	}
}

// SignHubMsg signs the given serialized hub msg with the given configuration.
func SignHubMsg(msg []byte, env *jess.Envelope, enableTofu bool) ([]byte, error) {
	// start session from envelope
	session, err := env.Correspondence(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to signing session: %s", err)
	}
	// sign the data
	letter, err := session.Close(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign msg: %s", err)
	}

	if enableTofu {
		// smuggle the public key
		// letter.Keys is usually only used for key exchanges and encapsulation
		// neither is used when signing, so we can use letter.Keys to transport public keys
		for _, sender := range env.Senders {
			// get public key
			public, err := sender.AsRecipient()
			if err != nil {
				return nil, fmt.Errorf("failed to get public key of %s: %s", sender.ID, err)
			}
			// serialize key
			err = public.StoreKey()
			if err != nil {
				return nil, fmt.Errorf("failed to serialize public key %s: %s", sender.ID, err)
			}
			// add to keys
			letter.Keys = append(letter.Keys, &jess.Seal{
				Value: public.Key,
			})
		}
	}

	// pack
	data, err := letter.ToDSD(dsd.JSON)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// OpenHubMsg opens a signed hub msg and verifies the signature using the local database. If TOFU is enabled, the signature is always accepted, if valid.
func OpenHubMsg(data []byte, scope Scope, tofu bool) (msg []byte, sendingHub *Hub, err error) {
	letter, err := jess.LetterFromDSD(data)
	if err != nil {
		return nil, nil, fmt.Errorf("malformed letter: %s", err)
	}

	// check signatures
	var seal *jess.Seal
	switch len(letter.Signatures) {
	case 0:
		return nil, nil, errors.New("missing signature")
	case 1:
		seal = letter.Signatures[0]
	default:
		return nil, nil, fmt.Errorf("too many signatures (%d)", len(letter.Signatures))
	}

	// check signature signer ID
	if seal.ID == "" {
		return nil, nil, errors.New("signature is missing signer ID")
	}

	// get hub for public key
	hub, err := GetHub(scope, seal.ID)
	if err != nil {
		if err != database.ErrNotFound {
			return nil, nil, fmt.Errorf("failed to get existing hub: %s", err)
		}
		hub = nil
	}

	var truststore jess.TrustStore
	if hub != nil {
		// FIXME: check seal against hub
		// check ID integrity
		if !verifyHubID(seal.ID, hub.PublicKey.Scheme, hub.PublicKey.Key) {
			return nil, nil, errors.New("ID integrity violated with existing key")
		}
	} else {
		if !tofu {
			return nil, nil, errors.New("hub msg sender unknown")
		}

		// trust on first use, extract key from keys
		// FIXME: testing if works without

		// get key
		var pubkey *jess.Seal
		switch len(letter.Keys) {
		case 0:
			return nil, nil, errors.New("missing key for TOFU")
		case 1:
			pubkey = letter.Keys[0]
		default:
			return nil, nil, fmt.Errorf("too many keys for TOFU (%d)", len(letter.Keys))
		}

		// check ID integrity
		if !verifyHubID(seal.ID, seal.Scheme, pubkey.Value) {
			return nil, nil, errors.New("ID integrity violated with new key")
		}

		hub = &Hub{
			ID:    seal.ID,
			Scope: scope,
			PublicKey: &jess.Signet{
				ID:     seal.ID,
				Scheme: seal.Scheme,
				Key:    pubkey.Value,
				Public: true,
			},
		}
		err = hub.PublicKey.LoadKey()
		if err != nil {
			return nil, nil, err
		}
	}

	// create trust store
	truststore = &SingleTrustStore{hub.PublicKey}

	// remove keys from letter, as they are only used to transfer the public key
	letter.Keys = nil

	// check signature
	err = letter.Verify(hubMsgRequirements, truststore)
	if err != nil {
		return nil, nil, err
	}

	return letter.Data, hub, nil
}

// Export exports the announcement with the given signature configuration.
func (ha *HubAnnouncement) Export(env *jess.Envelope) ([]byte, error) {
	// pack
	msg, err := dsd.Dump(ha, dsd.JSON)
	if err != nil {
		return nil, fmt.Errorf("failed to pack announcement: %s", err)
	}

	return SignHubMsg(msg, env, true)
}

// ImportAnnouncement imports an announcement if it passes all the checks.
func ImportAnnouncement(data []byte, scope Scope) error {
	updateLock.Lock()
	defer updateLock.Unlock()

	// open and verify
	msg, hub, err := OpenHubMsg(data, scope, true)
	if err != nil {
		return err
	}

	// parse
	announcement := &HubAnnouncement{}
	_, err = dsd.Load(msg, announcement)
	if err != nil {
		return err
	}

	// integrity check

	// `hub.ID` is taken from the first ever received announcement message.
	// `announcement.ID` is additionally present in the message as we need
	// a signed version of the ID to mitigate fake IDs.
	// Fake IDs are possible because the hash algorithm of the ID is dynamic.
	if hub.ID != announcement.ID {
		return fmt.Errorf("announcement ID (%s) mismatches hub ID (%s)", announcement.ID, hub.ID)
	}

	// check timestamp
	if announcement.Timestamp > time.Now().Add(clockSkewTolerance).Unix() {
		return fmt.Errorf(
			"announcement from %s is from the future: %s",
			announcement.ID,
			time.Unix(announcement.Timestamp, 0),
		)
	}

	// version check
	if hub.Info != nil {
		// check if we already have this version
		switch {
		case announcement.Timestamp == hub.Info.Timestamp:
			// The new copy is not saved, as we expect the versions to be identical.
			// Also, the new version has not been validated at this point.
			return errors.New("version already imported")
		case announcement.Timestamp < hub.Info.Timestamp:
			// Received an old version, do not update.
			return errors.New("newer version present")
		}
	}

	// validation

	// validate IP changes
	if hub.Info != nil {
		switch {
		case hub.Info.IPv4 != nil && announcement.IPv4 == nil:
			return errors.New("previously announced IPv4 address missing")
		case hub.Info.IPv4 != nil && !announcement.IPv4.Equal(hub.Info.IPv4):
			return errors.New("IPv4 address changed")
		case hub.Info.IPv6 != nil && announcement.IPv6 == nil:
			return errors.New("previously announced IPv6 address missing")
		case hub.Info.IPv6 != nil && !announcement.IPv6.Equal(hub.Info.IPv6):
			return errors.New("IPv6 address changed")
		}
	}

	// validate IP scopes
	if announcement.IPv4 != nil {
		classification := netutils.ClassifyIP(announcement.IPv4)
		switch {
		case scope == ScopeLocal && classification != netutils.LinkLocal && classification != netutils.SiteLocal:
			return errors.New("IPv4 scope violation: outside of local scope")
		case scope == ScopePublic && classification != netutils.Global:
			return errors.New("IPv4 scope violation: outside of global scope")
		}
	}
	if announcement.IPv6 != nil {
		classification := netutils.ClassifyIP(announcement.IPv6)
		switch {
		case scope == ScopeLocal && classification != netutils.LinkLocal && classification != netutils.SiteLocal:
			return errors.New("IPv6 scope violation: outside of local scope")
		case scope == ScopePublic && classification != netutils.Global:
			return errors.New("IPv6 scope violation: outside of global scope")
		}
	}

	// validate transports
	invalidTransports := 0
	for _, definition := range announcement.Transports {
		_, err := ParseTransport(definition)
		if err != nil {
			invalidTransports++
			log.Warningf("spn/hub: invalid transport in announcement from %s: %s", hub.ID, definition)
		}
	}
	if invalidTransports >= len(announcement.Transports) {
		return errors.New("no valid transports present")
	}

	// validate IP ownership if public hub
	if conf.PublicHub() {
		if validateHubIP == nil {
			return errors.New("IP address validation not configured")
		}
		if announcement.IPv4 != nil {
			err := validateHubIP(hub, announcement.IPv4)
			if err != nil {
				return fmt.Errorf("%w: failed to validate IPv4 of %s: %s", ErrTemporaryValidationError, hub, err)
			}
		}
		if announcement.IPv6 != nil {
			err := validateHubIP(hub, announcement.IPv6)
			if err != nil {
				return fmt.Errorf("%w: failed to validate IPv6 of %s: %s", ErrTemporaryValidationError, hub, err)
			}
		}
	}

	// save to database

	hub.Lock()
	hub.Info = announcement

	if hub.FirstSeen.IsZero() {
		hub.FirstSeen = time.Now().UTC()
	}
	hub.Unlock()

	return hub.Save()
}

// Export exports the status with the given signature configuration.
func (hs *HubStatus) Export(env *jess.Envelope) ([]byte, error) {
	// pack
	msg, err := dsd.Dump(hs, dsd.JSON)
	if err != nil {
		return nil, fmt.Errorf("failed to pack status: %s", err)
	}

	return SignHubMsg(msg, env, false)
}

// ImportStatus imports a status update if it passes all the checks.
func ImportStatus(data []byte, scope Scope) error {
	updateLock.Lock()
	defer updateLock.Unlock()

	// open and verify
	msg, hub, err := OpenHubMsg(data, scope, false)
	if err != nil {
		return err
	}

	// parse
	status := &HubStatus{}
	_, err = dsd.Load(msg, status)
	if err != nil {
		return err
	}

	// integrity check

	// check timestamp
	if status.Timestamp > time.Now().Add(clockSkewTolerance).Unix() {
		return fmt.Errorf(
			"status from %s is from the future: %s",
			hub.ID,
			time.Unix(status.Timestamp, 0),
		)
	}

	// version check
	if hub.Status != nil {
		// check if we already have this version
		switch {
		case status.Timestamp == hub.Status.Timestamp:
			// The new copy is not saved, as we expect the versions to be identical.
			// Also, the new version has not been validated at this point.
			return errors.New("version already imported")
		case status.Timestamp < hub.Status.Timestamp:
			// Received an old version, do not update.
			return errors.New("newer version present")
		}
	}

	// validation

	// TODO: validate keys

	// save to database

	hub.Lock()
	hub.Status = status
	hub.Unlock()

	return hub.Save()
}

// CreateHubSignet creates a signet with the correct ID for usage as a Hub Identity.
func CreateHubSignet(toolID string, securityLevel int) (private, public *jess.Signet, err error) {
	private, err = jess.GenerateSignet(toolID, securityLevel)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate key: %s", err)
	}
	err = private.StoreKey()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to store private key: %s", err)
	}

	// get public key for creating the Hub ID
	public, err = private.AsRecipient()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public key: %s", err)
	}
	err = public.StoreKey()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to store public key: %s", err)
	}

	// assign IDs
	private.ID = createHubID(public.Scheme, public.Key)
	public.ID = private.ID

	return private, public, nil
}

func createHubID(scheme string, pubkey []byte) string {
	// compile scheme and public key
	c := container.New()
	c.AppendAsBlock([]byte(scheme))
	c.AppendAsBlock(pubkey)

	return lhash.Digest(lhash.BLAKE2b_256, c.CompileData()).String()
}

func verifyHubID(id string, scheme string, pubkey []byte) (ok bool) {
	// load labeled hash from ID
	labeledHash, err := lhash.LoadFromString(id)
	if err != nil {
		return false
	}

	// compile scheme and public key
	c := container.New()
	c.AppendAsBlock([]byte(scheme))
	c.AppendAsBlock(pubkey)

	// check if it matches
	return labeledHash.Matches(c.CompileData())
}
