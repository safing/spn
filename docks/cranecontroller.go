package docks

import (
	"errors"
	"fmt"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
)

type CraneController struct {
	Crane *Crane
	send  chan *container.Container

	ConnectedHubVerified  *abool.AtomicBool
	verificationChallenge []byte
	Publishing            *abool.AtomicBool
}

const (
	CraneMsgTypeNewLine           uint8 = 1
	CraneMsgTypeDiscardLine       uint8 = 2
	CraneMsgTypeAddLineSpace      uint8 = 3
	CraneMsgTypeHubAnnouncement   uint8 = 4
	CraneMsgTypeHubStatus         uint8 = 5
	CraneMsgTypeVerification      uint8 = 6
	CraneMsgTypePublishConnection uint8 = 7
	CraneMsgError                 uint8 = 8
	CraneMsgClose                 uint8 = 9
)

func NewCraneController(crane *Crane, send chan *container.Container) *CraneController {
	return &CraneController{
		Crane:                crane,
		send:                 send,
		ConnectedHubVerified: abool.New(),
		Publishing:           abool.New(),
	}
}

func (cControl *CraneController) Handle(c *container.Container) error {
	msgType, err := c.GetNextN8()
	if err != nil {
		return err
	}

	// always available endpoints
	switch msgType {
	case CraneMsgTypeNewLine:
		err = cControl.handleNewLine(c)
		if err != nil {
			log.Infof("crane %s: failed to set up new incoming line: %s", cControl.Crane.ID, err)
			return nil
		}
	case CraneMsgTypeDiscardLine:
		err = cControl.handleDiscardLine(c)
	case CraneMsgTypeAddLineSpace:
		err = cControl.handleAddLineSpace(c)
	case CraneMsgTypeHubAnnouncement:
		err = cControl.handleHubAnnouncement(c)
	case CraneMsgTypeHubStatus:
		err = cControl.handleHubStatus(c)
	case CraneMsgTypePublishConnection:
		err = cControl.handlePublishConnection(c)
	default:
		err = errors.New("unknown message type")
	}

	if err != nil {
		return fmt.Errorf("failed to handle control message %d: %s", msgType, err)
	}
	return nil
}

func (cControl *CraneController) getDockingRequest() []byte {
	c := container.New()
	c.AppendNumber(uint64(cControl.Crane.version))
	return c.CompileData()
}

func (cControl *CraneController) handleDockingRequest(c *container.Container) error {
	version, err := c.GetNextN64()
	if err != nil {
		return err
	}
	if uint8(version) != cControl.Crane.version {
		return errors.New("incompatible version")
	}
	return nil
}

func (cControl *CraneController) NewLine(version int) (*ConveyorLine, error) {
	newLine, err := NewConveyorLine(cControl.Crane, cControl.Crane.getNextLineID())
	if err != nil {
		return nil, err
	}

	msg := container.New()
	msg.AppendNumber(uint64(newLine.ID))
	msg.AppendNumber(uint64(version))

	cControl.Crane.linesLock.Lock()
	cControl.Crane.lines[newLine.ID] = newLine
	cControl.Crane.linesLock.Unlock()

	new := container.NewContainer([]byte{CraneMsgTypeNewLine}, msg.CompileData())

	cControl.send <- new

	log.Infof("crane %s: set up new outgoing line %d", cControl.Crane.ID, newLine.ID)
	return newLine, nil
}

func (cControl *CraneController) handleNewLine(c *container.Container) error {
	lineID, err := c.GetNextN32()
	if err != nil {
		return err
	}
	version, err := c.GetNextN8()
	if err != nil {
		return err
	}

	// build conveyor
	newLine, err := NewConveyorLine(cControl.Crane, lineID)
	if err != nil {
		return err
	}

	// add tinker
	ec, err := NewEncryptionConveyor(int(version), cControl.Crane.identity, nil)
	if err != nil {
		return err
	}
	newLine.AddConveyor(ec)

	// add API
	newLine.AddLastConveyor(NewAPI(int(version), true, false))

	cControl.Crane.linesLock.Lock()
	cControl.Crane.lines[lineID] = newLine
	cControl.Crane.linesLock.Unlock()

	log.Infof("crane %s: set up new incoming line %d", cControl.Crane.ID, newLine.ID)

	return nil
}

func (cControl *CraneController) discardLine(id uint32) {
	cControl.Crane.linesLock.Lock()
	line, ok := cControl.Crane.lines[id]
	if ok {
		delete(cControl.Crane.lines, id)
		line.fromShip <- nil
	}
	cControl.Crane.linesLock.Unlock()
	cControl.send <- container.NewContainer([]byte{CraneMsgTypeDiscardLine}, varint.Pack32(id))
}

func (cControl *CraneController) handleDiscardLine(c *container.Container) error {
	id, err := c.GetNextN32()
	if err != nil {
		return err
	}
	cControl.discardLine(id)
	return nil
}

func (cControl *CraneController) addLineSpace(lineID uint32, space int32) {
	c := container.NewContainer([]byte{CraneMsgTypeAddLineSpace})
	c.Append(varint.Pack32(lineID))
	c.Append(varint.Pack32(uint32(space)))
	cControl.send <- c
}

func (cControl *CraneController) CheckAllLineSpaces() {
	c := container.NewContainer([]byte{CraneMsgTypeAddLineSpace})
	cControl.Crane.linesLock.RLock()
	defer cControl.Crane.linesLock.RUnlock()

	for lineID, line := range cControl.Crane.lines {
		report, space := line.getShoreSpaceForReport()
		if report {
			c.Append(varint.Pack32(lineID))
			c.Append(varint.Pack32(uint32(space)))
		}
	}

	cControl.send <- c
}

func (cControl *CraneController) handleAddLineSpace(c *container.Container) error {
	cControl.Crane.linesLock.RLock()
	defer cControl.Crane.linesLock.RUnlock()
	for {
		if c.Length() == 0 {
			return nil
		}

		lineID, err := c.GetNextN32()
		if err != nil {
			log.Warningf("crane %s: could not read space update lineID: %s", cControl.Crane.ID, err)
			return nil
		}

		space, err := c.GetNextN32()
		if err != nil {
			log.Warningf("crane %s: could not read space update space: %s", cControl.Crane.ID, err)
			return nil
		}

		line, ok := cControl.Crane.lines[lineID]
		if ok {
			// log.Debugf("crane %s: updated ship space for line %d: %d", cControl.Crane.ID, lineID, space)
			line.addAvailableShipSpace(int32(space))
		}

	}
}
