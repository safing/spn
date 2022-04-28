package docks

import (
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/terminal"
)

// CraneControllerTerminal is a terminal for the crane itself.
type CraneControllerTerminal struct {
	*terminal.TerminalBase

	Crane *Crane
}

// NewLocalCraneControllerTerminal returns a new local crane controller.
func NewLocalCraneControllerTerminal(
	crane *Crane,
	initMsg *terminal.TerminalOpts,
) (*CraneControllerTerminal, *container.Container, *terminal.Error) {
	// Remove unnecessary options from the crane controller.
	initMsg.Padding = 0

	// Create Terminal Base.
	t, initData, err := terminal.NewLocalBaseTerminal(
		crane.ctx,
		0,
		crane.ID,
		nil,
		initMsg,
		crane.submitImportantTerminalMsg,
		true,
	)
	if err != nil {
		return nil, nil, err
	}

	return initCraneController(crane, t, initMsg), initData, nil
}

// NewRemoteCraneControllerTerminal returns a new remote crane controller.
func NewRemoteCraneControllerTerminal(
	crane *Crane,
	initData *container.Container,
) (*CraneControllerTerminal, *terminal.TerminalOpts, *terminal.Error) {
	// Create Terminal Base.
	t, initMsg, err := terminal.NewRemoteBaseTerminal(
		crane.ctx,
		0,
		crane.ID,
		nil,
		initData,
		crane.submitImportantTerminalMsg,
		true,
	)
	if err != nil {
		return nil, nil, err
	}

	return initCraneController(crane, t, initMsg), initMsg, nil
}

func initCraneController(
	crane *Crane,
	t *terminal.TerminalBase,
	initMsg *terminal.TerminalOpts,
) *CraneControllerTerminal {
	// Create Crane Terminal and assign it as the extended Terminal.
	cct := &CraneControllerTerminal{
		TerminalBase: t,
		Crane:        crane,
	}
	t.SetTerminalExtension(cct)

	// Assign controller to crane.
	crane.Controller = cct
	crane.terminals[cct.ID()] = cct

	// Copy the options to the crane itself.
	crane.opts = *initMsg

	// Grant crane controller permission.
	t.GrantPermission(terminal.IsCraneController)

	// Start workers.
	t.StartWorkers(module, "crane controller terminal")

	return cct
}

// Abandon abandons the crane controller.
func (controller *CraneControllerTerminal) Abandon(err *terminal.Error) {
	if controller.Abandoning.SetToIf(false, true) {
		// Send stop msg and end all operations.
		controller.StartAbandonProcedure(err, false, func() {
			// Send error manually, as terminal base packs it into another data msg.
			// TODO: Send via terminal again when DFQ is merged.
			if !err.IsExternal() {
				stopMsg := container.New(err.Pack())
				terminal.MakeMsg(stopMsg, controller.ID(), terminal.MsgTypeStop)
				err := controller.Crane.submitImportantTerminalMsg(stopMsg)
				if err != nil {
					log.Warningf("spn/docks: %s controller failed to submit stop msg: %s", controller.Crane, err)
				}
			}

			// Abandon terminal.
			controller.Crane.AbandonTerminal(0, err)

			// Stop controlled crane.
			controller.Crane.Stop(nil)
		})
	}
}
