package docks

import (
	"context"
	"errors"
	"fmt"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

// ExpansionTerminal is used for expanding to another Hub.
type ExpansionTerminal struct {
	*terminal.TerminalBase

	opID         uint32
	relayOp      terminal.OpTerminal
	relayOpEnded *abool.AtomicBool

	changeNotifyFuncReady *abool.AtomicBool
	changeNotifyFunc      func()
}

// ExpandTo initiates an expansion.
func ExpandTo(t terminal.OpTerminal, routeTo string, encryptFor *hub.Hub) (*ExpansionTerminal, *terminal.Error) {
	// Create expansion terminal.
	opts := &terminal.TerminalOpts{
		Version:         1,
		Padding:         8,
		FlowControl:     terminal.FlowControlNone,
		FlowControlSize: terminal.DefaultQueueSize,
		Encrypt:         encryptFor != nil,
	}
	expansion := &ExpansionTerminal{
		relayOp:               t,
		relayOpEnded:          abool.New(),
		changeNotifyFuncReady: abool.New(),
	}

	// Create base terminal for expansion.
	tBase, initData, tErr := terminal.NewLocalBaseTerminal(
		context.Background(),
		0,
		t.FmtID(),
		encryptFor,
		opts,
		expansion.submitUpstream,
		false,
	)
	if tErr != nil {
		return nil, tErr.Wrap("failed to create expansion terminal base")
	}
	tBase.SetTerminalExtension(expansion)
	tBase.SetTimeout(expansionClientTimeout)
	expansion.TerminalBase = tBase

	// Create setup message.
	opMsg := container.New()
	opMsg.AppendAsBlock([]byte(routeTo))
	opMsg.AppendContainer(initData)

	// Initialize expansion.
	tErr = t.OpInit(expansion, opMsg)
	if tErr != nil {
		return nil, tErr.Wrap("failed to init expansion")
	}

	// Start Workers.
	tBase.StartWorkers(module, "expansion terminal")

	return expansion, nil
}

func (t *ExpansionTerminal) submitUpstream(c *container.Container) *terminal.Error {
	err := t.relayOp.OpSend(t, c)
	if err != nil {
		t.relayOp.OpEnd(t, err.Wrap("failed to send relay op msg"))
	}
	return err
}

// ID returns the operation ID.
func (t *ExpansionTerminal) ID() uint32 {
	return t.opID
}

// SetID sets the operation ID.
func (t *ExpansionTerminal) SetID(id uint32) {
	t.opID = id
}

// Type returns the type ID.
func (t *ExpansionTerminal) Type() string {
	return ExpandOpType
}

// HasEnded returns whether the operation has ended.
func (t *ExpansionTerminal) HasEnded(end bool) bool {
	if end {
		// Return false if we just only set it to ended.
		return !t.relayOpEnded.SetToIf(false, true)
	}
	return t.relayOpEnded.IsSet()
}

// End ends the operation.
func (t *ExpansionTerminal) End(err *terminal.Error) {
	t.stop(err)
}

// Abandon ends the terminal.
func (t *ExpansionTerminal) Abandon(err *terminal.Error) {
	t.stop(err)
}

func (t *ExpansionTerminal) stop(err *terminal.Error) {
	if t.Abandoning.SetToIf(false, true) {
		switch {
		case err == nil:
			log.Debugf("spn/docks: expansion terminal %s is being abandoned", t.FmtID())
		case errors.Is(err, terminal.ErrTimeout):
			log.Debugf("spn/docks: expansion terminal %s %s", t.FmtID(), err)
		default:
			log.Warningf("spn/docks: expansion terminal %s: %s", t.FmtID(), err)
		}

		// End all operations.
		t.StartAbandonProcedure(nil, false, func() {
			// Send stop message.
			t.relayOp.OpEnd(t, nil)

			// Trigger update of connected Pin.
			if t.changeNotifyFuncReady.IsSet() {
				t.changeNotifyFunc()
			}
		})
	}
}

// IsBeingAbandoned returns whether the terminal is being abandoned.
func (t *ExpansionTerminal) IsBeingAbandoned() bool {
	return t.Abandoning.IsSet()
}

// SetChangeNotifyFunc sets a callback function that is called when the terminal state changes.
func (t *ExpansionTerminal) SetChangeNotifyFunc(f func()) {
	if t.changeNotifyFuncReady.IsSet() {
		return
	}
	t.changeNotifyFunc = f
	t.changeNotifyFuncReady.Set()
}

// FmtID formats the operation ID.
func (t *ExpansionTerminal) FmtID() string {
	return fmt.Sprintf("%s#%d", t.relayOp.FmtID(), t.opID)
}
