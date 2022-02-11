package captain

import (
	"context"
	"strings"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

// GossipQueryOpType is the type ID of the gossip query operation.
const GossipQueryOpType string = "gossip/query"

// GossipQueryOp is used to query gossip messages.
type GossipQueryOp struct {
	terminal.OpBase

	t         terminal.OpTerminal
	client    bool
	importCnt int

	ctx       context.Context
	cancelCtx context.CancelFunc
}

// Type returns the type ID.
func (op *GossipQueryOp) Type() string {
	return GossipQueryOpType
}

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:     GossipQueryOpType,
		Requires: terminal.IsCraneController,
		RunOp:    runGossipQueryOp,
	})
}

// NewGossipQueryOp starts a new gossip query operation.
func NewGossipQueryOp(t terminal.OpTerminal) (*GossipQueryOp, *terminal.Error) {
	// Create and init.
	op := &GossipQueryOp{
		t:      t,
		client: true,
	}
	op.ctx, op.cancelCtx = context.WithCancel(context.Background())
	op.OpBase.Init()
	err := t.OpInit(op, nil)
	if err != nil {
		return nil, err
	}
	return op, nil
}

func runGossipQueryOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Create, init, register and return.
	op := &GossipQueryOp{t: t}
	op.ctx, op.cancelCtx = context.WithCancel(context.Background())
	op.OpBase.Init()
	op.OpBase.SetID(opID)

	module.StartWorker("gossip query handler", op.handler)

	return op, nil
}

func (op *GossipQueryOp) handler(_ context.Context) error {
	tErr := op.sendMsgs(hub.MsgTypeAnnouncement)
	if tErr != nil {
		op.t.OpEnd(op, tErr)
		return nil // Clean worker exit.
	}

	tErr = op.sendMsgs(hub.MsgTypeStatus)
	if tErr != nil {
		op.t.OpEnd(op, tErr)
		return nil // Clean worker exit.
	}

	op.t.OpEnd(op, nil)
	return nil // Clean worker exit.
}

func (op *GossipQueryOp) sendMsgs(msgType hub.MsgType) *terminal.Error {
	it, err := hub.QueryRawGossipMsgs(conf.MainMapName, msgType)
	if err != nil {
		return terminal.ErrInternalError.With("failed to query: %w", err)
	}
	defer it.Cancel()

iterating:
	for {
		select {
		case r := <-it.Next:
			// Check if we are done.
			if r == nil {
				return nil
			}

			// Ensure we're handling a hub msg.
			hubMsg, err := hub.EnsureHubMsg(r)
			if err != nil {
				log.Warningf("spn/captain: failed to load hub msg: %s", err)
				continue iterating
			}

			// Create gossip msg.
			var c *container.Container
			switch hubMsg.Type {
			case hub.MsgTypeAnnouncement:
				c = container.New(
					varint.Pack8(uint8(GossipHubAnnouncementMsg)),
					hubMsg.Data,
				)
			case hub.MsgTypeStatus:
				c = container.New(
					varint.Pack8(uint8(GossipHubStatusMsg)),
					hubMsg.Data,
				)
			default:
				log.Warningf("spn/captain: unknown hub msg for gossip query at %q: %s", hubMsg.Key(), hubMsg.Type)
			}

			// Send msg.
			if c != nil {
				tErr := op.t.OpSendWithTimeout(op, c, 100*time.Millisecond)
				if tErr != nil {
					return tErr.Wrap("failed to send msg")
				}
			}

		case <-op.ctx.Done():
			return terminal.ErrStopping
		}
	}
}

// Deliver delivers the message to the operation.
func (op *GossipQueryOp) Deliver(c *container.Container) *terminal.Error {
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
		log.Warningf("spn/captain: received unknown gossip message type from gossip query: %d", gossipMsgType)
		return nil
	}

	// Import and verify.
	h, forward, tErr := docks.ImportAndVerifyHubInfo(module.Ctx, "", announcementData, statusData, conf.MainMapName, conf.MainMapScope)
	if tErr != nil {
		log.Warningf("spn/captain: failed to import %s from gossip query: %s", gossipMsgType, tErr)
	} else {
		log.Infof("spn/captain: received %s for %s from gossip query", gossipMsgType, h)
		op.importCnt++
	}

	// Relay data.
	if forward {
		// TODO: Find better way to get craneID.
		craneID := strings.SplitN(op.t.FmtID(), "#", 2)[0]
		gossipRelayMsg(craneID, gossipMsgType, data)
	}
	return nil
}

// End ends the operation.
func (op *GossipQueryOp) End(err *terminal.Error) {
	if op.client {
		log.Infof("spn/captain: gossip query imported %d entries", op.importCnt)
	}
	op.cancelCtx()
}
