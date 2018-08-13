package port17

import (
	"fmt"

	"github.com/Safing/safing-core/container"
	"github.com/Safing/safing-core/formats/varint"
	"github.com/Safing/safing-core/log"
)

type CraneController struct {
	Crane *Crane
	send  chan *container.Container

	verificationChallenge []byte
}

const (
	CraneMsgTypeNewLine        uint8 = 1
	CraneMsgTypeDiscardLine    uint8 = 2
	CraneMsgTypeAddLineSpace   uint8 = 3
	CraneMsgTypeUpdateBottle   uint8 = 4
	CraneMsgTypeDistrustBottle uint8 = 5
	CraneMsgTypePublishChannel uint8 = 7
	CraneMsgError              uint8 = 8
	CraneMsgClose              uint8 = 9
)

func NewCraneController(crane *Crane, send chan *container.Container) *CraneController {
	return &CraneController{
		Crane: crane,
		send:  send,
	}
}

func (cControl *CraneController) Handle(c *container.Container) error {

	msgType, err := c.GetNextN8()
	if err != nil {
		return err
	}

	err = nil

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
	case CraneMsgTypeUpdateBottle:
		err = cControl.handleUpdateBottle(c)
	case CraneMsgTypeDistrustBottle:
		err = cControl.handleDistrustBottle(c)
	case CraneMsgTypePublishChannel:
		err = cControl.handlePublishChannel(c)
	}

	if err != nil {
		return fmt.Errorf("msgType %d: %s", msgType, err)
	}
	return nil
}

func (cControl *CraneController) NewLine(init *Initializer) (*ConveyorLine, error) {
	newLine, err := NewConveyorLine(cControl.Crane, cControl.Crane.getNextLineID())
	if err != nil {
		return nil, err
	}

	init.LineID = newLine.ID
	data, err := init.Pack()
	if err != nil {
		return nil, err
	}

	cControl.Crane.linesLock.Lock()
	cControl.Crane.lines[newLine.ID] = newLine
	cControl.Crane.linesLock.Unlock()

	new := container.NewContainer([]byte{CraneMsgTypeNewLine}, data)

	cControl.send <- new

	log.Infof("crane %s: set up new outgoing line %d", cControl.Crane.ID, newLine.ID)
	return newLine, nil
}

func (cControl *CraneController) handleNewLine(c *container.Container) error {
	init, err := UnpackInitializer(c.CompileData())
	if err != nil {
		return err
	}

	newLine, err := NewConveyorLine(cControl.Crane, init.LineID)
	if err != nil {
		return err
	}

	// add tinker
	tc, err := NewTinkerConveyor(true, init, cControl.Crane.serverBottle)
	if err != nil {
		return err
	}
	newLine.AddConveyor(tc)

	// add port17 api
	newLine.AddLastConveyor(NewAPI(true, false))

	cControl.Crane.linesLock.Lock()
	cControl.Crane.lines[init.LineID] = newLine
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
			log.Warningf("crane %s: could not read space update lineID: %s")
			return nil
		}

		space, err := c.GetNextN32()
		if err != nil {
			log.Warningf("crane %s: could not read space update space: %s")
			return nil
		}

		line, ok := cControl.Crane.lines[lineID]
		if ok {
			// log.Debugf("crane %s: updated ship space for line %d: %d", cControl.Crane.ID, lineID, space)
			line.addAvailableShipSpace(int32(space))
		}

	}
	return nil
}
