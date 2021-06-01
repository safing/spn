package terminal

import (
	"context"
	"sync"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/tevino/abool"
)

type TerminalCmd struct {
	ID       uint8
	Name     string
	Requires TerminalPermission
	Assign   AssignmentFactory
}

var (
	cmds       map[uint8]*TerminalCmd
	cmdsLock   sync.Mutex
	cmdsLocked = abool.New()
)

// RegisterCmd registeres a new command and may only be called during Go's
// init and module prep phase.
func RegisterCmd(tc *TerminalCmd) {
	// Check if we can still register a command.
	if cmdsLocked.IsSet() {
		log.Errorf("terminal: failed to register %q command: command registry is already locked", tc.Name)
		return
	}

	cmdsLock.Lock()
	defer cmdsLock.Unlock()

	// Check if a command with the same ID was already registered.
	if _, ok := cmds[tc.ID]; ok {
		log.Errorf("terminal: failed to register %q command: ID %d already taken", tc.Name, tc.ID)
		return
	}

	// Save to registry.
	cmds[tc.ID] = tc
}

func lockCmds() {
	cmdsLocked.Set()
}

func (t *Terminal) runCmd(ctx context.Context, assignmentID uint32, initialData *container.Container) {
	// Extract the requested command ID.
	cmdID, err := initialData.GetNextN8()
	if err != nil {
		t.SendError(assignmentID, ErrMalformedData)
		return
	}

	// Get the command from the registry.
	cmd, ok := cmds[cmdID]
	if !ok {
		t.SendError(assignmentID, ErrUnknownCommand)
		return
	}

	// Check if the Terminal has the required permission to access the command.
	if !t.Permission.Has(cmd.Requires) {
		t.SendError(assignmentID, ErrPermissinDenied)
		return
	}

	cmd.Assign(ctx, t, assignmentID, initialData)
}

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
