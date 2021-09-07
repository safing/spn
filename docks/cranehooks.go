package docks

import (
	"github.com/safing/portbase/log"
	"github.com/tevino/abool"
)

var (
	craneUpdateHook        func(crane *Crane)
	craneUpdateHookEnabled = abool.New()
	craneUpdateHookActive  = abool.New()
)

// RegisterCraneUpdateHook allows the captain to hook into receiving updates for cranes.
func RegisterCraneUpdateHook(fn func(crane *Crane)) {
	if craneUpdateHookEnabled.SetToIf(false, true) {
		craneUpdateHook = fn
		craneUpdateHookActive.Set()
	} else {
		log.Error("spn/docks: crane update hook already registered")
	}
}

// NotifyUpdate calls the registers crane update hook function.
func (crane *Crane) NotifyUpdate() {
	if craneUpdateHookActive.IsSet() {
		craneUpdateHook(crane)
	}
}
