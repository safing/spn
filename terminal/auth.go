package terminal

type TerminalPermission uint16

const (
	MayExpand    TerminalPermission = 0x1
	MayTunnel    TerminalPermission = 0x2
	IsHubOwner   TerminalPermission = 0x100
	IsHubAdvisor TerminalPermission = 0x200
)

// Has returns if the supplied requiredPermissions are fulfilled.
func (tp TerminalPermission) Has(requiredPermission TerminalPermission) bool {
	return tp&requiredPermission == requiredPermission
}

func (t *Terminal) grantPermission(tp TerminalPermission) {
	t.Permission = t.Permission | tp
}
