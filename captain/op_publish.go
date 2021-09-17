package captain

import (
	"github.com/safing/portbase/container"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

const PublishOpType string = "publish"

type PublishOp struct {
	terminal.OpBase
	controller *docks.CraneControllerTerminal

	identity      *cabin.Identity
	requestingHub *hub.Hub
	verification  *cabin.Verification
	result        chan *terminal.Error
}

func (op *PublishOp) Type() string {
	return PublishOpType
}

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:     PublishOpType,
		Requires: terminal.IsCraneController,
		RunOp:    runPublishOp,
	})
}

func NewPublishOp(controller *docks.CraneControllerTerminal, identity *cabin.Identity) (*PublishOp, *terminal.Error) {
	// Create and init.
	op := &PublishOp{
		controller: controller,
		identity:   identity,
		result:     make(chan *terminal.Error),
	}
	msg := container.New()

	// Add Hub Announcement.
	announcementData, err := identity.ExportAnnouncement()
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to export announcement: %w", err)
	}
	msg.AppendAsBlock(announcementData)

	// Add Hub Status.
	statusData, err := identity.ExportStatus()
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to export status: %w", err)
	}
	msg.AppendAsBlock(statusData)

	tErr := controller.OpInit(op, msg)
	if tErr != nil {
		return nil, tErr
	}
	return op, nil
}

func runPublishOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Check if we are run by a controller.
	controller, ok := t.(*docks.CraneControllerTerminal)
	if !ok {
		return nil, terminal.ErrIncorrectUsage.With("publish op may only be started by a crane controller terminal, but was started by %T", t)
	}

	// Parse and import Announcement and Status.
	announcementData, err := data.GetNextBlock()
	if err != nil {
		return nil, terminal.ErrMalformedData.With("failed to get announcement: %w", err)
	}
	statusData, err := data.GetNextBlock()
	if err != nil {
		return nil, terminal.ErrMalformedData.With("failed to get status: %w", err)
	}
	h, forward, tErr := docks.ImportAndVerifyHubInfo(module.Ctx, "", announcementData, statusData, hub.ScopePublic)
	if tErr != nil {
		return nil, tErr.Wrap("failed to import and verify hub")
	}
	// Update reference in case it was changed by the import.
	controller.Crane.ConnectedHub = h

	// Relay data.
	if forward {
		gossipRelayMsg(controller.Crane.ID, GossipHubAnnouncementMsg, announcementData)
		gossipRelayMsg(controller.Crane.ID, GossipHubStatusMsg, statusData)
	}

	// Create verification request.
	v, request, err := cabin.CreateVerificationRequest(PublishOpType, "", "")
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to create verification request: %w", err)
	}

	// Create operation.
	op := &PublishOp{
		controller:    controller,
		requestingHub: h,
		verification:  v,
		result:        make(chan *terminal.Error),
	}
	op.SetID(opID)

	// Reply with verification request.
	tErr = controller.OpSend(op, container.New(request))
	if tErr != nil {
		return nil, tErr.Wrap("failed to send verification request")
	}

	return op, nil
}

func (op *PublishOp) Deliver(c *container.Container) *terminal.Error {
	if op.identity != nil {
		// Client

		// Sign the received verification request.
		response, err := op.identity.SignVerificationRequest(c.CompileData(), PublishOpType, "", "")
		if err != nil {
			return terminal.ErrPermissinDenied.With("signing verification request failed: %w", err)
		}

		return op.controller.OpSend(op, container.New(response))
	} else if op.requestingHub != nil {
		// Server

		// Verify the signed request.
		err := op.verification.Verify(c.CompileData(), op.requestingHub)
		if err != nil {
			return terminal.ErrPermissinDenied.With("checking verification request failed: %w", err)
		}
		return terminal.ErrExplicitAck
	}

	return terminal.ErrInternalError.With("invalid operation state")
}

func (op *PublishOp) Result() <-chan *terminal.Error {
	return op.result
}

func (op *PublishOp) End(tErr *terminal.Error) {
	if tErr.Is(terminal.ErrExplicitAck) {
		// TODO: Check for concurrenct access.
		if op.controller.Crane.ConnectedHub == nil {
			op.controller.Crane.ConnectedHub = op.requestingHub
		}

		// Publish crane, abort if it fails.
		err := op.controller.Crane.Publish()
		if err != nil {
			tErr = terminal.ErrInternalError.With("failed to publish crane: %w", err)
			op.controller.Crane.Stop(tErr)
		} else {
			op.controller.Crane.NotifyUpdate()
		}
	}

	select {
	case op.result <- tErr:
	default:
	}
}
