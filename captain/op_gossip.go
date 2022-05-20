package captain

import (
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

// GossipOpType is the type ID of the gossip operation.
const GossipOpType string = "gossip"

// GossipMsgType is the gossip message type.
type GossipMsgType uint8

// Gossip Message Types.
const (
	GossipHubAnnouncementMsg GossipMsgType = 1
	GossipHubStatusMsg       GossipMsgType = 2
)

func (msgType GossipMsgType) String() string {
	switch msgType {
	case GossipHubAnnouncementMsg:
		return "hub announcement"
	case GossipHubStatusMsg:
		return "hub status"
	default:
		return "unknown gossip msg"
	}
}

// GossipOp is used to gossip Hub messages.
type GossipOp struct {
	terminal.OpBase

	controller *docks.CraneControllerTerminal
}

// Type returns the type ID.
func (op *GossipOp) Type() string {
	return GossipOpType
}

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:     GossipOpType,
		Requires: terminal.IsCraneController,
		RunOp:    runGossipOp,
	})
}

// NewGossipOp start a new gossip operation.
func NewGossipOp(controller *docks.CraneControllerTerminal) (*GossipOp, *terminal.Error) {
	// Create and init.
	op := &GossipOp{
		controller: controller,
	}
	op.OpBase.Init()
	err := controller.OpInit(op, nil)
	if err != nil {
		return nil, err
	}

	// Register and return.
	registerGossipOp(controller.Crane.ID, op)
	return op, nil
}

func runGossipOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Check if we are run by a controller.
	controller, ok := t.(*docks.CraneControllerTerminal)
	if !ok {
		return nil, terminal.ErrIncorrectUsage.With("gossip op may only be started by a crane controller terminal, but was started by %T", t)
	}

	// Create, init, register and return.
	op := &GossipOp{
		controller: controller,
	}
	op.OpBase.Init()
	op.OpBase.SetID(opID)
	registerGossipOp(controller.Crane.ID, op)
	return op, nil
}

func (op *GossipOp) sendMsg(msgType GossipMsgType, data []byte) {
	c := container.New(
		varint.Pack8(uint8(msgType)),
		data,
	)
	err := op.controller.OpSendWithTimeout(op, c, time.Second)
	if err != nil {
		log.Debugf("spn/captain: failed to forward %s via %s: %s", msgType, op.controller.Crane.ID, err)
	}
}

// Deliver delivers a message to the operation.
func (op *GossipOp) Deliver(c *container.Container) *terminal.Error {
	gossipMsgTypeN, err := c.GetNextN8()
	if err != nil {
		return terminal.ErrMalformedData.With("failed to parse gossip message type")
	}
	gossipMsgType := GossipMsgType(gossipMsgTypeN)

	// Prepare data.
	data := c.CompileData()
	var announcementData, statusData []byte
	switch gossipMsgType {
	case GossipHubAnnouncementMsg:
		announcementData = data
	case GossipHubStatusMsg:
		statusData = data
	default:
		log.Warningf("spn/captain: received unknown gossip message type from %s: %d", op.controller.Crane.ID, gossipMsgType)
		return nil
	}

	// Import and verify.
	h, forward, tErr := docks.ImportAndVerifyHubInfo(module.Ctx, "", announcementData, statusData, conf.MainMapName, conf.MainMapScope)
	if tErr != nil {
		if tErr.Is(hub.ErrOldData) {
			log.Debugf("spn/captain: ignoring old %s from %s", gossipMsgType, op.controller.Crane.ID)
		} else {
			log.Warningf("spn/captain: failed to import %s from %s: %s", gossipMsgType, op.controller.Crane.ID, tErr)
		}
	} else if forward {
		// Only log if we received something to save/forward.
		log.Infof("spn/captain: received %s for %s", gossipMsgType, h)
	}

	// Relay data.
	if forward {
		gossipRelayMsg(op.controller.Crane.ID, gossipMsgType, data)
	}
	return nil
}

// End ends the operation.
func (op *GossipOp) End(err *terminal.Error) (errorToSend *terminal.Error) {
	deleteGossipOp(op.controller.Crane.ID)
	return err
}
