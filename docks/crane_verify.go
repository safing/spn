package docks

import (
	"github.com/safing/portbase/container"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

const (
	hubVerificationPurpose = "hub address"
)

func VerifyHubAddress(h *hub.Hub) error {
	// FIXME: implement
	return nil
}

func (crane *Crane) handleCraneVerification(request *container.Container) *terminal.Error {
	// Check if we have an identity.
	if crane.identity == nil {
		return terminal.ErrIncorrectUsage.With("cannot handle verification request without designated identity")
	}

	response, err := crane.identity.SignVerificationRequest(
		request.CompileData(),
		hubVerificationPurpose,
		"", "",
	)
	if err != nil {
		return terminal.ErrInternalError.With("failed to sign verification request: %w", err)
	}
	msg := container.New(response)

	// Manually send reply.
	msg.PrependLength()
	err = crane.ship.Load(msg.CompileData())
	if err != nil {
		return terminal.ErrShipSunk.With("failed to send verification reply: %w", err)
	}

	return nil
}
