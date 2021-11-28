package access

import (
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/access/token"
	"github.com/safing/spn/terminal"
)

const (
	OpTypeAccessCodeAuth = "auth"
)

func init() {
	terminal.RegisterOpType(terminal.OpParams{
		Type:  OpTypeAccessCodeAuth,
		RunOp: checkAccessCode,
	})
}

type AuthorizeOp struct {
	terminal.OpBaseRequest
}

func (op *AuthorizeOp) Type() string {
	return OpTypeAccessCodeAuth
}

func AuthorizeToTerminal(t terminal.OpTerminal) (*AuthorizeOp, *terminal.Error) {
	op := &AuthorizeOp{}
	op.Init(0)

	newToken, err := GetToken(ExpandAndConnectZones)
	if err != nil {
		return nil, terminal.ErrInternalError.With("failed to get access token: %w", err)
	}

	tErr := t.OpInit(op, container.New(newToken.Raw()))
	if tErr != nil {
		return nil, terminal.ErrInternalError.With("failed to init auth op: %w", tErr)
	}

	return op, nil
}

func checkAccessCode(t terminal.OpTerminal, opID uint32, initData *container.Container) (terminal.Operation, *terminal.Error) {
	// Parse provided access token.
	receivedToken, err := token.ParseRawToken(initData.CompileData())
	if err != nil {
		return nil, terminal.ErrMalformedData.With("failed to parse access token: %w", err)
	}

	// Check if token is valid.
	granted, err := VerifyToken(receivedToken)
	if err != nil {
		return nil, terminal.ErrPermissinDenied.With("invalid access token: %w", err)
	}

	// Get the authorizing terminal for applying the granted permission.
	authTerm, ok := t.(terminal.AuthorizingTerminal)
	if !ok {
		return nil, terminal.ErrIncorrectUsage.With("terminal does not handle authorization")
	}

	// Grant permissions.
	authTerm.GrantPermission(granted)
	log.Debugf("spn/access: granted %s permissions via %s zone", t.FmtID(), receivedToken.Zone)

	// End successfully.
	return nil, terminal.ErrExplicitAck
}
