package terminal

type Permission uint16

const (
	MayExpand         Permission = 0x1
	MayTunnel         Permission = 0x2
	IsHubOwner        Permission = 0x100
	IsHubAdvisor      Permission = 0x200
	IsCraneController Permission = 0x8000
)

// GrandPermission grants the specified permissions to the Terminal.
func (t *TerminalBase) GrantPermission(grant Permission) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.permission = t.permission | grant
}

// HasPermission returns if the Terminal has the specified permissions.
func (t *TerminalBase) HasPermission(required Permission) bool {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.permission&required == required
}
