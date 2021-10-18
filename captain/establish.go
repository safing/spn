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

func EstablishCrane(dst *hub.Hub) (*docks.Crane, error) {
	if conf.PublicHub() && dst.ID == publicIdentity.ID {
		return nil, errors.New("connecting to self")
	}
	if docks.GetAssignedCrane(dst.ID) != nil {
		return nil, fmt.Errorf("route to %s already exists", dst.ID)
	}

	ship, err := ships.Launch(context.Background(), dst, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to launch ship: %w", err)
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

	// Query all gossip msgs on first connection.
	if gossipQueryInitiated.SetToIf(false, true) {
		_, tErr = NewGossipQueryOp(crane.Controller)
		if tErr != nil {
			log.Warningf("spn/captain: failed to start initial gossip query: %s", tErr)
			gossipQueryInitiated.UnSet()
		}
	}

	return crane, nil
}
