package core

import (
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/varint"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/rng"
	"github.com/safing/spn/bottle"
	"github.com/safing/spn/identity"
	"github.com/safing/spn/ships"
	"github.com/safing/tinker"
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

	tinker       *tinker.Tinker
	tinkerLock   sync.Mutex
	clientBottle *bottle.Bottle
	serverBottle *bottle.Bottle
	stopped      *abool.AtomicBool
	stop         chan struct{}

	ship      ships.Ship
	fromShip  chan *container.Container
	toShip    chan *container.Container
	fromShore chan *container.Container
	toShore   chan *container.Container

	Controller     *CraneController
	fromController chan *container.Container

	lines      map[uint32]*ConveyorLine
	linesLock  sync.RWMutex
	nextLineID uint32

	maxContainerSize int
}

func NewCrane(ship ships.Ship, serverBottle *bottle.Bottle) (*Crane, error) {

	if serverBottle == nil {
		return nil, errors.New("tried to create crane without serverBottle")
	}

	randomID, err := rng.Bytes(3)
	if err != nil {
		return nil, err
	}

	new := &Crane{
		ID:           hex.EncodeToString(randomID),
		serverBottle: serverBottle,
		stopped:      abool.NewBool(false),
		stop:         make(chan struct{}),

		ship:      ship,
		fromShip:  make(chan *container.Container, 100),
		toShip:    make(chan *container.Container, 100),
		fromShore: make(chan *container.Container, 100),
		toShore:   make(chan *container.Container, 100),

		fromController: make(chan *container.Container, 0),

		lines: make(map[uint32]*ConveyorLine),

		maxContainerSize: 5000,
	}
	new.Controller = NewCraneController(new, new.fromController)

	if ship.IsMine() {
		if hooksActive.IsSet() {
			new.clientBottle = identity.Get()
		}
	} else {
		new.nextLineID = 1
	}

	return new, nil

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

func (crane *Crane) unloader() {

	lenBufLen := 5
	buf := make([]byte, 0, 5)

	maxMsgLen := 8192
	var msgLen int

	for {

		// unload data into buf
		n, ok, err := crane.ship.UnloadTo(buf[len(buf):cap(buf)])
		if !ok {
			if err != nil {
				log.Warningf("crane %s: failed to unload ship: %s", crane.ID, err)
				crane.Stop()
			}
			return
		}
		// set correct used buf length
		buf = buf[:len(buf)+n]
		// log.Debugf("crane %s: read %d bytes from ship, buf is now len=%d, cap=%d", crane.ID, n, len(buf), cap(buf))

		// get message
		if msgLen == 0 {
			// get msgLen
			if len(buf) < 5 {
				continue
			}

			// unpack uvarint
			uMsgLen, n, err := varint.Unpack32(buf)
			if err != nil {
				log.Warningf("crane %s: failed to read msg length: %s", crane.ID, err)
				crane.Stop()
				return
			}
			msgLen = int(uMsgLen)
			// log.Debugf("crane %s: next msg length is %d", crane.ID, msgLen)

			// check sanity
			if msgLen > maxMsgLen {
				log.Warningf("crane %s: invalid msg length greater than %d received: %d", crane.ID, maxMsgLen, msgLen)
				crane.Stop()
				return
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
			crane.fromShip <- container.NewContainer(buf)
			msgLen = 0
			buf = make([]byte, 0, lenBufLen)
		}

	}
}

func (crane *Crane) Initialize() {
	log.Infof("port17: starting to set up crane %s for %s", crane.ID, crane.ship)

	go crane.unloader()

	// setup
	var tk *tinker.Tinker

	if crane.ship.IsMine() {
		// setup tinker
		tk = tinker.NewTinker(tinker.RecommendedNetwork).CommClient().NoSelfAuth()

		// TODO: randomly select a correct key
		keyID, validKey := crane.serverBottle.GetValidKey()
		if validKey == nil {
			log.Warningf("crane %s: failed to initialize: server bottle does not have any valid keys", crane.ID)
			crane.Stop()
			return
		}
		tk.SupplyAuthenticatedServerExchKey(validKey.Key)

		_, err := tk.Init()
		if err != nil {
			log.Warningf("crane %s: failed to create tinker: %s", crane.ID, err)
			crane.Stop()
			return
		}

		// send Initializer
		init := NewInitializer()
		init.TinkerTools = tinker.RecommendedNetwork
		init.KeyexIDs = []int{keyID}
		packedInit, err := init.Pack()
		if err != nil {
			log.Warningf("crane %s: failed to pack initializer: %s", crane.ID, err)
			crane.Stop()
			return
		}
		ok, err := crane.ship.Load(varint.PrependLength(packedInit))
		if !ok {
			if err != nil {
				log.Warningf("crane %s: failed to load ship with initializer: %s", crane.ID, err)
				crane.Stop()
			}
			return
		}
	} else {
		// receive Initializer
		c := <-crane.fromShip
		init, err := UnpackInitializer(c.CompileData())
		if err != nil {
			log.Warningf("crane %s: failed to unpack initializer: %s", crane.ID, err)
			crane.Stop()
			return
		}
		crane.version = init.portVersion

		// setup tinker
		tk = tinker.NewTinker(init.TinkerTools).CommServer().NoRemoteAuth()

		// TODO: control keys with initializer
		crane.serverBottle.Lock()
		for _, keyID := range init.KeyexIDs {
			tk.SupplyAuthenticatedServerExchKey(crane.serverBottle.Keys[keyID].Key)
		}
		crane.serverBottle.Unlock()

		_, err = tk.Init()
		if err != nil {
			log.Warningf("crane %s: failed to create tinker: %s", crane.ID, err)
			crane.Stop()
			return
		}
	}

	crane.tinker = tk
	log.Infof("port17: crane %s for %s successfully set up", crane.ID, crane.ship)

	go crane.handler()
	go crane.loader()

}

func (crane *Crane) handler() {

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
		case <-crane.stop:
			return
		case c := <-crane.fromShip:

			// log.Debugf("crane %s: before decrypt: %v ... %v", crane.ID, c.CompileData()[:10], c.CompileData()[c.Length()-10:])

			if crane.tinker != nil {
				var err error
				crane.tinkerLock.Lock()
				newShipmentData, err = crane.tinker.Decrypt(c.CompileData())
				crane.tinkerLock.Unlock()
				if err != nil {
					log.Warningf("crane %s: failed to decrypt shipment @ %d: %s", crane.ID, cnt, err)
					crane.Stop()
					return
				}
				cnt++
			} else {
				newShipmentData = c.CompileData()
			}

			// get real data part
			realDataLen, n, err := varint.Unpack32(newShipmentData)
			if err != nil {
				log.Warningf("crane %s: could not get length of real data: %s", crane.ID, err)
				crane.Stop()
				return
			}
			dataEnd := n + int(realDataLen)
			if dataEnd > len(newShipmentData) {
				log.Warningf("crane %s: length of real data is greater than available data: %d", crane.ID, realDataLen)
				crane.Stop()
				return
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
						return
					}
					nextContainerLen = int(uContainerLen)

					// check sanity
					if nextContainerLen > maxContainerLen {
						log.Warningf("crane %s: invalid container length (greater than %d) received: %d", crane.ID, maxContainerLen, nextContainerLen)
						crane.Stop()
						return
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
						return
					}

					if lineID == 0 {
						err := crane.Controller.Handle(nextContainer)
						if err != nil {
							log.Warningf("crane %s: failed to handle controller msg: %s", crane.ID, err)
							crane.Stop()
							return
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

func (crane *Crane) loader() {

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
			case <-crane.stop:
				return
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
					return
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
				return
			}

			ok, err := crane.load(shipmentWorkingBuf)
			if !ok {
				if err != nil {
					log.Warningf("crane %s: failed to load ship: %s", crane.ID, err)
					crane.Stop()
				}
				return
			}

			shipmentBuf = make([]byte, shipmentBufSize+shipmentBufAlignmentSize)
			shipmentWorkingBuf = shipmentBuf[shipmentBufLengthSize:shipmentBufSize] // reserve bytes for length
			send = false

		}

	}

}

func (crane *Crane) load(shipment []byte) (ok bool, err error) {

	var wireData []byte

	if crane.tinker != nil {
		var err error
		crane.tinkerLock.Lock()
		wireData, err = crane.tinker.Encrypt(shipment)
		crane.tinkerLock.Unlock()
		if err != nil {
			return false, err
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
		log.Warningf("crane %s: stopped, %s sunk.", crane.ID, crane.ship)
	}
}
