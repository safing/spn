package docks

import (
	"time"

	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/info"
	"github.com/safing/portbase/log"

	"github.com/safing/jess"
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

/*

Crane Init Message Format:
used by init procedures

- Data [bytes block]
	- MsgType [varint]
	- Data [bytes; only when MsgType is Verify or Start*]

Crane Init Response Format:

- Data [bytes block]

Crane Operational Message Format:

- Data [bytes block]
	- possibly encrypted

*/

const (
	CraneMsgTypeEnd              = 0
	CraneMsgTypeInfo             = 1
	CraneMsgTypeVerify           = 2
	CraneMsgTypeRequestHubInfo   = 3
	CraneMsgTypeStartEncrypted   = 4
	CraneMsgTypeStartUnencrypted = 5
)

func (crane *Crane) Start() error {
	log.Tracef("spn/docks: %s is starting", crane)

	// Start crane depending on situation.
	var err *terminal.Error
	if crane.ship.IsMine() {
		err = crane.startLocal()
	} else {
		err = crane.startRemote()
	}

	// Stop crane again if starting failed.
	if err != nil {
		crane.Stop(err)
		return err
	} else {
		log.Debugf("spn/docks: %s started", crane)
		// Return an explicit nil for working "!= nil" checks.
		return nil
	}
}

func (crane *Crane) startLocal() *terminal.Error {
	module.StartWorker("crane unloader", crane.unloader)

	if !crane.ship.IsSecure() {
		// Start encrypted channel.
		// Check if we have all the data we need from the Hub.
		if crane.ConnectedHub == nil {
			return terminal.ErrIncorrectUsage.With("cannot start encrypted channel without connected hub")
		}

		// Try to select a public key.
		signet := crane.ConnectedHub.SelectSignet()
		if signet == nil {
			// We have no signet, request hub info.
			err := crane.ship.Load(append(
				varint.Pack8(1),
				varint.Pack8(CraneMsgTypeRequestHubInfo)...,
			))
			if err != nil {
				return terminal.ErrShipSunk.With("failed to request unencrypted channel: %s", err)
			}

			// Wait for reply.
			var reply *container.Container
			select {
			case reply = <-crane.unloading:
			case <-time.After(1 * time.Second):
				return terminal.ErrTimeout.With("timed out waiting for hub info")
			case <-crane.ctx.Done():
				return terminal.ErrShipSunk
			}

			// Parse and import Announcement.
			announcementData, err := reply.GetNextBlock()
			if err != nil {
				return terminal.ErrMalformedData.With("failed to get announcement: %w", err)
			}
			err = hub.ImportAnnouncement(announcementData, hub.ScopePublic)
			if err != nil {
				return terminal.ErrInternalError.With("failed to import announcement: %w", err)
			}
			// Parse and import Status.
			statusData, err := reply.GetNextBlock()
			if err != nil {
				return terminal.ErrMalformedData.With("failed to get status: %w", err)
			}
			err = hub.ImportStatus(statusData, hub.ScopePublic)
			if err != nil {
				return terminal.ErrInternalError.With("failed to import status: %w", err)
			}

			// Refetch Hub from DB to ensure we have the new version.
			dstHub, err := hub.GetHub(hub.ScopePublic, crane.ConnectedHub.ID)
			if err != nil {
				return terminal.ErrInternalError.With("failed to refetch destination Hub: %w", err)
			}
			crane.ConnectedHub = dstHub

			// Now, try to select a public key again.
			signet = crane.ConnectedHub.SelectSignet()
			if signet == nil {
				return terminal.ErrHubNotReady.With("failed to select signet even after hub info request")
			}
		}

		// Configure encryption.
		env := jess.NewUnconfiguredEnvelope()
		env.SuiteID = jess.SuiteWireV1
		env.Recipients = []*jess.Signet{signet}

		// Do not encrypt directly, rather get session for future use, then encrypt.
		var err error
		crane.jession, err = env.WireCorrespondence(nil)
		if err != nil {
			return terminal.ErrInternalError.With("failed to create encryption session: %w", err)
		}
	}

	// Create crane controller.
	_, initData, tErr := NewLocalCraneControllerTerminal(crane, &terminal.TerminalOpts{
		QueueSize: 100,
		Padding:   8,
	})
	if tErr != nil {
		return tErr.Wrap("failed to set up controller")
	}

	// Prepare init message for sending.
	if crane.ship.IsSecure() {
		initData.PrependNumber(CraneMsgTypeStartUnencrypted)
	} else {
		// Encrypt controller initializer.
		letter, err := crane.jession.Close(initData.CompileData())
		if err != nil {
			return terminal.ErrInternalError.With("failed to encrypt initial packet: %w", err)
		}
		initData, err = letter.ToWire()
		if err != nil {
			return terminal.ErrInternalError.With("failed to pack initial packet: %w", err)
		}
		initData.PrependNumber(CraneMsgTypeStartEncrypted)
	}

	// Send start message.
	initData.PrependLength()
	err := crane.ship.Load(initData.CompileData())
	if err != nil {
		return terminal.ErrShipSunk.With("failed to send init msg: %w", err)
	}

	// Start remaining workers.
	module.StartWorker("crane loader", crane.loader)
	module.StartWorker("crane handler", crane.handler)

	return nil
}

func (crane *Crane) startRemote() *terminal.Error {
	var initMsg *container.Container

	module.StartWorker("crane unloader", crane.unloader)

handling:
	for {
		// Wait for request.
		var request *container.Container
		select {
		case request = <-crane.unloading:

		case <-time.After(1 * time.Second):
			return terminal.ErrTimeout.With("timed out waiting for crane init msg")
		case <-crane.ctx.Done():
			return terminal.ErrShipSunk
		}

		msgType, err := request.GetNextN8()
		if err != nil {
			return terminal.ErrMalformedData.With("failed to parse crane msg type: %s", err)
		}

		switch msgType {
		case CraneMsgTypeEnd:
			// End connection.
			return terminal.ErrStopping

		case CraneMsgTypeInfo:
			// Info is a terminating request.
			err := crane.handleCraneInfo()
			if err != nil {
				return err
			}

		case CraneMsgTypeVerify:
			// Verify is a terminating request.
			err := crane.handleCraneVerification(request)
			if err != nil {
				return err
			}

		case CraneMsgTypeRequestHubInfo:
			// Handle Hub info request.
			err := crane.handleCraneHubInfo()
			if err != nil {
				return err
			}

		case CraneMsgTypeStartUnencrypted:
			initMsg = request

			// Start crane with initMsg.
			break handling

		case CraneMsgTypeStartEncrypted:
			if crane.identity == nil {
				return terminal.ErrIncorrectUsage.With("cannot start incoming crane without designated identity")
			}

			// Set up encryption.
			letter, err := jess.LetterFromWireData(request.CompileData())
			if err != nil {
				return terminal.ErrMalformedData.With("failed to unpack initial packet: %w", err)
			}
			crane.jession, err = letter.WireCorrespondence(crane.identity)
			if err != nil {
				return terminal.ErrInternalError.With("failed to create encryption session: %w", err)
			}
			initMsgData, err := crane.jession.Open(letter)
			if err != nil {
				return terminal.ErrIntegrity.With("failed to decrypt initial packet: %w", err)
			}
			initMsg = container.New(initMsgData)

			// Start crane with initMsg.
			break handling
		}
	}

	_, _, err := NewRemoteCraneControllerTerminal(crane, initMsg)
	if err != nil {
		return err.Wrap("failed to start crane controller")
	}

	// Start remaining workers.
	module.StartWorker("crane loader", crane.loader)
	module.StartWorker("crane handler", crane.handler)

	return nil
}

func (crane *Crane) handleCraneInfo() *terminal.Error {
	// Pack info data.
	infoData, err := dsd.Dump(info.GetInfo(), dsd.JSON)
	if err != nil {
		return terminal.ErrInternalError.With("failed to pack info: %w", err)
	}
	msg := container.New(infoData)

	// Manually send reply.
	msg.PrependLength()
	err = crane.ship.Load(msg.CompileData())
	if err != nil {
		return terminal.ErrShipSunk.With("failed to send info reply: %w", err)
	}

	return nil
}

func (crane *Crane) handleCraneHubInfo() *terminal.Error {
	msg := container.New()

	// Check if we have an identity.
	if crane.identity == nil {
		return terminal.ErrIncorrectUsage.With("cannot handle hub info request without designated identity")
	}

	// Add Hub Announcement.
	announcementData, err := crane.identity.ExportAnnouncement()
	if err != nil {
		return terminal.ErrInternalError.With("failed to export announcement: %w", err)
	}
	msg.AppendAsBlock(announcementData)

	// Add Hub Status.
	statusData, err := crane.identity.ExportStatus()
	if err != nil {
		return terminal.ErrInternalError.With("failed to export status: %w", err)
	}
	msg.AppendAsBlock(statusData)

	// Manually send reply.
	msg.PrependLength()
	err = crane.ship.Load(msg.CompileData())
	if err != nil {
		return terminal.ErrShipSunk.With("failed to send hub info reply: %w", err)
	}

	return nil
}
