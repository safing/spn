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
	"github.com/safing/spn/terminal"
)

const (
	QOTD = "Privacy is not an option, and it shouldn't be the price we accept for just getting on the Internet.\nGary Kovacs\n"

	// maxUnloadSize defines the maximum size of a message to unload.
	maxUnloadSize    = 16384
	maxSegmentLength = 16384
)

var (
	// optimalMinLoadSize defines minimum for Crane.targetLoadSize.
	optimalMinLoadSize = 3072 // Targeting around 4096.
)

// Errors.
var (
	ErrDone = errors.New("crane is done")
)

// Crane Status Options.
var (
	CraneStatusStopped int8 = -1
	CraneStatusPrivate int8 = 0
	CraneStatusPublic  int8 = 1
)

type Crane struct {
	// ID is the ID of the Crane.
	ID string
	// opts holds options.
	opts terminal.TerminalOpts

	// ctx is the context of the Terminal.
	ctx context.Context
	// cancelCtx cancels ctx.
	cancelCtx context.CancelFunc
	// stopped indicates if the Crane has been stopped. Whoever stopped the Crane
	// already took care of notifying everyone, so a silent fail is normally the
	// best response.
	stopped *abool.AtomicBool
	// public indiciates if this Crane is publicly advertised. If that is the
	// case, IP addresses will be used in logging.
	public *abool.AtomicBool

	// ConnectedHub is the identity of the remote Hub.
	ConnectedHub *hub.Hub
	// identity is identity of this instance and is usually only populated on a server.
	identity *cabin.Identity

	// jession is the jess session used for encryption.
	jession *jess.Session
	// jessionLock locks jession.
	jessionLock sync.Mutex

	// Controller is the Crane's Controller Terminal.
	Controller *CraneControllerTerminal

	// ship represents the underlying physical connection.
	ship ships.Ship
	// unloading moves containers from the ship to the crane.
	unloading chan *container.Container
	// loading moves containers from the crane to the ship.
	loading chan *container.Container
	// terminalMsgs holds containers from terminals waiting to be laoded.
	terminalMsgs chan *container.Container
	// importantMsgs holds important containers from terminals waiting to be laoded.
	importantMsgs chan *container.Container

	// terminals holds all the connected terminals.
	terminals map[uint32]terminal.TerminalInterface
	// terminalsLock locks terminals.
	terminalsLock sync.Mutex
	// nextTerminalID holds the next terminal ID.
	nextTerminalID uint32

	// targetLoadSize defines the optimal loading size.
	targetLoadSize int
}

func NewCrane(ship ships.Ship, connectedHub *hub.Hub, id *cabin.Identity) (*Crane, error) {
	ctx, cancelCtx := context.WithCancel(module.Ctx)
	randomID, err := rng.Bytes(3)
	if err != nil {
		return nil, err
	}

	new := &Crane{
		ID: hex.EncodeToString(randomID),

		ctx:       ctx,
		cancelCtx: cancelCtx,
		stopped:   abool.NewBool(false),
		public:    abool.NewBool(false),

		ConnectedHub: connectedHub,
		identity:     id,

		ship:          ship,
		unloading:     make(chan *container.Container, 0),
		loading:       make(chan *container.Container, 100),
		terminalMsgs:  make(chan *container.Container, 100),
		importantMsgs: make(chan *container.Container, 100),

		terminals: make(map[uint32]terminal.TerminalInterface),
	}

	// Shift next terminal IDs on the server.
	if !ship.IsMine() {
		new.nextTerminalID += 4
	}

	// Calculate target load size.
	loadSize := ship.LoadSize()
	if loadSize <= 0 {
		loadSize = ships.BaseMTU
	}
	new.targetLoadSize = loadSize
	for new.targetLoadSize < optimalMinLoadSize {
		new.targetLoadSize += loadSize
	}
	// Subtract overhead needed for encryption.
	new.targetLoadSize -= 25 // Manually tested for jess.SuiteWireV1
	// Subtract space needed for length encoding the final chunk.
	new.targetLoadSize -= varint.EncodedSize(uint64(new.targetLoadSize))

	return new, nil
}

func (crane *Crane) getNextTerminalID() uint32 {
	crane.terminalsLock.Lock()
	defer crane.terminalsLock.Unlock()

	for {
		// Bump to next ID.
		crane.nextTerminalID += 8

		// Check if it's free.
		_, ok := crane.terminals[crane.nextTerminalID]
		if !ok {
			return crane.nextTerminalID
		}
	}
}

func (crane *Crane) getTerminal(id uint32) (t terminal.TerminalInterface, ok bool) {
	crane.terminalsLock.Lock()
	defer crane.terminalsLock.Unlock()

	t, ok = crane.terminals[id]
	return
}

func (crane *Crane) setTerminal(t terminal.TerminalInterface) {
	crane.terminalsLock.Lock()
	defer crane.terminalsLock.Unlock()

	crane.terminals[t.ID()] = t
}

func (crane *Crane) deleteTerminal(id uint32) (t terminal.TerminalInterface, ok bool) {
	crane.terminalsLock.Lock()
	defer crane.terminalsLock.Unlock()

	t, ok = crane.terminals[id]
	if ok {
		delete(crane.terminals, id)
		return t, true
	}
	return nil, false
}

func (crane *Crane) AbandonTerminal(id uint32, err *terminal.Error) {
	// Get active terminal.
	t, ok := crane.deleteTerminal(id)
	if !ok {
		// Do nothing if terminal is not found.
		return
	}

	// Send error to the connected terminal, if the error is internal.
	if !err.IsExternal() {
		// Build abandon message.
		abandonMsg := container.New(err.Pack())
		terminal.MakeMsg(abandonMsg, id, terminal.MsgTypeStop)

		// Send message directly, or async.
		select {
		case crane.terminalMsgs <- abandonMsg:
		default:
			// Send error async.
			module.StartWorker("abandon terminal", func(ctx context.Context) error {
				select {
				case crane.terminalMsgs <- abandonMsg:
				case <-ctx.Done():
				}
				return nil
			})
		}
	}

	// Log reason the terminal is ending. Override stopping error with nil.
	if err == nil {
		log.Debugf("spn/docks: %T %s is being abandoned", t, t.FmtID())
	} else if errors.Is(err, terminal.ErrStopping) {
		err = nil
		log.Debugf("spn/docks: %T %s is being abandoned by peer", t, t.FmtID())
	} else {
		log.Warningf("spn/docks: %T %s: %s", t, t.FmtID(), err)
	}

	// Call the terminal's abandon function.
	t.Abandon(err)
}

func (crane *Crane) submitImportantTerminalMsg(c *container.Container) {
	crane.importantMsgs <- c
}

func (crane *Crane) submitTerminalMsg(c *container.Container) {
	crane.terminalMsgs <- c
}

func (crane *Crane) encrypt(shipment *container.Container) (encrypted *container.Container, err error) {
	// Skip if encryption is not enabled.
	if crane.jession == nil {
		return shipment, nil
	}

	crane.jessionLock.Lock()
	defer crane.jessionLock.Unlock()

	letter, err := crane.jession.Close(shipment.CompileData())
	if err != nil {
		return nil, err
	}

	encrypted, err = letter.ToWire()
	if err != nil {
		return nil, fmt.Errorf("failed to pack letter: %s", err)
	}

	return encrypted, nil
}

func (crane *Crane) decrypt(shipment *container.Container) (decrypted *container.Container, err error) {
	// Skip if encryption is not enabled.
	if crane.jession == nil {
		return shipment, nil
	}

	crane.jessionLock.Lock()
	defer crane.jessionLock.Unlock()

	letter, err := jess.LetterFromWire(shipment)
	if err != nil {
		return nil, fmt.Errorf("failed to parse letter: %s", err)
	}

	decryptedData, err := crane.jession.Open(letter)
	if err != nil {
		return nil, err
	}

	return container.New(decryptedData), nil
}

func (crane *Crane) unloader(ctx context.Context) error {
	for {
		// Get first couple bytes to get the packet length.
		// 3 bytes are enough enough to encode 2097151.
		// On the other hand, packets could theoretically be only 3 bytes small.
		lenBuf := make([]byte, 3)
		err := crane.unloadUntilFull(lenBuf)
		if err != nil {
			crane.Stop(terminal.ErrInternalError.With("failed to unload: %w", err))
			return nil
		}

		// Unpack length.
		containerLen, n, err := varint.Unpack64(lenBuf)
		if err != nil {
			crane.Stop(terminal.ErrMalformedData.With("failed to get container length: %w", err))
			return nil
		}
		if containerLen > maxUnloadSize {
			crane.Stop(terminal.ErrMalformedData.With("received oversized container with length %d", containerLen))
			return nil
		}

		// Create container buffer and copy leftovers from the length buffer.
		containerBuf := make([]byte, containerLen)
		leftovers := len(lenBuf) - n
		if leftovers > 0 {
			copy(containerBuf, lenBuf[n:])
		}

		// Read remaining container.
		err = crane.unloadUntilFull(containerBuf[leftovers:])
		if err != nil {
			crane.Stop(terminal.ErrInternalError.With("failed to unload: %w", err))
			return nil
		}

		// Submit to handler.
		select {
		case <-crane.ctx.Done():
			crane.Stop(nil)
			return nil
		case crane.unloading <- container.New(containerBuf):
		}
	}
}

func (crane *Crane) unloadUntilFull(buf []byte) error {
	var bytesRead int
	for {
		// Get shipment from ship.
		n, err := crane.ship.UnloadTo(buf[bytesRead:])
		if err != nil {
			return fmt.Errorf("failed to unload ship: %s", err)
		}
		bytesRead += n

		// Return if buffer has been fully filled.
		if bytesRead == len(buf) {
			return nil
		}
	}
}

/*
func (crane *Crane) OldStart() (err error) {
	err = crane.start()
	if err != nil {
		crane.Stop()
	}
	return
}
*/

/*
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
*/

func (crane *Crane) handler(ctx context.Context) error {
	var partialShipment *container.Container
	var segmentLength uint32

handling:
	for {
		select {
		case <-ctx.Done():
			crane.Stop(nil)
			return nil

		case shipment := <-crane.unloading:

			// log.Debugf("crane %s: before decrypt: %v ... %v", crane.ID, c.CompileData()[:10], c.CompileData()[c.Length()-10:])

			// Decrypt shipment.
			shipment, err := crane.decrypt(shipment)
			if err != nil {
				crane.Stop(terminal.ErrIntegrity.With("failed to decrypt: %w", err))
				return nil
			}

			// Process all segments/containers of the shipment.
			for shipment.HoldsData() {
				if partialShipment != nil {
					// Continue processing partial segment.
					// Append new shipment to previous partial segment.
					partialShipment.AppendContainer(shipment)
					shipment, partialShipment = partialShipment, nil
				}

				// Get next segment length.
				if segmentLength == 0 {
					segmentLength, err = shipment.GetNextN32()
					if err != nil {
						if errors.Is(err, varint.ErrBufTooSmall) {
							// Continue handling when there is not yet enough data.
							partialShipment = shipment
							segmentLength = 0
							continue handling
						}

						crane.Stop(terminal.ErrMalformedData.With("failed to get segment length: %w", err))
						return nil
					}

					if segmentLength == 0 {
						// Remainder is padding.
						continue handling
					}

					// Check if the segment is within the boundary.
					if segmentLength > maxSegmentLength {
						crane.Stop(terminal.ErrMalformedData.With("received oversized segment with length %d", segmentLength))
						return nil
					}
				}

				// Check if we have enough data for the segment.
				if uint32(shipment.Length()) < segmentLength {
					partialShipment = shipment
					continue handling
				}

				// Get segment from shipment.
				segment, err := shipment.GetAsContainer(int(segmentLength))
				if err != nil {
					crane.Stop(terminal.ErrMalformedData.With("failed to get segment: %w", err))
					return nil
				}
				segmentLength = 0

				// Get terminal ID and message type of segment.
				terminalID, terminalMsgType, err := terminal.ParseIDType(segment)
				if err != nil {
					crane.Stop(terminal.ErrMalformedData.With("failed to get terminal ID and msg type: %s", err))
					return nil
				}

				switch terminalMsgType {
				case terminal.MsgTypeInit:
					crane.establishTerminal(terminalID, segment)

				case terminal.MsgTypeData:
					// Get terminal and let it further handle the message.
					t, ok := crane.getTerminal(terminalID)
					if ok {
						deliveryErr := t.Deliver(segment)
						if deliveryErr != nil {
							// This is a hot path. Start a worker for abandoning the terminal.
							module.StartWorker("end terminal", func(_ context.Context) error {
								crane.AbandonTerminal(t.ID(), deliveryErr.Wrap("failed to deliver data"))
								return nil
							})
						}
					} else {
						log.Tracef("spn/docks: %s received msg for unknown terminal %d", crane, terminalID)
					}

				case terminal.MsgTypeStop:
					// Parse error.
					receivedErr, err := terminal.ParseExternalError(segment.CompileData())
					if err != nil {
						log.Warningf("spn/docks: %s failed to parse abandon error: %s", crane, err)
						receivedErr = terminal.ErrUnknownError.AsExternal()
					}
					// This is a hot path. Start a worker for abandoning the terminal.
					module.StartWorker("end terminal", func(_ context.Context) error {
						crane.AbandonTerminal(terminalID, receivedErr)
						return nil
					})
				}
			}
		}
	}
}

func (crane *Crane) loader(ctx context.Context) (err error) {
	shipment := container.New()
	var newSegment, partialShipment *container.Container

	loadingMaxWaitDuration := 5 * time.Millisecond
	var loadingTimer *time.Timer

	// Return the loading wait channel if waiting.
	loadNow := func() <-chan time.Time {
		if loadingTimer != nil {
			return loadingTimer.C
		}
		return nil
	}

	for {

	fillingShipment:
		for shipment.Length() < crane.targetLoadSize {
			newSegment = nil
			// Gather segments until shipment is filled, or

			// Prioritize messages from the controller.
			select {
			case newSegment = <-crane.importantMsgs:
			case <-ctx.Done():
				crane.Stop(nil)
				return nil

			default:
				// Then listen for all.
				select {
				case newSegment = <-crane.importantMsgs:
				case newSegment = <-crane.terminalMsgs:
				case <-loadNow():
					break fillingShipment
				case <-ctx.Done():
					crane.Stop(nil)
					return nil
				}
			}

			// Handle new segment.
			if newSegment != nil {
				// Check length.
				if newSegment.Length() > maxSegmentLength {
					log.Warningf("spn/docks: %s ignored oversized segment with length %d", crane, newSegment.Length())
					continue fillingShipment
				}

				// Append to shipment.
				shipment.AppendContainer(newSegment)

				// Set loading max wait timer on first segment.
				if loadingTimer == nil {
					loadingTimer = time.NewTimer(loadingMaxWaitDuration)
				}

			} else if crane.stopped.IsSet() {
				// If there is no new segment, this might have been triggered by a
				// closed channel. Check if the crane is still active.
				return nil
			}
		}

	sendingShipment:
		for {
			// Check if we are over the target load size and split the shipment.
			if shipment.Length() > crane.targetLoadSize {
				partialShipment, err = shipment.GetAsContainer(crane.targetLoadSize)
				if err != nil {
					crane.Stop(terminal.ErrInternalError.With("failed to split segment: %w", err))
					return nil
				}
				shipment, partialShipment = partialShipment, shipment
			}

			// Load shipment.
			err = crane.load(shipment)
			if err != nil {
				crane.Stop(terminal.ErrShipSunk.With("failed to load shipment: %w", err))
				return nil
			}

			// Reset loading timer.
			loadingTimer = nil

			// Continue loading with partial shipment, or a new one.
			if partialShipment != nil {
				// Continue loading with a partial previous shipment.
				shipment, partialShipment = partialShipment, nil

				// If shipment is not big enough to send immediately, wait for more data.
				if shipment.Length() < crane.targetLoadSize {
					loadingTimer = time.NewTimer(loadingMaxWaitDuration)
					break sendingShipment
				}

			} else {
				// Continue loading with new shipment.
				shipment = container.New()
				break sendingShipment
			}
		}
	}

	/*
		for {
			if nextContainer == nil {
				select {
				// prioritize messages from controller
				case <-ctx.Done():
					return nil
				case nextContainer = <-crane.fromController:
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
	*/
}

func (crane *Crane) load(c *container.Container) error {
	if crane.opts.Padding > 0 {
		// Add Padding if needed.
		paddingNeeded := int(crane.opts.Padding) -
			((c.Length() + varint.EncodedSize(uint64(c.Length()))) % int(crane.opts.Padding))
		// As the length changes slightly with the padding, we should avoid loading
		// lengths around the varint size hops:
		// - 128
		// - 16384
		// - 2097152
		// - 268435456

		// Pad to target load size at maximum.
		maxPadding := crane.targetLoadSize - c.Length()
		if paddingNeeded > maxPadding {
			paddingNeeded = maxPadding
		}

		if paddingNeeded > 0 {
			// Add padding indicator.
			c.Append([]byte{0})
			paddingNeeded -= 1

			// Add needed padding data.
			if paddingNeeded > 0 {
				padding, err := rng.Bytes(paddingNeeded)
				if err != nil {
					log.Debugf("spn/docks: %s failed to get random padding data, using zeros instead", crane)
					padding = make([]byte, paddingNeeded)
				}
				c.Append(padding)
			}
		}
	}

	// Encrypt shipment.
	c, err := crane.encrypt(c)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	// Load onto ship.
	c.PrependLength()
	err = crane.ship.Load(c.CompileData())
	if err != nil {
		return fmt.Errorf("failed to load ship: %w", err)
	}

	return nil
}

func (crane *Crane) Stop(err *terminal.Error) {
	if !crane.stopped.SetToIf(false, true) {
		return
	}

	// Log error message.
	if err != nil {
		if errors.Is(err, ErrDone) {
			log.Debugf("spn/docks: %s is done", crane)
		} else {
			log.Warningf("spn/docks: %s is stopping: %s", crane, err)
		}
	}

	// Unregister crane.
	RetractCraneByID(crane.ID)

	// Call discontinue connection hook.
	if hooksActive.IsSet() {
		err := discontinueConnectionHook(crane.Controller, crane.ConnectedHub, nil)
		if err != nil {
			log.Warningf("spn/docks: %s failed to call discontinue connection hook: %s", crane, err)
		}
	}

	// Close connection.
	crane.ship.Sink()

	// Cancel crane context.
	crane.cancelCtx()

	// Stop controller.
	if crane.Controller != nil {
		crane.Controller.Abandon(nil)
	}

	// Stop all terminals.
	for _, t := range crane.allTerms() {
		t.Abandon(err)
	}
}

func (crane *Crane) allTerms() []terminal.TerminalInterface {
	crane.terminalsLock.Lock()
	defer crane.terminalsLock.Unlock()

	terms := make([]terminal.TerminalInterface, 0, len(crane.terminals))
	for _, term := range crane.terminals {
		terms = append(terms, term)
	}

	return terms
}

func (crane *Crane) String() string {
	remoteAddr := "[private]"
	if crane.public.IsSet() {
		remoteAddr = crane.ship.RemoteAddr().String()
	}

	if crane.ship.IsMine() {
		return fmt.Sprintf("crane %s to %s", crane.ID, remoteAddr)
	}
	return fmt.Sprintf("crane %s from %s", crane.ID, remoteAddr)
}
