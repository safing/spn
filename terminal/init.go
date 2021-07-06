package terminal

import (
	"context"

	"github.com/safing/portbase/formats/varint"

	"github.com/safing/jess"
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/log"
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
		- TerminalInitMsg as DSD

*/

const (
	minSupportedTerminalVersion = 1
	maxSupportedTerminalVersion = 1
)

// TerminalInitMsg holds initialization data for the terminal.
type TerminalInitMsg struct {
	Version   uint8  `json:"-"`
	QueueSize uint16 `json:"qs,omitempty"`
}

func NewLocalBaseTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	remoteHub *hub.Hub,
	initMsg *TerminalInitMsg,
) (
	t *TerminalBase,
	initData *container.Container,
	tErr Error,
) {
	var flags uint8
	var initMsgData *container.Container

	// Create baseline.
	t = createTerminalBase(ctx, id, parentID, false, initMsg)

	// Set default version.
	if initMsg.Version == 0 {
		initMsg.Version = 1
	}
	if initMsg.QueueSize == 0 {
		initMsg.QueueSize = 100
	}

	// Pack init message.
	packedInitMsg, err := dsd.Dump(initMsg, dsd.JSON)
	if err != nil {
		log.Warningf("spn/terminal: failed to pack init message: %s", err)
		return nil, nil, ErrInternalError
	}

	// Use encryption if enabled.
	if remoteHub != nil {
		flags |= 0x01

		// Select signet (public key) of remote Hub to use.
		s := remoteHub.SelectSignet()
		if s == nil {
			log.Warning("spn/terminal: failed to select signet of remote hub")
			return nil, nil, ErrInvalidConfiguration
		}

		// Create new session.
		env := jess.NewUnconfiguredEnvelope()
		env.SuiteID = jess.SuiteWireV1
		env.Recipients = []*jess.Signet{s}
		jession, err := env.WireCorrespondence(nil)
		if err != nil {
			log.Warningf("spn/terminal: failed to initialize encryption: %s", err)
			return nil, nil, ErrIntegrity
		}
		t.jession = jession

		// Encrypt init message.
		letter, err := jession.Close(packedInitMsg)
		if err != nil {
			log.Warningf("spn/terminal: failed to encrypt init message: %s", err)
			return nil, nil, ErrIntegrity
		}
		initMsgData, err = letter.ToWire()
		if err != nil {
			log.Warningf("spn/terminal: failed to pack encryption letter: %s", err)
			return nil, nil, ErrInternalError
		}

	} else {
		initMsgData = container.New(packedInitMsg)
	}

	// Compile init message.
	initData = container.New(
		varint.Pack8(initMsg.Version),
		varint.Pack8(flags),
	)
	initData.AppendContainer(initMsgData)

	return t, initData, ErrNil
}

func NewRemoteBaseTerminal(
	ctx context.Context,
	id uint32,
	parentID string,
	identity *cabin.Identity,
	initData *container.Container,
) (
	t *TerminalBase,
	initMsg *TerminalInitMsg,
	tErr Error,
) {
	var initMsgData []byte

	// Parse and check version.
	version, err := initData.GetNextN8()
	if err != nil {
		log.Warningf("spn/terminal: failed to parse version: %s", err)
		return nil, nil, ErrMalformedData
	}
	if version < minSupportedTerminalVersion || version > maxSupportedTerminalVersion {
		log.Warningf("spn/terminal: unsupprted terminal version requested: %d", version)
		return nil, nil, ErrUnsupportedTerminalVersion
	}

	// Parse flags.
	flags, err := initData.GetNextN8()
	if err != nil {
		log.Warningf("spn/terminal: failed to parse flags: %s", err)
		return nil, nil, ErrMalformedData
	}

	// Use encryption if enabled.
	var jession *jess.Session
	if flags&0x01 == 0x01 {
		if identity == nil {
			log.Warning("spn/terminal: missing identity for setting up incoming encryption")
			return nil, nil, ErrInternalError
		}

		// Initialize encryption.
		letter, err := jess.LetterFromWire(initData)
		if err != nil {
			log.Warningf("failed to parse encryption letter: %s", err)
			return nil, nil, ErrMalformedData
		}
		jession, err := letter.WireCorrespondence(identity)
		if err != nil {
			log.Warningf("failed to initialize encryption: %s", err)
			return nil, nil, ErrIntegrity
		}

		// Open init message.
		data, err := jession.Open(letter)
		if err != nil {
			log.Warningf("failed to decrypt init message: %s", err)
			return nil, nil, ErrIntegrity
		}
		initMsgData = data
	} else {
		initMsgData = initData.CompileData()
	}

	// Parse init message.
	initMsg = &TerminalInitMsg{}
	_, err = dsd.Load(initMsgData, initMsg)
	if err != nil {
		log.Warningf("failed to parse init message: %s", err)
		return nil, nil, ErrMalformedData
	}
	initMsg.Version = version

	// Check boundaries.
	if initMsg.QueueSize <= 0 || initMsg.QueueSize > 100 {
		log.Warning("spn/terminal: invalid queue size")
		return nil, nil, ErrInvalidConfiguration
	}

	// Create baseline.
	t = createTerminalBase(ctx, id, parentID, true, initMsg)
	t.jession = jession

	return t, initMsg, ErrNil
}
