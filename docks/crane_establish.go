package docks

import (
	"context"

	"github.com/safing/portbase/log"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/terminal"
)

func (crane *Crane) EstablishNewTerminal(
	localTerm terminal.TerminalInterface,
	initData *container.Container,
) *terminal.Error {
	// Prepend header.
	terminal.MakeMsg(initData, localTerm.ID(), terminal.MsgTypeInit)

	// Register terminal with crane.
	crane.setTerminal(localTerm)

	// Send message.
	select {
	case crane.importantMsgs <- initData:
		log.Debugf("spn/docks: %s initiated new terminal %d", crane, localTerm.ID())
		return nil
	case <-crane.ctx.Done():
		crane.AbandonTerminal(localTerm.ID(), terminal.ErrStopping.With("initation aborted"))
		return terminal.ErrStopping
	}
}

func (crane *Crane) establishTerminal(id uint32, initData *container.Container) {
	// Create new remote crane terminal.
	newTerminal, _, err := NewRemoteCraneTerminal(
		crane,
		id,
		initData,
	)
	if err == nil {
		// Register terminal with crane.
		crane.setTerminal(newTerminal)
		log.Debugf("spn/docks: %s established new crane terminal %d", crane, newTerminal.ID())
		return
	}

	// If something goes wrong, send an error back.
	log.Warningf("spn/docks: %s failed to establish crane terminal: %s", crane, err)

	// Build abandon message.
	abandonMsg := container.New(err.Pack())
	terminal.MakeMsg(abandonMsg, id, terminal.MsgTypeStop)

	// Send message directly, or async.
	select {
	case crane.terminalMsgs <- abandonMsg:
	default:
		// Send error async.
		module.StartWorker("abandon terminal", func(ctx context.Context) error {
			select {
			case crane.terminalMsgs <- abandonMsg:
			case <-ctx.Done():
			}
			return nil
		})
	}
}
