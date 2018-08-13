package identity

import (
	"github.com/tevino/abool"
)

type publishHookFunc func()

var (
	publishHook publishHookFunc
	hooksActive = abool.New()
)

// RegisterPublishHook allows the manager to be notified whenever the identity is updated and should be published.
func RegisterPublishHook(hook publishHookFunc) {
	if hook != nil {
		publishHook = hook
		hooksActive.Set()
	}
}

func RemoveConnection(portName string) {
	identityLock.Lock()
	defer identityLock.Unlock()
	if identity != nil {
		// TODO: this is a workaround for goimports importing this package
		myID := identity
		myID.RemoveConnection(portName)
		go UpdateIdentity(identity)
	}
}
