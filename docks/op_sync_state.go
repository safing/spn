package docks

import (
	"context"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/spn/terminal"
)

const (
	SyncStateOpType = "sync/state"
)

type SyncStateOp struct {
	terminal.OpBase
	t      terminal.OpTerminal
	result chan *terminal.Error
}

type SyncStateMessage struct {
	Stopping bool
}

func (op *SyncStateOp) Type() string {
	return SyncStateOpType
}

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:     SyncStateOpType,
		Requires: terminal.IsCraneController,
		RunOp:    runSyncStateOp,
	})
}

func (controller *CraneControllerTerminal) SyncState(ctx context.Context) *terminal.Error {
	// Check if we own the crane and it is public.
	if !controller.Crane.IsMine() || !controller.Crane.Public() {
		return nil
	}

	// Create and init.
	op := &SyncStateOp{
		t:      controller,
		result: make(chan *terminal.Error, 1),
	}
	op.OpBase.Init()

	// Create sync message.
	msg := &SyncStateMessage{
		Stopping: controller.Crane.stopping.IsSet(),
	}
	data, err := dsd.Dump(msg, dsd.CBOR)
	if err != nil {
		return terminal.ErrInternalError.With("%w", err)
	}

	// Send message.
	tErr := controller.OpInit(op, container.New(data))
	if tErr != nil {
		return tErr
	}

	// Wait for reply
	select {
	case tErr = <-op.result:
		if tErr.IsError() {
			return tErr
		}
		return nil
	case <-ctx.Done():
		return nil
	case <-time.After(1 * time.Minute):
		return terminal.ErrTimeout.With("timed out while waiting for sync crane result")
	}
}

func runSyncStateOp(t terminal.OpTerminal, opID uint32, data *container.Container) (terminal.Operation, *terminal.Error) {
	// Check if we are a on a crane controller.
	var ok bool
	var controller *CraneControllerTerminal
	if controller, ok = t.(*CraneControllerTerminal); !ok {
		return nil, terminal.ErrIncorrectUsage.With("can only be used with a crane controller")
	}

	// Check if we don't own the crane, but it is public.
	if controller.Crane.IsMine() || !controller.Crane.Public() {
		return nil, terminal.ErrPermissinDenied.With("only public lane owner may change the crane status")
	}

	// Load message.
	syncState := &SyncStateMessage{}
	_, err := dsd.Load(data.CompileData(), syncState)
	if err != nil {
		return nil, terminal.ErrMalformedData.With("failed to load sync state message: %w", err)
	}

	// Apply sync state.
	var changed bool
	if syncState.Stopping {
		if controller.Crane.stopping.SetToIf(false, true) {
			changed = true
		}
	} else {
		if controller.Crane.stopping.SetToIf(true, false) {
			changed = true
		}
	}

	// Notify of change.
	if changed {
		controller.Crane.NotifyUpdate()
	}

	return nil, nil
}

func (op *SyncStateOp) Deliver(c *container.Container) *terminal.Error {
	return terminal.ErrIncorrectUsage
}

func (op *SyncStateOp) End(tErr *terminal.Error) {
	if op.result != nil {
		select {
		case op.result <- tErr:
		default:
		}
	}
}
