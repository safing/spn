package docks

import (
	"fmt"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

type ExpansionTerminal struct {
	*terminal.TerminalBase
	*terminal.DuplexFlowQueue

	opID    uint32
	relayOp terminal.OpTerminal
}

func ExpandTo(t terminal.OpTerminal, routeTo string, encryptFor *hub.Hub) (*ExpansionTerminal, error) {
	// Create expansion terminal.
	opts := &terminal.TerminalOpts{
		Version:   1,
		QueueSize: 100,
	}
	tBase, initData, tErr := terminal.NewLocalBaseTerminal(module.Ctx, 0, t.FmtID()+"-", encryptFor, opts)
	if tErr != nil {
		return nil, fmt.Errorf("failed to create expansion terminal base: %s", tErr)
	}
	expansion := &ExpansionTerminal{
		TerminalBase: tBase,
		relayOp:      t,
	}
	expansion.TerminalBase.SetTerminalExtension(expansion)
	expansion.DuplexFlowQueue = terminal.NewDuplexFlowQueue(expansion, opts.QueueSize, expansion.submitUpstream)

	// Create setup message.
	opMsg := container.New()
	opMsg.AppendAsBlock([]byte(routeTo))
	opMsg.AppendContainer(initData)

	// Initialize expansion.
	tErr = t.OpInit(expansion, opMsg)
	if tErr != nil {
		return nil, fmt.Errorf("failed to init expansion: %w", tErr)
	}

	module.StartWorker("expansion terminal handler", expansion.Handler)
	module.StartWorker("expansion terminal flow handler", expansion.FlowHandler)

	return expansion, nil
}

func (t *ExpansionTerminal) Deliver(c *container.Container) *terminal.Error {
	return t.DuplexFlowQueue.Deliver(c)
}

func (t *ExpansionTerminal) submitUpstream(c *container.Container) {
	err := t.relayOp.OpSend(t, c)
	if err != nil {
		t.relayOp.OpEnd(t, err.Wrap("failed to send relay op msg"))
	}
}

func (t *ExpansionTerminal) ID() uint32 {
	return t.opID
}

func (t *ExpansionTerminal) SetID(id uint32) {
	t.opID = id
}

func (t *ExpansionTerminal) Type() string {
	return ExpandOpType
}

func (t *ExpansionTerminal) End(err *terminal.Error) {
	t.stop(err, true)
}

func (t *ExpansionTerminal) Abandon(err *terminal.Error) {
	t.stop(err, false)
}

func (t *ExpansionTerminal) stop(err *terminal.Error, opErr bool) {
	if t.Abandoned.SetToIf(false, true) {
		if err != nil {
			log.Warningf("spn/docks: expansion terminal %s: %s", t.FmtID(), err)
		} else {
			log.Debugf("spn/docks: expansion terminal %s is being abandoned", t.FmtID())
		}

		// End all operations and stop all connected workers.
		t.StopAll(err.Wrap("%s was abandoned", t.FmtID()))

		// Notify lower layer.
		if !opErr {
			t.relayOp.OpEnd(t, err)
		}
	}
}

func (t *ExpansionTerminal) FmtID() string {
	return fmt.Sprintf("%s-#%d", t.relayOp.FmtID(), t.opID)
}
