package terminal

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/safing/portbase/formats/varint"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/tevino/abool"
)

type Operation interface {
	ID() uint32
	SetID(id uint32)
	Type() string
	Deliver(data *container.Container) Error
	End(action string, err Error)
}

type OpBase struct {
	Params *OpParams
}

type OpParams struct {
	// Type is the type name of an operation.
	Type string
	// Requires defines the required permissions to run an operation.
	Requires Permission
	// RunOp is the function that start a new operation.
	RunOp OpRunner
}

type OpRunner func(t OpTerminal, opID uint32, initialData *container.Container) Operation

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

func (t *TerminalBase) runOperation(ctx context.Context, opTerminal OpTerminal, opID uint32, initialData *container.Container) {
	// Extract the requested operation name.
	opType, err := initialData.GetNextBlock()
	if err != nil {
		t.OpEnd(&unknownOp{id: opID}, "run op", ErrMalformedData)
		return
	}

	// Get the operation parameters from the registry.
	params, ok := opRegistry[string(opType)]
	if !ok {
		t.OpEnd(&unknownOp{id: opID}, "run op", ErrUnknownOperationType)
		return
	}

	// Check if the Terminal has the required permission to run the operation.
	if !t.HasPermission(params.Requires) {
		t.OpEnd(&unknownOp{
			id:     opID,
			opType: params.Type,
		}, "run op", ErrPermissinDenied)
		return
	}

	// Run the operation.
	// If the operation requires persistence, it will be returned.
	op := params.RunOp(opTerminal, opID, initialData)
	if op != nil {
		t.SetActiveOp(opID, op)
	}
}

// OpTerminal provides Operations with the necessary interface to interact with
// the Terminal.
type OpTerminal interface {
	// OpInit initialized the operation with the given data.
	OpInit(op Operation, data *container.Container) Error

	// OpSend sends data.
	OpSend(op Operation, data *container.Container) Error

	// OpEnd sends the end signal and calls End(ErrNil) on the Operation.
	// The Operation should cease operation after calling this function.
	OpEnd(op Operation, action string, err Error)
}

// OpInit initialized the operation with the given data.
func (t *TerminalBase) OpInit(op Operation, data *container.Container) Error {
	// Get next operation ID and set it on the operation.
	op.SetID(atomic.AddUint32(t.nextOpID, 2))

	// Always add operation to the active operations, as we need to receive a
	// reply in any case.
	t.SetActiveOp(op.ID(), op)

	// Add or create the operation type block.
	if data == nil {
		opTypeData := []byte(op.Type())
		data = container.New(
			varint.Pack64(uint64(len(opTypeData))),
			opTypeData,
		)
	} else {
		data.PrependAsBlock([]byte(op.Type()))
	}

	return t.addToOpMsgSendBuffer(op.ID(), MsgTypeInit, data, false)
}

// OpSend sends data.
func (t *TerminalBase) OpSend(op Operation, data *container.Container) Error {
	return t.addToOpMsgSendBuffer(op.ID(), MsgTypeData, data, false)
}

// OpEnd sends the end signal with an optional error and then deletes the
// operation from the Terminal state and calls End(ErrNil) on the Operation.
// The Operation should cease operation after calling this function.
func (t *TerminalBase) OpEnd(op Operation, action string, err Error) {
	log.Warningf("terminal: operation %s %s#%d>%d %s: %s", op.Type(), t.parentID, t.id, op.ID(), action, err)

	// Send error to connected Operation.
	t.addToOpMsgSendBuffer(op.ID(), MsgTypeEnd, container.New([]byte(err)), true)

	if op, ok := t.DeleteActiveOp(op.ID()); ok {
		// The operation reported an error, but the error is meant for the other
		// side, not the operation itself. Else, the operation would not be able
		// to differentiate if an error came from itself or the other side.
		op.End(action, ErrNil)
	}
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
func (t *TerminalBase) DeleteActiveOp(opID uint32) (op Operation, ok bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	op, ok = t.operations[opID]
	if ok {
		delete(t.operations, opID)
	}
	return
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

func (op unknownOp) Deliver(data *container.Container) Error {
	return ErrMalformedData
}

func (op unknownOp) End(action string, err Error) {}

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
