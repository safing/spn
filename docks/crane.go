package docks

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/jess"
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/cabin"
	"github.com/safing/spn/hub"
	"github.com/safing/spn/ships"
)

const (
	QOTD = "Privacy is not an option, and it shouldn't be the price we accept for just getting on the Internet.\nGary Kovacs\n"
)

// Crane Status Options
var (
	CraneStatusStopped          int8 = -1
	CraneStatusPrivate          int8 = 0
	CraneStatusPublishRequested int8 = 1
	CraneStatusPublishVerifying int8 = 2
	CraneStatusPublished        int8 = 3
)

type Crane struct {
	version uint8
	ID      string
	status  int8

	// involved Hubs
	identity     *cabin.Identity
	ConnectedHub *hub.Hub

	// link encryption
	session     *jess.Session
	sessionLock sync.Mutex

	// controller
	Controller     *CraneController
	fromController chan *container.Container

	// lifecycle management
	stopped *abool.AtomicBool
	stop    chan struct{}

	// data lanes
	ship      ships.Ship
	fromShip  chan *container.Container
	toShip    chan *container.Container
	fromShore chan *container.Container
	toShore   chan *container.Container

	lines      map[uint32]*ConveyorLine
	linesLock  sync.RWMutex
	nextLineID uint32

	maxContainerSize int
}

func NewCrane(ship ships.Ship, id *cabin.Identity, connectedHub *hub.Hub) (*Crane, error) {
	randomID, err := rng.Bytes(3)
	if err != nil {
		return nil, err
	}

	new := &Crane{
		version: 1,
		ID:      hex.EncodeToString(randomID),

		identity:     id,
		ConnectedHub: connectedHub,

		fromController: make(chan *container.Container),
		stopped:        abool.NewBool(false),
		stop:           make(chan struct{}),

		ship:      ship,
		fromShip:  make(chan *container.Container, 100),
		toShip:    make(chan *container.Container, 100),
		fromShore: make(chan *container.Container, 100),
		toShore:   make(chan *container.Container, 100),

		lines: make(map[uint32]*ConveyorLine),

		maxContainerSize: 5000,
	}
	new.Controller = NewCraneController(new, new.fromController)

	if !ship.IsMine() {
		new.nextLineID = 1
	}

	return new, nil
}

func (crane *Crane) Status() int8 {
	return crane.status
}

func (crane *Crane) getNextLineID() uint32 {
	for {
		if crane.nextLineID > 2147483640 {
			crane.nextLineID -= 2147483640
		}
		crane.nextLineID += 2
		crane.linesLock.RLock()
		_, ok := crane.lines[crane.nextLineID]
		crane.linesLock.RUnlock()
		if !ok {
			return crane.nextLineID
		}
	}
}

func (crane *Crane) unloader(ctx context.Context) error {

	lenBufLen := 5
	buf := make([]byte, 0, 5)

	maxMsgLen := 8192
	var msgLen int

	for {

		// unload data into buf
		n, ok, err := crane.ship.UnloadTo(buf[len(buf):cap(buf)])
		if !ok {
			if err != nil {
				if !crane.stopped.IsSet() {
					log.Warningf("crane %s: failed to unload ship: %s", crane.ID, err)
					crane.Stop()
				}
			}
			return nil
		}
		// set correct used buf length
		buf = buf[:len(buf)+n]
		// log.Debugf("crane %s: read %d bytes from ship, buf is now len=%d, cap=%d", crane.ID, n, len(buf), cap(buf))

		// get message
		if msgLen == 0 {
			// get msgLen
			if len(buf) < 2 {
				continue
			}

			// unpack uvarint
			uMsgLen, n, err := varint.Unpack32(buf)
			if err != nil {
				// Don't treat as error if there are less than 5 bytes available
				if len(buf) < 5 {
					continue
				}
				if !crane.stopped.IsSet() {
					log.Warningf("crane %s: failed to read msg length: %s", crane.ID, err)
					crane.Stop()
				}
				return nil
			}
			msgLen = int(uMsgLen)
			// log.Debugf("crane %s: next msg length is %d", crane.ID, msgLen)

			// check sanity
			if msgLen > maxMsgLen {
				if !crane.stopped.IsSet() {
					log.Warningf("crane %s: invalid msg length greater than %d received: %d", crane.ID, maxMsgLen, msgLen)
					crane.Stop()
				}
				return nil
			}

			// copy leftovers to new msg buf
			msgBuf := make([]byte, 0, msgLen)
			copy(msgBuf[:cap(msgBuf)], buf[n:])
			msgBuf = msgBuf[:len(buf[n:])]
			// log.Debugf("crane %s: copied remaining %d bytes to msg buf", crane.ID, len(msgBuf))
			buf = msgBuf
		}

		// forward msg if complete
		if len(buf) == cap(buf) {
			crane.fromShip <- container.New(buf)
			msgLen = 0
			buf = make([]byte, 0, lenBufLen)
		}

	}
}

const (
	// hubInfoRequest is a special code used to request the Hub Announcement and
	// Status from the server. The value 0x7F is used at the place where the wire
	// format version of the first transmitted jess.Letter would be. It is the
	// highest single-byte varint.
	hubInfoRequest = 0x7F
)

func (crane *Crane) Start() (err error) {
	err = crane.start()
	if err != nil {
		crane.Stop()
	}
	return
}

func (crane *Crane) start() (err error) {
	log.Infof("spn/docks: starting crane %s for %s", crane.ID, crane.ship)

	module.StartWorker("crane unloader", crane.unloader)

	if crane.ship.IsMine() {
		if crane.ConnectedHub == nil {
			return errors.New("cannot start outgoing crane without connected Hub")
		}

		// Workaround: always send hub info request, as keys are not saved to disk
		// select a public key of connected hub
		// s := crane.ConnectedHub.SelectSignet()
		// if s == nil {
		// send request
		request := container.New([]byte{hubInfoRequest})
		request.PrependLength()
		ok, err := crane.ship.Load(request.CompileData())
		if err != nil {
			return fmt.Errorf("ship sank: %w", err)
		}
		if !ok {
			return errors.New("ship sank")
		}

		// wait for reply
		var reply *container.Container
		select {
		case reply = <-crane.fromShip:
		case <-time.After(1 * time.Second):
			return errors.New("timed out waiting for hub info")
		}

		// parse announcement
		announcementData, err := reply.GetNextBlock()
		if err != nil {
			return fmt.Errorf("failed to get announcement: %w", err)
		}
		err = hub.ImportAnnouncement(announcementData, hub.ScopePublic)
		if err != nil {
			return fmt.Errorf("failed to import announcement: %w", err)
		}
		// parse status
		statusData, err := reply.GetNextBlock()
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}
		err = hub.ImportStatus(statusData, hub.ScopePublic)
		if err != nil {
			return fmt.Errorf("failed to import status: %w", err)
		}

		// refetch from DB to ensure we have the new version
		dstHub, err := hub.GetHub(hub.ScopePublic, crane.ConnectedHub.ID)
		if err != nil {
			return fmt.Errorf("failed to refetch destination Hub: %w", err)
		}
		crane.ConnectedHub = dstHub

		// try to select public key again
		s := crane.ConnectedHub.SelectSignet()
		if s == nil {
			return errors.New("failed to select signet even after hub info request")
		}
		// }

		// create envelope
		env := jess.NewUnconfiguredEnvelope()
		env.SuiteID = jess.SuiteWireV1
		env.Recipients = []*jess.Signet{s}

		// do not encrypt directly, rather get session for future use, then encrypt
		crane.session, err = env.WireCorrespondence(nil)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}

		// get setup package from controller
		data := crane.Controller.getDockingRequest()

		// encrypt
		letter, err := crane.session.Close(data)
		if err != nil {
			return fmt.Errorf("failed to encrypt initial packet: %w", err)
		}

		// serialize
		c, err := letter.ToWire()
		if err != nil {
			return fmt.Errorf("failed to pack initial packet: %w", err)
		}

		// manually send docking request
		c.PrependLength()
		ok, err = crane.ship.Load(c.CompileData())
		if err != nil {
			return fmt.Errorf("ship sank: %w", err)
		}
		if !ok {
			return errors.New("ship sank")
		}

	} else {
		if crane.identity == nil {
			return errors.New("cannot start incoming crane without designated identity")
		}

		// receive first packet
		var c *container.Container
		select {
		case c = <-crane.fromShip:
		case <-time.After(1 * time.Second):
			// send QOTD
			_, _ = crane.ship.Load([]byte(QOTD))
			crane.Stop()
			return errors.New("timed out while waiting for first packet, sent QotD")
		}

		// check for status request
		// TODO: find a better way
		data := c.CompileData()
		if len(data) >= 1 && data[0] == 0x7F {
			// 0x7F is the highest single-byte varint
			// and is a request for the Hub Announcement and Status

			msg := container.New()

			// send announcement
			announcementData, err := crane.identity.ExportAnnouncement()
			if err != nil {
				return err
			}
			msg.AppendAsBlock(announcementData)

			// send status
			statusData, err := crane.identity.ExportStatus()
			if err != nil {
				return err
			}
			msg.AppendAsBlock(statusData)

			// manually send info reply
			msg.PrependLength()
			ok, err := crane.ship.Load(msg.CompileData())
			if err != nil {
				return fmt.Errorf("ship sank: %w", err)
			}
			if !ok {
				return errors.New("ship sank")
			}

			// receive next message
			select {
			case c = <-crane.fromShip:
				data = c.CompileData()
			case <-time.After(1 * time.Second):
				return errors.New("timed out waiting for docking request")
			}
		}

		// parse packet
		letter, err := jess.LetterFromWireData(data)
		if err != nil {
			return fmt.Errorf("failed to parse initial packet: %w", err)
		}

		// do not decrypt directly, rather get session for future use, then open
		crane.session, err = letter.WireCorrespondence(crane.identity)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}

		// decrypt message
		data, err = crane.session.Open(letter)
		if err != nil {
			return fmt.Errorf("failed to decrypt initial packet: %w", err)
		}

		// crane setup
		err = crane.Controller.handleDockingRequest(container.New(data))
		if err != nil {
			return fmt.Errorf("failed to initialize crane controller: %w", err)
		}
	}

	log.Infof("spn/docks: crane %s for %s operational", crane.ID, crane.ship)

	module.StartWorker("crane loader", crane.loader)
	module.StartWorker("crane handler", crane.handler)
	return nil
}

func (crane *Crane) handler(ctx context.Context) error {

	var newShipmentData []byte
	shipment := container.NewContainer()

	nextContainer := container.NewContainer()
	var nextContainerLen int
	maxContainerLen := 8192

	cnt := 1

	// start handling
handling:
	for {
		select {
		case <-ctx.Done():
			crane.Stop()
			return nil
		case <-crane.stop:
			return nil
		case c := <-crane.fromShip:

			// log.Debugf("crane %s: before decrypt: %v ... %v", crane.ID, c.CompileData()[:10], c.CompileData()[c.Length()-10:])

			crane.sessionLock.Lock()
			if crane.session != nil {
				// parse packet
				letter, err := jess.LetterFromWireData(c.CompileData())
				if err == nil {
					newShipmentData, err = crane.session.Open(letter)
				}
				if err != nil {
					log.Warningf("spn/docks: crane %s failed to decrypt shipment @ %d: %s", crane.ID, cnt, err)
					crane.Stop()
					return nil
				}
				cnt++
			} else {
				newShipmentData = c.CompileData()
			}
			crane.sessionLock.Unlock()

			// get real data part
			realDataLen, n, err := varint.Unpack32(newShipmentData)
			if err != nil {
				log.Warningf("crane %s: could not get length of real data: %s", crane.ID, err)
				crane.Stop()
				return nil
			}
			dataEnd := n + int(realDataLen)
			if dataEnd > len(newShipmentData) {
				log.Warningf("crane %s: length of real data is greater than available data: %d", crane.ID, realDataLen)
				crane.Stop()
				return nil
			}

			shipment.Append(newShipmentData[n:dataEnd])

			for shipment.Length() > 0 {

				if nextContainerLen == 0 {

					// get nextContainerLen
					if shipment.Length() < 5 {
						continue handling
					}

					// unpack uvarint
					uContainerLen, err := shipment.GetNextN32()
					if err != nil {
						log.Warningf("crane %s: failed to read container length: %s", crane.ID, err)
						crane.Stop()
						return nil
					}
					nextContainerLen = int(uContainerLen)

					// check sanity
					if nextContainerLen > maxContainerLen {
						log.Warningf("crane %s: invalid container length (greater than %d) received: %d", crane.ID, maxContainerLen, nextContainerLen)
						crane.Stop()
						return nil
					}

				}

				nextContainer.Append(shipment.GetMax(nextContainerLen - nextContainer.Length()))

				// forward container if complete
				if nextContainer.Length() == nextContainerLen {

					// log.Infof("crane %s: handling container: %s", crane.ID, string(nextContainer.CompileData()))

					lineID, err := nextContainer.GetNextN32()
					if err != nil {
						log.Warningf("crane %s: could not get line ID from container: %s", crane.ID, err)
						crane.Stop()
						return nil
					}

					if lineID == 0 {
						err := crane.Controller.Handle(nextContainer)
						if err != nil {
							log.Warningf("crane %s: failed to handle controller msg: %s", crane.ID, err)
							crane.Stop()
							return nil
						}
					} else {
						crane.linesLock.RLock()
						line, ok := crane.lines[lineID]
						crane.linesLock.RUnlock()
						if ok {
							if nextContainer.Length() == 0 {
								nextContainer = nil
							}
							select {
							case line.fromShip <- nextContainer:
								report, space := line.notifyOfNewContainer()
								if report {
									crane.Controller.addLineSpace(lineID, space)
								}
							default:
								log.Warningf("crane %s: discarding line %d, because it is full", crane.ID, lineID)
								crane.dispatchContainer(lineID, container.New())
								go func() {
									line.fromShip <- nil
								}()
							}
						}
					}
					nextContainer = container.NewContainer()
					nextContainerLen = 0
				}

			}

		}
	}

}

func (crane *Crane) loader(ctx context.Context) error {

	shipmentBufSize := 4096
	shipmentBufLengthSize := 2    // we need to bytes to encode 4094 with varint
	shipmentBufAlignmentSize := 1 // varint also my also just use 1 byte instead two

	shipmentBufDataSpace := shipmentBufSize - shipmentBufLengthSize

	shipmentBuf := make([]byte, shipmentBufSize+shipmentBufAlignmentSize)
	shipmentWorkingBuf := shipmentBuf[shipmentBufLengthSize:shipmentBufSize] // reserve bytes for length

	var nextContainer *container.Container

	waitDuration := 1 * time.Millisecond
	send := false

	timer := time.NewTimer(waitDuration)
	timer.Stop()

	for {

		if nextContainer == nil {
			select {
			// prioritize messages from controller
			case <-ctx.Done():
				return nil
			case <-crane.stop:
				return nil
			case nextContainer = <-crane.fromController:
				nextContainer.Prepend(varint.Pack8(0))
				nextContainer.PrependLength()
				// set timer if first data of shipment
				if len(shipmentWorkingBuf) == shipmentBufDataSpace {
					timer.Reset(waitDuration)
				}
			default:
				select {
				case <-crane.stop:
					return nil
				case nextContainer = <-crane.fromController:
					nextContainer.Prepend(varint.Pack8(0))
					nextContainer.PrependLength()
					// set timer if first data of shipment
					if len(shipmentWorkingBuf) == shipmentBufDataSpace {
						timer.Reset(waitDuration)
					}
				case nextContainer = <-crane.toShip:
					// set timer if first data of shipment
					if len(shipmentWorkingBuf) == shipmentBufDataSpace {
						timer.Reset(waitDuration)
					}
				case <-timer.C:
					send = true
				}
			}
		}

		if nextContainer != nil {
			n, containerEmptied := nextContainer.WriteToSlice(shipmentWorkingBuf)
			shipmentWorkingBuf = shipmentWorkingBuf[n:]
			if containerEmptied {
				nextContainer = nil
			}
		}

		if send || len(shipmentWorkingBuf) == 0 {

			// encode length of real data without wasting space and always ending up with 4096 bytes
			encodedShipmentLength := varint.Pack16(uint16(shipmentBufDataSpace - len(shipmentWorkingBuf)))
			// log.Debugf("crane %s: loading %d bytes of real data", crane.ID, shipmentBufDataSpace-len(shipmentWorkingBuf))
			switch len(encodedShipmentLength) {
			case 1:
				shipmentBuf[1] = encodedShipmentLength[0]
				shipmentWorkingBuf = shipmentBuf[1:]
			case 2:
				shipmentBuf[0] = encodedShipmentLength[0]
				shipmentBuf[1] = encodedShipmentLength[1]
				shipmentWorkingBuf = shipmentBuf[:4096]
			default:
				log.Warningf("crane %s: invalid shipment length: %v", crane.ID, encodedShipmentLength)
				crane.Stop()
				return nil
			}

			ok, err := crane.load(shipmentWorkingBuf)
			if !ok {
				if err != nil {
					log.Warningf("crane %s: failed to load ship: %s", crane.ID, err)
					crane.Stop()
				}
				return nil
			}

			shipmentBuf = make([]byte, shipmentBufSize+shipmentBufAlignmentSize)
			shipmentWorkingBuf = shipmentBuf[shipmentBufLengthSize:shipmentBufSize] // reserve bytes for length
			send = false

		}

	}

}

func (crane *Crane) load(shipment []byte) (ok bool, err error) {

	var wireData []byte

	if crane.session != nil {
		// encrypt
		crane.sessionLock.Lock()
		var letter *jess.Letter
		letter, err = crane.session.Close(shipment)
		if err == nil {
			c, jerr := letter.ToWire()
			if jerr != nil {
				err = jerr
			} else {
				wireData = c.CompileData()
			}
		}
		crane.sessionLock.Unlock()
		if err != nil {
			return false, fmt.Errorf("failed to encrypt initial packet: %w", err)
		}
	} else {
		wireData = shipment
	}

	// log.Debugf("crane %s: after encrypt: %v ... %v", crane.ID, wireData[:10], wireData[len(wireData)-10:])

	ok, err = crane.ship.Load(varint.Pack64(uint64(len(wireData))))
	if err != nil {
		return
	}

	ok, err = crane.ship.Load(wireData)
	if err != nil {
		return
	}

	return true, nil
}

func (crane *Crane) dispatchContainer(lineID uint32, c *container.Container) {
	c.Prepend(varint.Pack32(lineID))
	c.PrependLength()
	crane.toShip <- c
}

func (crane *Crane) Stop() {
	if crane.stopped.SetToIf(false, true) {
		crane.linesLock.Lock()
		for _, line := range crane.lines {
			line.fromShip <- nil
		}
		crane.linesLock.Unlock()
		close(crane.stop)
		crane.status = CraneStatusStopped
		crane.ship.Sink()
		RetractCraneByID(crane.ID)
		if hooksActive.IsSet() {
			_ = discontinueConnectionHook(crane.Controller, crane.ConnectedHub, nil)
		}
		log.Warningf("crane %s: stopped, %s sunk.", crane.ID, crane.ship)
	}
}
