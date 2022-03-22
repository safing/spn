package captain

import (
	"context"
	"errors"
	"fmt"

	"github.com/safing/portbase/log"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
	"github.com/safing/spn/terminal"
)

// EstablishCrane establishes a crane to another Hub.
func EstablishCrane(ctx context.Context, dst *hub.Hub) (*docks.Crane, error) {
	if conf.PublicHub() && dst.ID == publicIdentity.ID {
		return nil, errors.New("connecting to self")
	}
	if docks.GetAssignedCrane(dst.ID) != nil {
		return nil, fmt.Errorf("route to %s already exists", dst.ID)
	}

	ship, err := ships.Launch(ctx, dst, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to launch ship: %w", err)
	}

	// On pure clients, mark all ships as public in order to show unmasked data in logs.
	if conf.Client() && !conf.PublicHub() {
		ship.MarkPublic()
	}

	crane, err := docks.NewCrane(context.Background(), ship, dst, publicIdentity)
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

// EstablishPublicLane establishes a crane to another Hub and publishes it.
func EstablishPublicLane(ctx context.Context, dst *hub.Hub) (*docks.Crane, *terminal.Error) {
	crane, err := EstablishCrane(ctx, dst)
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to establish crane: %w", err)
	}

	// Publish as Lane.
	publishOp, tErr := NewPublishOp(crane.Controller, publicIdentity)
	if tErr != nil {
		return nil, terminal.ErrInternalError.With("failed to publish: %w", err)
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

	case <-ctx.Done():
		return nil, terminal.ErrCanceled
	}

	// Query all gossip msgs.
	_, tErr = NewGossipQueryOp(crane.Controller)
	if tErr != nil {
		log.Warningf("spn/captain: failed to start initial gossip query: %s", tErr)
	}

	return crane, nil
}
