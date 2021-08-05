package terminal

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
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
		log.Errorf("terminal: failed to register operation %s: operation registry is already locked", params.Type)
		return
	}

	opRegistryLock.Lock()
	defer opRegistryLock.Unlock()

	// Check if the operation type was already registered.
	if _, ok := opRegistry[params.Type]; ok {
		log.Errorf("terminal: failed to register operation type %s: type already registered", params.Type)
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
		t.OpEnd(&unknownOp{id: opID}, ErrMalformedData.With("failed to get init data: %w", err))
		return
	}

	// Get the operation parameters from the registry.
	params, ok := opRegistry[string(opType)]
	if !ok {
		t.OpEnd(&unknownOp{id: opID}, ErrUnknownOperationType)
		return
	}

	// Check if the Terminal has the required permission to run the operation.
	if !t.HasPermission(params.Requires) {
		t.OpEnd(&unknownOp{
			id:     opID,
			opType: params.Type,
		}, ErrPermissinDenied)
		return
	}

	// Run the operation.
	op, opErr := params.RunOp(opTerminal, opID, initData)
	if opErr != nil {
		t.OpEnd(&unknownOp{
			id:     opID,
			opType: params.Type,
		}, opErr)
		return
	}

	if op != nil {
		// If the operation started successfully and requires persistence, add it to the active ops.
		t.SetActiveOp(opID, op)
		log.Debugf("terminal: operation %s %s started", params.Type, fmtOperationID(t.parentID, t.id, opID))
	} else {
		// If the operation was a just single function call, log that it was executed.
		log.Debugf("terminal: operation %s %s executed", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()))
	}
}

// OpTerminal provides Operations with the necessary interface to interact with
// the Terminal.
type OpTerminal interface {
	// OpInit initialized the operation with the given data.
	OpInit(op Operation, data *container.Container) *Error

	// OpSend sends data.
	OpSend(op Operation, data *container.Container) *Error

	// OpEnd sends the end signal and calls End(ErrNil) on the Operation.
	// The Operation should cease operation after calling this function.
	OpEnd(op Operation, err *Error)

	// FmtID returns the formatted ID the Operation's Terminal.
	FmtID() string
}

// OpInit initialized the operation with the given data.
func (t *TerminalBase) OpInit(op Operation, data *container.Container) *Error {
	// Get next operation ID and set it on the operation.
	op.SetID(atomic.AddUint32(t.nextOpID, 2))

	// Always add operation to the active operations, as we need to receive a
	// reply in any case.
	t.SetActiveOp(op.ID(), op)

	log.Debugf("terminal: operation %s %s started", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()))

	// Add or create the operation type block.
	if data == nil {
		data = container.New()
		data.AppendAsBlock([]byte(op.Type()))
	} else {
		data.PrependAsBlock([]byte(op.Type()))
	}

	return t.addToOpMsgSendBuffer(op.ID(), MsgTypeInit, data, false)
}

// OpSend sends data.
func (t *TerminalBase) OpSend(op Operation, data *container.Container) *Error {
	return t.addToOpMsgSendBuffer(op.ID(), MsgTypeData, data, false)
}

// OpEnd sends the end signal with an optional error and then deletes the
// operation from the Terminal state and calls End(ErrNil) on the Operation.
// The Operation should cease operation after calling this function.
// Should only be called by an operation.
func (t *TerminalBase) OpEnd(op Operation, err *Error) {
	if err == nil {
		log.Debugf("terminal: operation %s %s ended", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()))
	} else {
		log.Warningf("terminal: operation %s %s: %s", op.Type(), fmtOperationID(t.parentID, t.id, op.ID()), err)
	}

	if !err.IsExternal() {
		// Send error to connected Operation.
		t.addToOpMsgSendBuffer(op.ID(), MsgTypeEnd, container.New(err.Pack()), true)
	}

	// Call operation end function.
	op.End(err)

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

type unknownOp struct {
	id     uint32
	opType string
}

func (op unknownOp) ID() uint32 {
	return op.id
}

func (op unknownOp) SetID(id uint32) {
	op.id = id
}

func (op unknownOp) Type() string {
	if op.opType != "" {
		return op.opType
	}
	return "unknown"
}

func (op unknownOp) Deliver(data *container.Container) *Error {
	return ErrMalformedData.With("unknown op shim cannot receive")
}

func (op unknownOp) End(err *Error) {}

// Terminal Message Types.
// FIXME: Delete after commands are implemented.
const (
	// Informational
	TerminalCmdInfo          uint8 = 1
	TerminalCmdLoad          uint8 = 2
	TerminalCmdStats         uint8 = 3
	TerminalCmdPublicHubFeed uint8 = 4

	// Diagnostics
	TerminalCmdEcho      uint8 = 16
	TerminalCmdSpeedtest uint8 = 17

	// User Access
	TerminalCmdUserAuth uint8 = 32

	// Tunneling
	TerminalCmdHop    uint8 = 40
	TerminalCmdTunnel uint8 = 41
	TerminalCmdPing   uint8 = 42

	// Admin/Mod Access
	TerminalCmdAdminAuth uint8 = 128

	// Mgmt
	TerminalCmdEstablishRoute uint8 = 144
	TerminalCmdShutdown       uint8 = 145
)
