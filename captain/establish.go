package captain

import (
	"fmt"

	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
	"github.com/safing/spn/terminal"
)

func EstablishCrane(dst *hub.Hub) (*docks.Crane, error) {
	if docks.GetAssignedCrane(dst.ID) != nil {
		return nil, fmt.Errorf("route to %s already exists", dst.ID)
	}

	ship, err := ships.Launch(module.Ctx, dst, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to launch ship: %w", err)
	}

	crane, err := docks.NewCrane(module.Ctx, ship, dst, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create crane: %w", err)
	}

	err = crane.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start crane: %w", err)
	}

	// Start gossip op for live map updates.
	_, tErr := NewGossipOp(crane.Controller)
	if tErr != nil {
		crane.Stop(tErr)
		return nil, fmt.Errorf("failed to start gossip op: %w", tErr)
	}

	return crane, nil
}

func EstablishPublicLane(dst *hub.Hub) (*docks.Crane, error) {
	crane, err := EstablishCrane(dst)
	if err != nil {
		return nil, err
	}

	publishOp, tErr := NewPublishOp(crane.Controller, publicIdentity)
	if tErr != nil {
		return nil, fmt.Errorf("failed to publish: %w", err)
	}

	// Wait for publishing to complete.
	select {
	case tErr := <-publishOp.Result():
		if !tErr.Is(terminal.ErrExplicitAck) {
			// Stop crane again, because we failed to publish it.
			defer crane.Stop(nil)
			return nil, terminal.ErrInternalError.With("failed to publish lane: %w", tErr)
		}

	case <-crane.Controller.Ctx().Done():
		defer crane.Stop(nil)
		return nil, terminal.ErrStopping
	}

	return crane, nil
}
