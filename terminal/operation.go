package terminal

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/utils"
	"github.com/tevino/abool"
)

const (
	// DefaultOperationTimeout is the default time duration after which an idle
	// operation times out and is ended or regarded as failed.
	DefaultOperationTimeout = 10 * time.Second
)

type Operation interface {
	ID() uint32
	SetID(id uint32)
	Type() string
	Deliver(data *container.Container) *Error
	HasEnded(end bool) bool
	End(err *Error)
}

type OpParams struct {
	// Type is the type name of an operation.
	Type string
	// Requires defines the required permissions to run an operation.
	Requires Permission
	// RunOp is the function that start a new operation.
	RunOp OpRunner
}

type OpRunner func(t OpTerminal, opID uint32, initData *container.Container) (Operation, *Error)

var (
	opRegistry       = make(map[string]*OpParams)
	opRegistryLock   sync.Mutex
	opRegistryLocked = abool.New()
)

// RegisterOpType registeres a new operation type and may only be called during
// Go's init and a module's prep phase.
func RegisterOpType(params OpParams) {
	// Check if we can still register an operation type.
	if opRegistryLocked.IsSet() {
		log.Errorf("spn/terminal: failed to register operation %s: operation registry is already locked", params.Type)
		return
	}

	opRegistryLock.Lock()
	defer opRegistryLock.Unlock()

	// Check if the operation type was already registered.
	if _, ok := opRegistry[params.Type]; ok {
		log.Errorf("spn/terminal: failed to register operation type %s: type already registered", params.Type)
		return
	}

	// Save to registry.
	opRegistry[params.Type] = &params
}

func lockOpRegistry() {
	opRegistryLocked.Set()
}

func (t *TerminalBase) runOperation(ctx context.Context, opTerminal OpTerminal, opID uint32, initData *container.Container) {
	// Extract the requested operation name.
	opType, err := initData.GetNextBlock()
	if err != nil {
		t.OpEnd(newUnknownOp(opID, ""), ErrMalformedData.With("failed to get init data: %w", err))
		return
	}

	// Get the operation parameters from the registry.
	params, ok := opRegistry[string(opType)]
	if !ok {
		t.OpEnd(newUnknownOp(opID, ""), ErrUnknownOperationType.With(utils.SafeFirst16Bytes(opType)))
		return
	}

	// Check if the Terminal has the required permission to run the operation.
	if !t.HasPermission(params.Requires) {
		t.OpEnd(newUnknownOp(opID, params.Type), ErrPermissinDenied)
		return
	}

	// Run the operation.
	op, opErr := params.RunOp(opTerminal, opID, initData)
	switch {
	case opErr != nil:
		// Something went wrong.
		t.OpEnd(newUnknownOp(opID, params.Type), opErr)
	case op == nil:
		// The Operation was successful and is done already.
		log.Debugf("spn/terminal: operation %s %s executed", params.Type, fmtOperationID(t.parentID, t.id, opID))
		t.OpEnd(newUnknownOp(opID, params.Type), nil)
	default:
		// The operation started successfully and requires persistence.
		t.SetActiveOp(opID, op)
		log.Debugf("spn/terminal: operation %s %s started", params.Type, fmtOperationID(t.parentID, t.id, opID))
	}
}

// OpTerminal provides Operations with the necessary interface to interact with
// the Terminal.
type OpTerminal interface {
	// OpInit initialized the operation with the given data.
	OpInit(op Operation, data *container.Container) *Error

	// OpSend sends data.
	OpSend(op Operation, data *container.Container) *Error

	// OpSendWithTimeout sends data, but fails after the given timeout passed.
	OpSendWithTimeout(op Operation, data *container.Container, timeout time.Duration) *Error

	// OpEnd sends the end signal and calls End(ErrNil) on the Operation.
	// The Operation should cease operation after calling this function.
	OpEnd(op Operation, err *Error)

	// FmtID returns the formatted ID the Operation's Terminal.
	FmtID() string
}

// OpInit initialized the operation with the given data.
func (t *TerminalBase) OpInit(op Operation, data *container.Container) *Error {
	// Get next operation ID and set it on the operation.
	op.SetID(atomic.AddUint32(t.nextOpID, 8))

	// Always add operation to the active operations, as we need to receive a
	// reply in any case.
	t.SetActiveOp(op.ID(), op)

	log.Debugf("spn/terminal: operation %s %s started", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()))

	// Add or create the operation type block.
	if data == nil {
		data = container.New()
		data.AppendAsBlock([]byte(op.Type()))
	} else {
		data.PrependAsBlock([]byte(op.Type()))
	}

	return t.addToOpMsgSendBuffer(op.ID(), MsgTypeInit, data, 10*time.Second)
}

// OpSend sends data.
func (t *TerminalBase) OpSend(op Operation, data *container.Container) *Error {
	return t.addToOpMsgSendBuffer(op.ID(), MsgTypeData, data, 0)
}

// OpSendWithTimeout sends data, but fails after the given timeout passed.
func (t *TerminalBase) OpSendWithTimeout(op Operation, data *container.Container, timeout time.Duration) *Error {
	return t.addToOpMsgSendBuffer(op.ID(), MsgTypeData, data, timeout)
}

// OpEnd sends the end signal with an optional error and then deletes the
// operation from the Terminal state and calls End(ErrNil) on the Operation.
// The Operation should cease operation after calling this function.
// Should only be called by an operation.
func (t *TerminalBase) OpEnd(op Operation, err *Error) {
	// Check if the operation has already ended.
	if op.HasEnded(true) {
		return
	}

	// Log reason the Operation is ending. Override stopping error with nil.
	switch {
	case err == nil:
		log.Debugf("spn/terminal: operation %s %s ended", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()))
	case err.IsSpecial():
		log.Debugf("spn/terminal: operation %s %s ended: %s", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()), err)
	default:
		log.Warningf("spn/terminal: operation %s %s failed: %s", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()), err)
	}

	// Call operation end function for proper shutdown cleaning up.
	op.End(err)

	// Send error to the connected Operation, if the error is internal.
	if !err.IsExternal() {
		t.addToOpMsgSendBuffer(op.ID(), MsgTypeStop, container.New(err.Pack()), 0)
	}

	// Remove operation from terminal.
	t.DeleteActiveOp(op.ID())

	return
}

// GetActiveOp returns the active operation with the given ID from the
// Terminal state.
func (t *TerminalBase) GetActiveOp(opID uint32) (op Operation, ok bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	op, ok = t.operations[opID]
	return
}

// SetActiveOp saves an active operation to the Terminal state.
func (t *TerminalBase) SetActiveOp(opID uint32, op Operation) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.operations[opID] = op
}

// DeleteActiveOp deletes an active operation from the Terminal state and
// returns it.
func (t *TerminalBase) DeleteActiveOp(opID uint32) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.operations, opID)
}

func newUnknownOp(id uint32, opType string) *unknownOp {
	return &unknownOp{
		id:     id,
		opType: opType,
		ended:  abool.New(),
	}
}

type unknownOp struct {
	id     uint32
	opType string
	ended  *abool.AtomicBool
}

func (op *unknownOp) ID() uint32 {
	return op.id
}

func (op *unknownOp) SetID(id uint32) {
	op.id = id
}

func (op *unknownOp) Type() string {
	if op.opType != "" {
		return op.opType
	}
	return "unknown"
}

func (op *unknownOp) Deliver(data *container.Container) *Error {
	return ErrIncorrectUsage.With("unknown op shim cannot receive")
}

func (op *unknownOp) End(err *Error) {}

func (op *unknownOp) HasEnded(end bool) bool {
	if end {
		// Return false if we just only it to ended.
		return !op.ended.SetToIf(false, true)
	}
	return op.ended.IsSet()
}
