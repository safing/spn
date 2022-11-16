package terminal

import (
	"fmt"

	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/unit"
)

var (
	module    *modules.Module
	rngFeeder *rng.Feeder = rng.NewFeeder()
)

func init() {
	module = modules.Register("terminal", nil, start, nil, "base")
}

func start() error {
	rngFeeder = rng.NewFeeder()

	scheduler = unit.NewScheduler(nil)
	module.StartServiceWorker("msg unit scheduler", 0, scheduler.SlotScheduler)

	lockOpRegistry()
	return nil
}

// FmtID formats the terminal ID together with the parent's ID.
func (t *TerminalBase) FmtID() string {
	return fmtTerminalID(t.parentID, t.id)
}

func fmtTerminalID(craneID string, terminalID uint32) string {
	return fmt.Sprintf("%s#%d", craneID, terminalID)
}

func fmtOperationID(craneID string, terminalID, operationID uint32) string {
	return fmt.Sprintf("%s#%d>%d", craneID, terminalID, operationID)
}
