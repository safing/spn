package docks

import (
	"errors"
	"fmt"

	"github.com/safing/jess"
	"github.com/safing/spn/cabin"

	"github.com/safing/portbase/container"
	"github.com/safing/spn/hub"
)

type EncryptionConveyor struct {
	ConveyorBase

	version int
	id      *cabin.Identity
	dst     *hub.Hub
	session *jess.Session
}

func NewEncryptionConveyor(version int, id *cabin.Identity, dst *hub.Hub) (*EncryptionConveyor, error) {
	return &EncryptionConveyor{
		version: version,
		id:      id,
		dst:     dst,
	}, nil
}

func (ec *EncryptionConveyor) setupIncoming(letter *jess.Letter) (err error) {
	if ec.id == nil {
		return errors.New("missing identity for setting up incoming encryption")
	}

	ec.session, err = letter.WireCorrespondence(ec.id)
	return err
}

func (ec *EncryptionConveyor) setupOutgoing() error {
	if ec.dst == nil {
		return errors.New("missing destination Hub")
	}

	// select a public key of connected hub
	s, err := ec.dst.SelectSignet()
	if err != nil {
		return fmt.Errorf("failed to select signet: %w", err)
	}

	// create envelope
	env := jess.NewUnconfiguredEnvelope()
	env.SuiteID = jess.SuiteWireV1
	env.Recipients = []*jess.Signet{s}

	// do not encrypt directly, rather get session for future use, then encrypt
	ec.session, err = env.WireCorrespondence(nil)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

func (ec *EncryptionConveyor) Run() {
	for {
		select {
		case c := <-ec.fromShore:

			// silent fail
			if c == nil {
				return
			}

			// init
			if ec.session == nil {
				err := ec.setupOutgoing()
				if err != nil {
					c.SetError(err)
					ec.toShip <- c
					ec.toShore <- c
					return
				}
			}

			// encrypt (even if it's an error)
			letter, err := ec.session.Close(c.CompileData())
			if err != nil {
				c.SetError(err)
				ec.toShip <- c
				ec.toShore <- c
				return
			}

			// pack
			encrypted, err := letter.ToWire()
			if err != nil {
				c.SetError(err)
				ec.toShip <- c
				ec.toShore <- c
				return
			}

			// send on its way
			// TODO: fix godep import mess
			ec.toShip <- container.New(encrypted.CompileData())

		case c := <-ec.fromShip:

			// silent fail
			if c == nil {
				return
			}

			// forward if error
			if c.HasError() {
				ec.toShore <- c
				return
			}

			// parse letter
			// TODO: fix godep import mess
			letter, err := jess.LetterFromWireData(c.CompileData())
			if err != nil {
				c.SetError(err)
				ec.toShip <- c
				ec.toShore <- c
				return
			}

			// init
			if ec.session == nil {
				err = ec.setupIncoming(letter)
				if err != nil {
					c.SetError(err)
					ec.toShip <- c
					ec.toShore <- c
					return
				}
			}

			decrypted, err := ec.session.Open(letter)
			if err != nil {
				c.SetError(err)
				ec.toShip <- c
				ec.toShore <- c
				return
			}

			// check if error
			c = container.NewContainer(decrypted)
			c.CheckError()
			if c.HasError() {
				// close other direction silenty
				ec.toShip <- nil
			}

			// send on its way (data or error)
			ec.toShore <- c

		}
	}
}
