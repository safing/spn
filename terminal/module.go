package terminal

import (
	"fmt"

	"github.com/safing/portbase/modules"
	"github.com/safing/portbase/rng"
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
	return nil
}

func (t *TerminalBase) FmtID() string {
	return fmtTerminalID(t.parentID, t.id)
}

func fmtTerminalID(craneID string, terminalID uint32) string {
	return fmt.Sprintf("%s#%d", craneID, terminalID)
}

func fmtOperationID(craneID string, terminalID, operationID uint32) string {
	return fmt.Sprintf("%s#%d>%d", craneID, terminalID, operationID)
}
