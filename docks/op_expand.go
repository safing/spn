package docks

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/terminal"
)

// ExpandOpType is the type ID of the expand operation.
const ExpandOpType string = "expand"

var activeExpandOps = new(int64)

// ExpandOp is used to expand to another Hub.
type ExpandOp struct {
	terminal.OperationBase
	opts *terminal.TerminalOpts

	// ctx is the context of the Terminal.
	ctx context.Context
	// cancelCtx cancels ctx.
	cancelCtx context.CancelFunc

	dataRelayed *uint64
	ended       *abool.AtomicBool

	relayTerminal *ExpansionRelayTerminal

	// flowControl holds the flow control system.
	flowControl terminal.FlowControl
	// deliverProxy is populated with the configured deliver function
	deliverProxy func(msg *terminal.Msg) *terminal.Error
	// recvProxy is populated with the configured recv function
	recvProxy func() <-chan *terminal.Msg
	// sendProxy is populated with the configured send function
	sendProxy func(msg *terminal.Msg, timeout time.Duration) *terminal.Error
}

// ExpansionRelayTerminal is a relay used for expansion.
type ExpansionRelayTerminal struct {
	op *ExpandOp

	id    uint32
	crane *Crane

	abandoning *abool.AtomicBool

	// flowControl holds the flow control system.
	flowControl terminal.FlowControl
	// deliverProxy is populated with the configured deliver function
	deliverProxy func(msg *terminal.Msg) *terminal.Error
	// recvProxy is populated with the configured recv function
	recvProxy func() <-chan *terminal.Msg
	// sendProxy is populated with the configured send function
	sendProxy func(msg *terminal.Msg, timeout time.Duration) *terminal.Error
}

// Type returns the type ID.
func (op *ExpandOp) Type() string {
	return ExpandOpType
}

// ID returns the operation ID.
func (t *ExpansionRelayTerminal) ID() uint32 {
	return t.id
}

// Ctx returns the operation context.
func (op *ExpandOp) Ctx() context.Context {
	return op.ctx
}

// Ctx returns the relay terminal context.
func (t *ExpansionRelayTerminal) Ctx() context.Context {
	return t.op.ctx
}

// Deliver delivers a message to the relay operation.
func (op *ExpandOp) Deliver(msg *terminal.Msg) *terminal.Error {
	// Pause unit before handing away.
	msg.PauseUnit()

	return op.deliverProxy(msg)
}

// Deliver delivers a message to the relay terminal.
func (t *ExpansionRelayTerminal) Deliver(msg *terminal.Msg) *terminal.Error {
	// Pause unit before handing away.
	msg.PauseUnit()

	return t.deliverProxy(msg)
}

// Flush writes all data in the queues.
func (op *ExpandOp) Flush() {
	if op.flowControl != nil {
		op.flowControl.Flush()
	}
}

// Flush writes all data in the queues.
func (t *ExpansionRelayTerminal) Flush() {
	if t.flowControl != nil {
		t.flowControl.Flush()
	}
}

func init() {
	terminal.RegisterOpType(terminal.OperationFactory{
		Type:     ExpandOpType,
		Requires: terminal.MayExpand,
		Start:    expand,
	})
}

func expand(t terminal.Terminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Submit metrics.
	newExpandOp.Inc()

	// Check if we are running a public hub.
	if !conf.PublicHub() {
		return nil, terminal.ErrPermissinDenied.With("expanding is only allowed on public hubs")
	}

	// Parse destination hub ID.
	dstData, err := data.GetNextBlock()
	if err != nil {
		return nil, terminal.ErrMalformedData.With("failed to parse destination: %w", err)
	}

	// Parse terminal options.
	opts, tErr := terminal.ParseTerminalOpts(data)
	if tErr != nil {
		return nil, tErr.Wrap("failed to parse terminal options")
	}

	// Get crane with destination.
	relayCrane := GetAssignedCrane(string(dstData))
	if relayCrane == nil {
		return nil, terminal.ErrHubUnavailable.With("no crane assigned to %q", string(dstData))
	}

	// TODO: Expand outside of hot path.

	// Create operation and terminal.
	op := &ExpandOp{
		opts:        opts,
		dataRelayed: new(uint64),
		ended:       abool.New(),
		relayTerminal: &ExpansionRelayTerminal{
			crane:      relayCrane,
			id:         relayCrane.getNextTerminalID(),
			abandoning: abool.New(),
		},
	}
	op.InitOperationBase(t, opID)
	op.ctx, op.cancelCtx = context.WithCancel(t.Ctx())
	op.relayTerminal.op = op

	// Create flow control.
	switch opts.FlowControl {
	case terminal.FlowControlDFQ:
		// Operation
		op.flowControl = terminal.NewDuplexFlowQueue(op.ctx, opts.FlowControlSize, op.submitBackstream)
		op.deliverProxy = op.flowControl.Deliver
		op.recvProxy = op.flowControl.Receive
		op.sendProxy = op.flowControl.Send
		// Relay Terminal
		op.relayTerminal.flowControl = terminal.NewDuplexFlowQueue(op.ctx, opts.FlowControlSize, op.submitForwardstream)
		op.relayTerminal.deliverProxy = op.relayTerminal.flowControl.Deliver
		op.relayTerminal.recvProxy = op.relayTerminal.flowControl.Receive
		op.relayTerminal.sendProxy = op.relayTerminal.flowControl.Send
	case terminal.FlowControlNone:
		// Operation
		deliverToOp := make(chan *terminal.Msg, opts.FlowControlSize)
		op.deliverProxy = terminal.MakeDirectDeliveryDeliverFunc(op.ctx, deliverToOp)
		op.recvProxy = terminal.MakeDirectDeliveryRecvFunc(deliverToOp)
		op.sendProxy = op.submitBackstream
		// Relay Terminal
		deliverToRelay := make(chan *terminal.Msg, opts.FlowControlSize)
		op.relayTerminal.deliverProxy = terminal.MakeDirectDeliveryDeliverFunc(op.ctx, deliverToRelay)
		op.relayTerminal.recvProxy = terminal.MakeDirectDeliveryRecvFunc(deliverToRelay)
		op.relayTerminal.sendProxy = op.submitForwardstream
	case terminal.FlowControlDefault:
		fallthrough
	default:
		return nil, terminal.ErrInternalError.With("unknown flow control type %d", opts.FlowControl)
	}

	// Establish terminal on destination.
	newInitData, tErr := opts.Pack()
	if tErr != nil {
		return nil, terminal.ErrInternalError.With("failed to re-pack options: %w", err)
	}
	tErr = op.relayTerminal.crane.EstablishNewTerminal(op.relayTerminal, newInitData)
	if tErr != nil {
		return nil, tErr
	}

	// Start workers.
	module.StartWorker("expand op forward relay", op.forwardHandler)
	module.StartWorker("expand op backward relay", op.backwardHandler)
	if op.flowControl != nil {
		op.flowControl.StartWorkers(module, "expand op")
	}
	if op.relayTerminal.flowControl != nil {
		op.relayTerminal.flowControl.StartWorkers(module, "expand op terminal")
	}

	return op, nil
}

func (op *ExpandOp) submitForwardstream(msg *terminal.Msg, timeout time.Duration) *terminal.Error {
	msg.FlowID = op.relayTerminal.id
	if msg.IsHighPriorityUnit() && op.opts.UsePriorityDataMsgs {
		msg.Type = terminal.MsgTypePriorityData
	} else {
		msg.Type = terminal.MsgTypeData
	}
	err := op.relayTerminal.crane.Send(msg, timeout)
	if err != nil {
		msg.FinishUnit()
		op.Stop(op, err.Wrap("failed to submit forward from relay op"))
	}
	return err
}

func (op *ExpandOp) submitBackstream(msg *terminal.Msg, timeout time.Duration) *terminal.Error {
	msg.FlowID = op.relayTerminal.id
	if msg.IsHighPriorityUnit() && op.opts.UsePriorityDataMsgs {
		msg.Type = terminal.MsgTypePriorityData
	} else {
		msg.Type = terminal.MsgTypeData
		msg.RemoveUnitPriority()
	}
	// Note: op.Send() will transform high priority units to priority data msgs.
	err := op.Send(msg, timeout)
	if err != nil {
		msg.FinishUnit()
		op.Stop(op, err.Wrap("failed to submit backward from relay op"))
	}
	return err
}

func (op *ExpandOp) forwardHandler(_ context.Context) error {
	// FIXME: can we just use the upstream handler to do this in a function instead?

	// Metrics setup and submitting.
	atomic.AddInt64(activeExpandOps, 1)
	started := time.Now()
	defer func() {
		atomic.AddInt64(activeExpandOps, -1)
		expandOpDurationHistogram.UpdateDuration(started)
		expandOpRelayedDataHistogram.Update(float64(atomic.LoadUint64(op.dataRelayed)))
	}()

	for {
		select {
		case msg := <-op.recvProxy():
			// Debugging:
			// log.Debugf("spn/testing: forwarding at %s: %s", op.FmtID(), spew.Sdump(c.CompileData()))

			// Count relayed data for metrics.
			atomic.AddUint64(op.dataRelayed, uint64(msg.Data.Length()))

			// Wait for processing slot.
			msg.WaitForUnitSlot()

			// Receive data from the origin and forward it to the relay.
			msg.PauseUnit()
			if err := op.relayTerminal.sendProxy(msg, 1*time.Minute); err != nil {
				msg.FinishUnit()
				op.relayTerminal.Abandon(err)
				return nil
			}

		case <-op.ctx.Done():
			return nil
		}
	}
}

func (op *ExpandOp) backwardHandler(_ context.Context) error {
	// FIXME: can we just use the upstream handler to do this in a function instead?

	for {
		select {
		case msg := <-op.relayTerminal.recvProxy():
			// Debugging:
			// log.Debugf("spn/testing: backwarding at %s: %s", op.FmtID(), spew.Sdump(c.CompileData()))

			// Wait for processing slot.
			msg.WaitForUnitSlot()

			// Count relayed data for metrics.
			atomic.AddUint64(op.dataRelayed, uint64(msg.Data.Length()))

			// Receive data from the relay and forward it to the origin.
			msg.PauseUnit()
			if err := op.sendProxy(msg, 1*time.Minute); err != nil {
				msg.FinishUnit()
				op.Stop(op, err)
				return nil
			}

		case <-op.ctx.Done():
			return nil
		}
	}
}

// HandleStop gives the operation the ability to cleanly shut down.
// The returned error is the error to send to the other side.
// Should never be called directly. Call Stop() instead.
func (op *ExpandOp) HandleStop(err *terminal.Error) (errorToSend *terminal.Error) {
	// Stop connected workers.
	op.cancelCtx()

	// Abandon connected terminal.
	op.relayTerminal.Abandon(nil)

	// Add context to error.
	if err.IsError() {
		return err.Wrap("relay operation failed with")
	}
	return err
}

// Abandon shuts down the terminal unregistering it from upstream and calling HandleAbandon().
func (t *ExpansionRelayTerminal) Abandon(err *terminal.Error) {
	if t.abandoning.SetToIf(false, true) {
		module.StartWorker("terminal abandon procedure", func(_ context.Context) error {
			t.handleAbandonProcedure(err)
			return nil
		})
	}
}

// HandleAbandon gives the terminal the ability to cleanly shut down.
// The returned error is the error to send to the other side.
// Should never be called directly. Call Abandon() instead.
func (t *ExpansionRelayTerminal) HandleAbandon(err *terminal.Error) (errorToSend *terminal.Error) {
	// Stop the connected relay operation.
	t.op.Stop(t.op, err)

	// Add context to error.
	if err.IsError() {
		return err.Wrap("relay terminal failed with")
	}
	return err
}

// HandleDestruction gives the terminal the ability to clean up.
// The terminal has already fully shut down at this point.
// Should never be called directly. Call Abandon() instead.
func (t *ExpansionRelayTerminal) HandleDestruction(err *terminal.Error) {}

func (t *ExpansionRelayTerminal) handleAbandonProcedure(err *terminal.Error) {
	// Call operation stop handle function for proper shutdown cleaning up.
	err = t.HandleAbandon(err)

	// Send error to the connected Operation, if the error is internal.
	if !err.IsExternal() {
		if err == nil {
			err = terminal.ErrStopping
		}

		msg := terminal.NewMsg(err.Pack())
		msg.FlowID = t.ID()
		msg.Type = terminal.MsgTypeStop

		tErr := t.op.submitForwardstream(msg, 1*time.Second)
		if tErr.IsError() {
			msg.FinishUnit()
			log.Warningf("spn/terminal: relay terminal %s failed to send stop msg: %s", t.FmtID(), tErr)
		}
	}

	// Flush all messages before stopping.
	t.Flush()
}

// FmtID returns the expansion ID hierarchy.
func (op *ExpandOp) FmtID() string {
	return fmt.Sprintf("%s>%d <r> %s#%d", op.Terminal().FmtID(), op.ID(), op.relayTerminal.crane.ID, op.relayTerminal.id)
}

// FmtID returns the expansion ID hierarchy.
func (t *ExpansionRelayTerminal) FmtID() string {
	return fmt.Sprintf("%s#%d", t.crane.ID, t.id)
}

// Fulfill terminal interface.

// Send is used by others to send a message through the terminal.
func (t *ExpansionRelayTerminal) Send(msg *terminal.Msg, timeout time.Duration) *terminal.Error {
	return terminal.ErrInternalError.With("relay terminal cannot be used to send messages")
}

// StartOperation starts the given operation by assigning it an ID and sending the given operation initialization data.
func (t *ExpansionRelayTerminal) StartOperation(op terminal.Operation, initData *container.Container, timeout time.Duration) *terminal.Error {
	return terminal.ErrInternalError.With("relay terminal cannot start operations")
}

// StopOperation stops the given operation.
func (t *ExpansionRelayTerminal) StopOperation(op terminal.Operation, err *terminal.Error) {
	log.Critical("spn/docks: internal error: relay terminal cannot stop operations")
}
