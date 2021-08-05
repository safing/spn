package terminal

import (
	"context"

	"github.com/safing/portbase/formats/varint"

	"github.com/safing/jess"
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/hub"
)

/*

Terminal Init Message Format:

- Version [varint]
- Flags [varint]
	- 0x01 - Encrypted
- Data Block [bytes; not blocked]
	- Letter (if Encrypted Flag is set)
		- TerminalOpts as DSD

*/

const (
	minSupportedTerminalVersion = 1
	maxSupportedTerminalVersion = 1
)

// TerminalOpts holds configuration for the terminal.
type TerminalOpts struct {
	Version   uint8  `json:"-"`
	QueueSize uint16 `json:"qs,omitempty"`
	Padding   uint16 `json:"p,omitempty"`
	Encrypt   bool   `json:"e,omitempty"`
}

func ParseTerminalOpts(c *container.Container) (*TerminalOpts, *Error) {
	// Parse and check version.
	version, err := c.GetNextN8()
	if err != nil {
		return nil, ErrMalformedData.With("failed to parse version: %w", err)
	}
	if version < minSupportedTerminalVersion || version > maxSupportedTerminalVersion {
		return nil, ErrUnsupportedTerminalVersion.With("requested %d", version)
	}

	// Parse init message.
	initMsg := &TerminalOpts{}
	_, err = dsd.Load(c.CompileData(), initMsg)
	if err != nil {
		return nil, ErrMalformedData.With("failed to parse init message: %w", err)
	}
	initMsg.Version = version

	return initMsg, nil
}

func (opts *TerminalOpts) Pack() (*container.Container, *Error) {
	// Pack init message.
	optsData, err := dsd.Dump(opts, dsd.JSON)
	if err != nil {
		return nil, ErrInternalError.With("failed to parse init message: %w", err)
	}

	// Compile init message.
	return container.New(
		varint.Pack8(opts.Version),
		optsData,
	), nil
}

func NewLocalBaseTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	remoteHub *hub.Hub,
	initMsg *TerminalOpts,
) (
	t *TerminalBase,
	initData *container.Container,
	err *Error,
) {
	// Create baseline.
	t = createTerminalBase(ctx, id, parentID, false, initMsg)

	// Set default values.
	if initMsg.Version == 0 {
		initMsg.Version = 1
	}
	if initMsg.QueueSize == 0 {
		initMsg.QueueSize = 100
	}

	// Setup encryption if enabled.
	if remoteHub != nil {
		initMsg.Encrypt = true

		// Select signet (public key) of remote Hub to use.
		s := remoteHub.SelectSignet()
		if s == nil {
			return nil, nil, ErrHubNotReady.With("failed to select signet of remote hub")
		}

		// Create new session.
		env := jess.NewUnconfiguredEnvelope()
		env.SuiteID = jess.SuiteWireV1
		env.Recipients = []*jess.Signet{s}
		jession, err := env.WireCorrespondence(nil)
		if err != nil {
			return nil, nil, ErrIntegrity.With("failed to initialize encryption: %w", err)
		}
		t.jession = jession
	}

	// Pack init message.
	initData, err = initMsg.Pack()
	if err != nil {
		return nil, nil, err
	}

	return t, initData, nil
}

func NewRemoteBaseTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	identity *cabin.Identity,
	initData *container.Container,
) (
	t *TerminalBase,
	initMsg *TerminalOpts,
	err *Error,
) {
	// Parse init message.
	initMsg, err = ParseTerminalOpts(initData)
	if err != nil {
		return nil, nil, err
	}

	// Check boundaries.
	if initMsg.QueueSize <= 0 || initMsg.QueueSize > 100 {
		return nil, nil, ErrInvalidOptions.With("invalid queue size of %d", initMsg.QueueSize)
	}

	// Create baseline.
	t = createTerminalBase(ctx, id, parentID, true, initMsg)

	// Setup encryption if enabled.
	if initMsg.Encrypt {
		if identity == nil {
			return nil, nil, ErrInternalError.With("missing identity for setting up incoming encryption")
		}
		t.identity = identity
	}

	return t, initMsg, nil
}
