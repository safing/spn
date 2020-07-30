package docks

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/hub"

	"github.com/tevino/abool"
)

type craneHookFunc func(controller *CraneController, connectedHub *hub.Hub, c *container.Container) error

var (
	ErrInternalError = errors.New("internal error")

	hubAnnouncementHook   craneHookFunc
	hubStatusHook         craneHookFunc
	publishConnectionHook craneHookFunc
	hooksActive           = abool.New()
)

// RegisterCraneHooks allows the manager to hook into important crane functions without creating an import loop
func RegisterCraneHooks(announcement, status, publish craneHookFunc) {
	if announcement != nil && status != nil && publish != nil {
		hubAnnouncementHook = announcement
		hubStatusHook = status
		publishConnectionHook = publish
		hooksActive.Set()
	}
}

func (cControl *CraneController) SendHubAnnouncement(msg []byte) {
	cControl.send <- container.NewContainer([]byte{CraneMsgTypeHubAnnouncement}, msg)
}

func (cControl *CraneController) handleHubAnnouncement(c *container.Container) error {
	if hooksActive.IsSet() {
		return hubAnnouncementHook(cControl, cControl.Crane.connectedHub, c)
	}
	return ErrInternalError
}

func (cControl *CraneController) SendHubStatus(msg []byte) {
	cControl.send <- container.NewContainer([]byte{CraneMsgTypeHubStatus}, msg)
}

func (cControl *CraneController) handleHubStatus(c *container.Container) error {
	if hooksActive.IsSet() {
		return hubStatusHook(cControl, cControl.Crane.connectedHub, c)
	}
	return ErrInternalError
}

func (cControl *CraneController) PublishConnection() error {
	// 1) [client] request publishing of channel

	if !cControl.Crane.ship.IsMine() {
		return errors.New("can only initiate publish for own connections")
	}

	// send announcement to initiate
	data, err := cControl.Crane.identity.ExportAnnouncement()
	if err != nil {
		return fmt.Errorf("failed to export announcement: %w", err)
	}
	cControl.send <- container.New(
		[]byte{CraneMsgTypePublishConnection},
		data,
	)

	log.Tracef("spn/docks: crane %s: requesting connection publish", cControl.Crane.ID)

	// update status of crane
	cControl.Crane.status = CraneStatusPublishRequested

	return nil
}

func (cControl *CraneController) handlePublishConnection(c *container.Container) error {

	client := cControl.Crane.ship.IsMine()

	switch {
	case !client && cControl.Crane.status == CraneStatusPrivate:
		// 2) [server] update announcement and return challenge

		log.Tracef("spn/docks: crane %s: connection publish requested, sending challenge", cControl.Crane.ID)

		// update announcement
		err := cControl.handleHubAnnouncement(c)
		if err != nil {
			return err
		}

		// return challenge token
		challenge, err := rng.Bytes(32)
		if err != nil {
			return err
		}
		cControl.verificationChallenge = challenge
		cControl.send <- container.New([]byte{CraneMsgTypePublishConnection}, challenge)

		// update status of crane
		cControl.Crane.status = CraneStatusPublishRequested

	case client && cControl.Crane.status == CraneStatusPublishRequested:
		// 3) [client] complete challenge

		log.Tracef("spn/docks: crane %s: got connection publish challenge, responding", cControl.Crane.ID)

		// sign challenge
		response := container.New()
		// To:
		response.AppendAsBlock([]byte(cControl.Crane.connectedHub.ID))
		// From:
		response.AppendAsBlock([]byte(cControl.Crane.identity.Hub.ID))
		// Token:
		response.AppendContainerAsBlock(c)
		// sign
		signedResponse, err := cControl.Crane.identity.SignHubMsg(response.CompileData())
		if err != nil {
			return fmt.Errorf("failed to sign challenge response: %s", err)
		}

		cControl.send <- container.New([]byte{CraneMsgTypePublishConnection}, signedResponse)

		// update status of crane
		cControl.Crane.status = CraneStatusPublishVerifying

	case !client && cControl.Crane.status == CraneStatusPublishRequested:
		// 4) [server] challange verification + publish

		log.Tracef("spn/docks: crane %s: got response to connection publish challenge, verifying", cControl.Crane.ID)

		// verify message
		msgData, sendingHub, err := hub.OpenHubMsg(c.CompileData(), hub.ScopePublic, false)
		if err != nil {
			return fmt.Errorf("challenge response verification failed: %w", err)
		}
		msg := container.New(msgData)

		// To:
		to, err := msg.GetNextBlock()
		if err != nil {
			return fmt.Errorf("format error: %w", err)
		}
		if string(to) != cControl.Crane.identity.Hub.ID {
			return errors.New("challange response recipient mismatch")
		}
		// From:
		from, err := msg.GetNextBlock()
		if err != nil {
			return fmt.Errorf("format error: %w", err)
		}
		if string(from) != sendingHub.ID {
			return errors.New("challange response sender mismatch")
		}
		// Token:
		token, err := msg.GetNextBlock()
		if err != nil {
			return fmt.Errorf("format error: %w", err)
		}
		if !bytes.Equal(token, cControl.verificationChallenge) {
			return errors.New("challange response token mismatch")
		}

		// set connected hub
		cControl.Crane.connectedHub = sendingHub

		// send confirmation
		cControl.send <- container.NewContainer([]byte{CraneMsgTypePublishConnection})

		// update status of crane
		log.Tracef("spn/docks: crane %s is ready to publish connection", cControl.Crane.ID)
		cControl.Crane.status = CraneStatusPublished

		// publish
		if hooksActive.IsSet() {
			return publishConnectionHook(cControl, cControl.Crane.connectedHub, nil)
		}

		return nil

	case client && cControl.Crane.status == CraneStatusPublishVerifying:

		// update status of crane
		log.Tracef("spn/docks: crane %s is ready to publish connection", cControl.Crane.ID)
		cControl.Crane.status = CraneStatusPublished

		// publish
		if hooksActive.IsSet() {
			return publishConnectionHook(cControl, cControl.Crane.connectedHub, nil)
		}

		return nil
	}

	return nil
}
