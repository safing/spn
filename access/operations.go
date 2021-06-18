package access

import (
	"github.com/safing/spn/terminal"
)

const (
	OpNameAccessCode = "access.code"

	// 
	zonePermissions = map[string]terminal.Permission
)

func init() {
	terminal.RegisterOperation(terminal.OpParams{
		Name:  OpNameAuth,
		RunOp: func(*Terminal, uint32, *container.Container) Operation { panic("not implemented") },
	})
}

func prepProductionAccessCodes() error {



	terminal.RegisterOperation(terminal.OpParams{
		Name:  OpNameAuth,
		RunOp: checkAccessCode,
	})
}

func checkAccessCode(t *Terminal, opID uint32, opParams *OpParams, initialData *container.Container) Operation {
	code, err := access.ParseRawCode(initialData.CompileData())
}
