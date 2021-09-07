package docks

import (
	"github.com/safing/portbase/container"
	"github.com/safing/spn/terminal"
)

type CraneControllerTerminal struct {
	*terminal.TerminalBase
	*terminal.DuplexFlowQueue

	Crane *Crane
}

func NewLocalCraneControllerTerminal(
	crane *Crane,
	initMsg *terminal.TerminalOpts,
) (*CraneControllerTerminal, *container.Container, *terminal.Error) {
	// Create Terminal Base.
	t, initData, err := terminal.NewLocalBaseTerminal(crane.ctx, 0, crane.ID, nil, initMsg)
	if err != nil {
		return nil, nil, err
	}

	return initCraneController(crane, t, initMsg), initData, nil
}

func NewRemoteCraneControllerTerminal(
	crane *Crane,
	initData *container.Container,
) (*CraneControllerTerminal, *terminal.TerminalOpts, *terminal.Error) {
	// Create Terminal Base.
	t, initMsg, err := terminal.NewRemoteBaseTerminal(crane.ctx, 0, crane.ID, nil, initData)
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
	// Create Flow Queue.
	dfq := terminal.NewDuplexFlowQueue(t, initMsg.QueueSize, t.SubmitAsDataMsg(crane.submitImportantTerminalMsg))

	// Create Crane Terminal and assign it as the extended Terminal.
	cct := &CraneControllerTerminal{
		TerminalBase:    t,
		DuplexFlowQueue: dfq,
		Crane:           crane,
	}
	t.SetTerminalExtension(cct)

	// Assign controller to crane.
	crane.Controller = cct
	crane.terminals[cct.ID()] = cct

	// Copy the options to the crane itself.
	crane.opts = *initMsg

	// Remove unnecessary options from the crane controller.
	initMsg.Padding = 0

	// Grant crane controller permission.
	t.GrantPermission(terminal.IsCraneController)

	// Start workers.
	module.StartWorker("crane controller terminal handler", cct.Handler)
	module.StartWorker("crane controller terminal sender", cct.Sender)
	module.StartWorker("crane controller terminal flow queue", cct.FlowHandler)

	return cct
}

func (controller *CraneControllerTerminal) Deliver(c *container.Container) *terminal.Error {
	return controller.DuplexFlowQueue.Deliver(c)
}

func (controller *CraneControllerTerminal) Abandon(err *terminal.Error) {
	if controller.Abandoned.SetToIf(false, true) {
		// Send stop msg and end all operations.
		controller.Shutdown(err)

		// Abandon terminal.
		controller.Crane.AbandonTerminal(0, err)

		// Stop controlled crane.
		controller.Crane.Stop(nil)
	}
}
