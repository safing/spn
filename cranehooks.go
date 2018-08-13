package port17

import (
	"bytes"
	"errors"

	"github.com/Safing/safing-core/container"
	"github.com/Safing/safing-core/crypto/random"
	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/port17/bottlerack"
	"github.com/Safing/safing-core/port17/identity"

	"github.com/tevino/abool"
)

type craneHookFunc func(controller *CraneController, bottle *bottle.Bottle, exportedBottle []byte) error

var (
	updateBottleHook   craneHookFunc
	distrustBottleHook craneHookFunc
	publishChanHook    craneHookFunc
	hooksActive        = abool.New()
)

// RegisterCraneHooks allows the manager to hook into important crane functions without creating an import loop
func RegisterCraneHooks(update, distrust, publishChan craneHookFunc) {
	if update != nil && distrust != nil && publishChan != nil {
		updateBottleHook = update
		distrustBottleHook = distrust
		publishChanHook = publishChan
		hooksActive.Set()
	}
}

func (cControl *CraneController) UpdateBottle(packedBottle []byte) {
	// if len(packedBottle) > 0 {
	cControl.send <- container.NewContainer([]byte{CraneMsgTypeUpdateBottle}, packedBottle)
	// }
}

func (cControl *CraneController) handleUpdateBottle(c *container.Container) error {
	b, err := bottle.LoadUntrustedBottle(c.CompileData())
	if err != nil {
		return err
	}
	if hooksActive.IsSet() {
		return updateBottleHook(cControl, b, c.CompileData())
	}
	return nil
}

func (cControl *CraneController) DistrustBottle(packedBottle []byte) {
	cControl.send <- container.NewContainer([]byte{CraneMsgTypeDistrustBottle}, packedBottle)
}

func (cControl *CraneController) handleDistrustBottle(c *container.Container) error {
	b, err := bottle.LoadUntrustedBottle(c.CompileData())
	if err != nil {
		return err
	}
	if hooksActive.IsSet() {
		return distrustBottleHook(cControl, b, c.CompileData())
	}
	return nil
}

func (cControl *CraneController) PublishChannel() error {
	// 1) [client] request publishing of channel

	log.Tracef("port17: crane %s: request channel publish", cControl.Crane.ID)

	// set clientBottle
	cControl.Crane.clientBottle = identity.Get()
	if cControl.Crane.clientBottle == nil {
		return errors.New("could not get identity for channel publishing")
	}

	// get exportBottle
	exportBottle, err := identity.Export()
	if err != nil {
		return err
	}

	// send request
	new := container.New([]byte{CraneMsgTypePublishChannel})
	new.Append(exportBottle)
	cControl.send <- new

	// update status of crane
	cControl.Crane.status = CraneStatusPublishRequested

	return nil
}

func (cControl *CraneController) handlePublishChannel(c *container.Container) error {

	client := cControl.Crane.ship.IsMine()

	switch {
	case !client && cControl.Crane.status == CraneStatusPrivate:
		// 2) [server] update bottle and return challenge
		cControl.Crane.status = CraneStatusPublishRequested
		log.Tracef("port17: crane %s: channel publish requested, sending challenge", cControl.Crane.ID)

		// update bottle
		err := cControl.handleUpdateBottle(c)
		if err != nil {
			return err
		}

		// return challenge
		challenge, err := random.Bytes(32)
		if err != nil {
			return err
		}
		cControl.verificationChallenge = challenge
		cControl.send <- container.NewContainer([]byte{CraneMsgTypePublishChannel}, challenge)

	case client && cControl.Crane.status == CraneStatusPublishRequested:
		// 3) [client] complete challenge
		cControl.Crane.status = CraneStatusPublishVerifying
		log.Tracef("port17: crane %s: got channel publish challenge, responding", cControl.Crane.ID)

		new := container.New([]byte{CraneMsgTypePublishChannel})

		// Signed Data
		toSign := container.New()
		// To:
		toSign.AppendAsBlock([]byte(cControl.Crane.serverBottle.PortName))
		// Challenge:
		toSign.Append(c.CompileData())
		// Sign:
		// TODO: sign
		// tinker.NewTinker("SHA2-256", "RSA_PSS").NoConfidentiality().SupplyCertificate(cert)

		// Unsigned Data
		// From:
		new.AppendAsBlock([]byte(cControl.Crane.clientBottle.PortName))
		// Signed Container
		new.Append(toSign.CompileData())

		cControl.send <- new

	case !client && cControl.Crane.status == CraneStatusPublishRequested:
		// [server] signature: check + publish
		cControl.Crane.status = CraneStatusPublishVerifying
		log.Tracef("port17: crane %s: got response to channel publish challenge, verifying", cControl.Crane.ID)

		clientPortName, err := c.GetNextBlock()
		if err != nil {
			return err
		}

		// get bottle
		clientBottle := bottlerack.Get(string(clientPortName))
		if clientBottle == nil {
			return errors.New("could not get client bottle to verify channel-publish")
		}

		// TODO: verify signature
		// tinker.NewTinker("SHA2-256", "RSA_PSS").NoConfidentiality().SupplyCertificate(cert)

		serverPortName, err := c.GetNextBlock()
		if err != nil {
			return err
		}
		if string(serverPortName) != cControl.Crane.serverBottle.PortName {
			return errors.New("channel-publish verification error: PortName mismatch")
		}
		if !bytes.Equal(c.CompileData(), cControl.verificationChallenge) {
			return errors.New("channel-publish verification error: challenge mismatch")
		}

		cControl.send <- container.NewContainer([]byte{CraneMsgTypePublishChannel})

		log.Tracef("port17: crane %s: channel ready for publishing", cControl.Crane.ID)
		if hooksActive.IsSet() {
			cControl.Crane.status = CraneStatusPublished
			return publishChanHook(cControl, clientBottle, nil)
		}

		log.Tracef("port17: crane %s: could not notify manager of publishable channel", cControl.Crane.ID)
		return nil

	case client && cControl.Crane.status == CraneStatusPublishVerifying:
		// [client] publish
		log.Tracef("port17: crane %s: channel ready for publishing", cControl.Crane.ID)

		if hooksActive.IsSet() {
			cControl.Crane.status = CraneStatusPublished
			return publishChanHook(cControl, cControl.Crane.serverBottle, nil)
		}
		log.Tracef("port17: crane %s: could not notify manager of publishable channel", cControl.Crane.ID)
		return nil
	}

	return nil
}
