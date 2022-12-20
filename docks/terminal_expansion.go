package docks

import (
	"fmt"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/terminal"
)

// ExpansionTerminal is used for expanding to another Hub.
type ExpansionTerminal struct {
	*terminal.TerminalBase

	relayOp *ExpansionTerminalRelayOp

	changeNotifyFuncReady *abool.AtomicBool
	changeNotifyFunc      func()
}

// ExpansionTerminalRelayOp the operation that connects to the relay.
type ExpansionTerminalRelayOp struct {
	terminal.OperationBase

	expansionTerminal *ExpansionTerminal
}

// Type returns the type ID.
func (op *ExpansionTerminalRelayOp) Type() string {
	return ExpandOpType
}

// ExpandTo initiates an expansion.
func ExpandTo(from terminal.Terminal, routeTo string, encryptFor *hub.Hub) (*ExpansionTerminal, *terminal.Error) {
	// First, create the local endpoint terminal to generate the init data.

	// Create options and bare expansion terminal.
	opts := terminal.DefaultExpansionTerminalOpts()
	opts.Encrypt = encryptFor != nil
	expansion := &ExpansionTerminal{
		changeNotifyFuncReady: abool.New(),
	}
	expansion.relayOp = &ExpansionTerminalRelayOp{
		expansionTerminal: expansion,
	}

	// Create base terminal for expansion.
	base, initData, tErr := terminal.NewLocalBaseTerminal(
		module.Ctx,
		0, // Ignore; The ID of the operation is used for communication.
		from.FmtID(),
		encryptFor,
		opts,
		expansion.relayOp,
	)
	if tErr != nil {
		return nil, tErr.Wrap("failed to create expansion terminal base")
	}
	expansion.TerminalBase = base
	base.SetTerminalExtension(expansion)

	// Second, start the actual relay operation.

	// Create setup message for relay operation.
	opInitData := container.New()
	opInitData.AppendAsBlock([]byte(routeTo))
	opInitData.AppendContainer(initData)

	// Start relay operation on connected Hub.
	tErr = from.StartOperation(expansion.relayOp, opInitData, 5*time.Second)
	if tErr != nil {
		return nil, tErr.Wrap("failed to start expansion operation")
	}

	// Start Workers.
	base.StartWorkers(module, "expansion terminal")

	return expansion, nil
}

// SetChangeNotifyFunc sets a callback function that is called when the terminal state changes.
func (t *ExpansionTerminal) SetChangeNotifyFunc(f func()) {
	if t.changeNotifyFuncReady.IsSet() {
		return
	}
	t.changeNotifyFunc = f
	t.changeNotifyFuncReady.Set()
}

// HandleDestruction gives the terminal the ability to clean up.
// The terminal has already fully shut down at this point.
// Should never be called directly. Call Abandon() instead.
func (t *ExpansionTerminal) HandleDestruction(err *terminal.Error) {
	// Trigger update of connected Pin.
	if t.changeNotifyFuncReady.IsSet() {
		t.changeNotifyFunc()
	}

	// Stop the relay operation.
	// The error message is arlready sent by the terminal.
	t.relayOp.Stop(t.relayOp, nil)
}

// CustomIDFormat formats the terminal ID.
func (t *ExpansionTerminal) CustomIDFormat() string {
	return fmt.Sprintf("%s~%d", t.relayOp.Terminal().FmtID(), t.relayOp.ID())
}

// Deliver delivers a message to the operation.
func (op *ExpansionTerminalRelayOp) Deliver(msg *terminal.Msg) *terminal.Error {
	// Proxy directly to expansion terminal.
	return op.expansionTerminal.Deliver(msg)
}

// HandleStop gives the operation the ability to cleanly shut down.
// The returned error is the error to send to the other side.
// Should never be called directly. Call Stop() instead.
func (op *ExpansionTerminalRelayOp) HandleStop(err *terminal.Error) (errorToSend *terminal.Error) {
	// Stop the expansion terminal.
	// The error message will be sent by the operation.
	op.expansionTerminal.Abandon(nil)

	return err
}
