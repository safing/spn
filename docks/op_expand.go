package docks

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/conf"
	"github.com/safing/spn/terminal"
)

// ExpandOpType is the type ID of the expand operation.
const ExpandOpType string = "expand"

var activeExpandOps = new(int64)

// ExpandOp is used to expand to another Hub.
type ExpandOp struct {
	terminal.OpBase
	opTerminal terminal.OpTerminal
	opts       *terminal.TerminalOpts

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
	deliverProxy func(c *container.Container) *terminal.Error
	// recvProxy is populated with the configured recv function
	recvProxy func() <-chan *container.Container
	// sendProxy is populated with the configured send function
	sendProxy func(c *container.Container, highPriority bool) *terminal.Error
	// prioMsgs holds the number of messages to forward with high priority.
	prioMsgs *int32
}

// ExpansionRelayTerminal is a relay used for expansion.
type ExpansionRelayTerminal struct {
	op *ExpandOp

	id    uint32
	crane *Crane

	abandoned *abool.AtomicBool

	// flowControl holds the flow control system.
	flowControl terminal.FlowControl
	// deliverProxy is populated with the configured deliver function
	deliverProxy func(c *container.Container) *terminal.Error
	// recvProxy is populated with the configured recv function
	recvProxy func() <-chan *container.Container
	// sendProxy is populated with the configured send function
	sendProxy func(c *container.Container, highPriority bool) *terminal.Error
	// prioMsgs holds the number of messages to forward with high priority.
	prioMsgs *int32
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
func (op *ExpandOp) Deliver(c *container.Container) *terminal.Error {
	return op.deliverProxy(c)
}

// DeliverHighPriority delivers a high priority message to the relay operation.
func (op *ExpandOp) DeliverHighPriority(c *container.Container) *terminal.Error {
	// Set prioMsgs for prioritizing sending of messages until the high priority
	// one is sent.
	// Always add two extra for (1) the current message and (2) because prioMsgs
	// might be reduced immediately for sending the current message being handled
	// concurrently.
	if op.flowControl != nil {
		// Set prioMsgs of the relay terminal to the full length of the op recv
		// queue, from which the relay terminal reads and then sends.
		atomic.StoreInt32(
			op.relayTerminal.prioMsgs,
			int32(op.flowControl.RecvQueueLen())+2,
		)
	} else {
		// Set prioMsgs of the relay terminal to 2 if no flow control is configured.
		atomic.StoreInt32(op.relayTerminal.prioMsgs, 2)
	}

	return op.deliverProxy(c)
}

// Deliver delivers a message to the relay terminal.
func (t *ExpansionRelayTerminal) Deliver(c *container.Container) *terminal.Error {
	return t.deliverProxy(c)
}

// DeliverHighPriority delivers a high priority message to the relay terminal.
func (t *ExpansionRelayTerminal) DeliverHighPriority(c *container.Container) *terminal.Error {
	// Set prioMsgs for prioritizing sending of messages until the high priority
	// one is sent.
	// Always add two extra for (1) the current message and (2) because prioMsgs
	// might be reduced immediately for sending the current message being handled
	// concurrently.
	if t.flowControl != nil {
		// Set prioMsgs of the relay operation to the full length of the terminal
		// recv queue, from which the relay operation reads and then sends.
		atomic.StoreInt32(
			t.op.prioMsgs,
			int32(t.flowControl.RecvQueueLen())+2,
		)
	} else {
		// Set prioMsgs of the relay operation to 2 if no flow control is configured.
		atomic.StoreInt32(t.op.prioMsgs, 2)
	}

	return t.deliverProxy(c)
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
	terminal.RegisterOpType(terminal.OpParams{
		Type:     ExpandOpType,
		Requires: terminal.MayExpand,
		RunOp:    expand,
	})
}

func expand(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
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
		opTerminal:  t,
		opts:        opts,
		dataRelayed: new(uint64),
		ended:       abool.New(),
		prioMsgs:    new(int32),
		relayTerminal: &ExpansionRelayTerminal{
			crane:     relayCrane,
			id:        relayCrane.getNextTerminalID(),
			abandoned: abool.New(),
			prioMsgs:  new(int32),
		},
	}
	op.OpBase.Init()
	op.OpBase.SetID(opID)
	op.ctx, op.cancelCtx = context.WithCancel(context.Background())
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
		deliverToOp := make(chan *container.Container, opts.FlowControlSize)
		op.deliverProxy = terminal.MakeDirectDeliveryDeliverFunc(op.ctx, deliverToOp)
		op.recvProxy = terminal.MakeDirectDeliveryRecvFunc(deliverToOp)
		op.sendProxy = op.submitBackstream
		// Relay Terminal
		deliverToRelay := make(chan *container.Container, opts.FlowControlSize)
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

func (op *ExpandOp) submitForwardstream(c *container.Container, highPriority bool) (err *terminal.Error) {
	if highPriority && op.opts.UsePriorityDataMsgs {
		terminal.MakeMsg(c, op.relayTerminal.id, terminal.MsgTypePriorityData)
		err = op.relayTerminal.crane.submitTerminalMsg(c, true)
	} else {
		terminal.MakeMsg(c, op.relayTerminal.id, terminal.MsgTypeData)
		err = op.relayTerminal.crane.submitTerminalMsg(c, false)
	}
	if err != nil {
		op.opTerminal.OpEnd(op, err.Wrap("failed to submit forward from relay op"))
	}
	return err
}

func (op *ExpandOp) submitBackstream(c *container.Container, highPriority bool) *terminal.Error {
	err := op.opTerminal.OpSend(op, c, 0, highPriority)
	if err != nil {
		op.opTerminal.OpEnd(op, err.Wrap("failed to submit backward from relay op"))
	}
	return err
}

func (op *ExpandOp) forwardHandler(_ context.Context) error {
	// Metrics setup and submitting.
	atomic.AddInt64(activeExpandOps, 1)
	started := time.Now()
	defer func() {
		atomic.AddInt64(activeExpandOps, -1)
		expandOpDurationHistogram.UpdateDuration(started)
		expandOpRelayedDataHistogram.Update(float64(atomic.LoadUint64(op.dataRelayed)))
	}()

	// Setup micro task done signal.
	microTaskDone := func() {}
	defer microTaskDone()

	for {
		// Signal that we've finished the micro task.
		microTaskDone()

		select {
		case c := <-op.recvProxy():
			// Debugging:
			// log.Debugf("spn/testing: forwarding at %s: %s", op.FmtID(), spew.Sdump(c.CompileData()))

			// Check if we should forward with high priority.
			remainingPrioMsgs := atomic.AddInt32(op.prioMsgs, -1)
			if remainingPrioMsgs >= 0 {
				microTaskDone = module.SignalHighPriorityMicroTask()
			} else {
				microTaskDone = module.SignalMicroTask(terminal.DefaultMediumPriorityMaxDelay)
				// Protect against wrapping.
				if remainingPrioMsgs < -2_000_000_000 {
					atomic.StoreInt32(op.prioMsgs, 0)
				}
			}

			// Count relayed data for metrics.
			atomic.AddUint64(op.dataRelayed, uint64(c.Length()))

			// Receive data from the origin and forward it to the relay.
			if err := op.relayTerminal.sendProxy(c, remainingPrioMsgs >= 0); err != nil {
				op.relayTerminal.Abandon(err)
				return nil
			}

		case <-op.ctx.Done():
			return nil
		}
	}
}

func (op *ExpandOp) backwardHandler(_ context.Context) error {
	// Setup micro task done signal.
	microTaskDone := func() {}
	defer microTaskDone()

	for {
		// Signal that we've finished the micro task.
		microTaskDone()

		select {
		case c := <-op.relayTerminal.recvProxy():
			// Debugging:
			// log.Debugf("spn/testing: backwarding at %s: %s", op.FmtID(), spew.Sdump(c.CompileData()))

			// Check if we should forward with high priority.
			remainingPrioMsgs := atomic.AddInt32(op.prioMsgs, -1)
			if remainingPrioMsgs >= 0 {
				microTaskDone = module.SignalHighPriorityMicroTask()
			} else {
				microTaskDone = module.SignalMicroTask(terminal.DefaultMediumPriorityMaxDelay)
				// Protect against wrapping.
				if remainingPrioMsgs < -2_000_000_000 {
					atomic.StoreInt32(op.prioMsgs, 0)
				}
			}

			// Count relayed data for metrics.
			atomic.AddUint64(op.dataRelayed, uint64(c.Length()))

			// Receive data from the relay and forward it to the origin.
			if err := op.sendProxy(c, remainingPrioMsgs >= 0); err != nil {
				op.Abandon(err)
				return nil
			}

		case <-op.ctx.Done():
			return nil
		}
	}
}

// Abandon abandons the expansion.
func (op *ExpandOp) Abandon(err *terminal.Error) {
	// Proxy for Terminal Interface needed for the Duplex Flow Queue.
	_ = op.End(err)
}

// End abandons the expansion.
func (op *ExpandOp) End(err *terminal.Error) (errorToSend *terminal.Error) {
	if op.ended.SetToIf(false, true) {
		// Init proper process.
		op.opTerminal.OpEnd(op, err)

		// Stop connected workers.
		op.cancelCtx()

		// Abandon connected terminal.
		op.relayTerminal.crane.AbandonTerminal(op.relayTerminal.id, nil)
	}
	return err
}

// Abandon abandons the expansion.
func (t *ExpansionRelayTerminal) Abandon(err *terminal.Error) {
	if t.abandoned.SetToIf(false, true) {
		// Init proper process.
		t.crane.AbandonTerminal(t.id, err)

		// End connected operation.
		_ = t.op.End(err.Wrap("relay failed with"))
	}
}

// FmtID returns the expansion ID hierarchy.
func (op *ExpandOp) FmtID() string {
	return fmt.Sprintf("%s>%d <r> %s#%d", op.opTerminal.FmtID(), op.ID(), op.relayTerminal.crane.ID, op.relayTerminal.id)
}

// FmtID returns the expansion ID hierarchy.
func (t *ExpansionRelayTerminal) FmtID() string {
	return fmt.Sprintf("%s#%d", t.crane.ID, t.id)
}
