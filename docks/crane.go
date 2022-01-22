package docks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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

	// loadingMaxWaitDuration is the maximum time a crane will wait for
	// additional data to send.
	loadingMaxWaitDuration = 5 * time.Millisecond
)

// Errors.
var (
	ErrDone = errors.New("crane is done")
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
	// stopping indicates if the Crane will be stopped soon. The Crane may still
	// be used until stopped, but must not be advertised anymore.
	Stopping *abool.AtomicBool
	// stopped indicates if the Crane has been stopped. Whoever stopped the Crane
	// already took care of notifying everyone, so a silent fail is normally the
	// best response.
	stopped *abool.AtomicBool
	// authenticated indicates if there is has been any successful authentication.
	authenticated *abool.AtomicBool

	// ConnectedHub is the identity of the remote Hub.
	ConnectedHub *hub.Hub
	// NetState holds the network optimization state.
	// It must always be set and the reference must not be changed.
	// Access to fields within are coordinated by itself.
	NetState *NetworkOptimizationState
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

func NewCrane(ctx context.Context, ship ships.Ship, connectedHub *hub.Hub, id *cabin.Identity) (*Crane, error) {
	ctx, cancelCtx := context.WithCancel(ctx)

	new := &Crane{
		ctx:           ctx,
		cancelCtx:     cancelCtx,
		Stopping:      abool.NewBool(false),
		stopped:       abool.NewBool(false),
		authenticated: abool.NewBool(false),

		ConnectedHub: connectedHub,
		NetState:     newNetworkOptimizationState(),
		identity:     id,

		ship:          ship,
		unloading:     make(chan *container.Container, 0),
		loading:       make(chan *container.Container, 100),
		terminalMsgs:  make(chan *container.Container, 100),
		importantMsgs: make(chan *container.Container, 100),

		terminals: make(map[uint32]terminal.TerminalInterface),
	}
	err := registerCrane(new)
	if err != nil {
		return nil, fmt.Errorf("failed to register crane: %w", err)
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

func (crane *Crane) IsMine() bool {
	return crane.ship.IsMine()
}

func (crane *Crane) Public() bool {
	return crane.ship.Public()
}

func (crane *Crane) Authenticated() bool {
	return crane.authenticated.IsSet()
}

func (crane *Crane) Publish() error {
	// Check if crane is connected.
	if crane.ConnectedHub == nil {
		return fmt.Errorf("spn/docks: %s: cannot publish without defined connected hub", crane)
	}

	// Submit metrics.
	if !crane.Public() {
		newPublicCranes.Inc()
	}

	// Mark crane as public.
	maskedID := crane.ship.MaskAddress(crane.ship.RemoteAddr())
	crane.ship.MarkPublic()

	// Assign crane to make it available to others.
	AssignCrane(crane.ConnectedHub.ID, crane)

	log.Infof("spn/docks: %s (was %s) is now public", crane, maskedID)
	return nil
}

func (crane *Crane) LocalAddr() net.Addr {
	return crane.ship.LocalAddr()
}

func (crane *Crane) RemoteAddr() net.Addr {
	return crane.ship.RemoteAddr()
}

func (crane *Crane) Transport() *hub.Transport {
	t := crane.ship.Transport()
	return &t
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

func (crane *Crane) terminalCount() int {
	crane.terminalsLock.Lock()
	defer crane.terminalsLock.Unlock()

	return len(crane.terminals)
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

	// Log reason the terminal is ending. Override stopping error with nil.
	switch {
	case err == nil:
		log.Debugf("spn/docks: %T %s is being abandoned", t, t.FmtID())
	case errors.Is(err, terminal.ErrStopping):
		err = nil
		log.Debugf("spn/docks: %T %s is being abandoned by peer", t, t.FmtID())
	default:
		log.Warningf("spn/docks: %T %s: %s", t, t.FmtID(), err)
	}

	// Call the terminal's abandon function.
	t.Abandon(err)

	// If the crane is stopping, check if we can stop.
	// FYI: The crane controller will always take up one slot.
	if crane.Stopping.IsSet() &&
		crane.terminalCount() <= 1 {
		// Stop the crane in worker, so the caller can do some work.
		module.StartWorker("retire crane", func(_ context.Context) error {
			crane.Stop(nil)
			return nil
		})
	}
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
		// 2 bytes are enough to encode 65535.
		// On the other hand, packets can be only 2 bytes small.
		lenBuf := make([]byte, 2)
		err := crane.unloadUntilFull(lenBuf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				crane.Stop(terminal.ErrStopping.With("connection closed"))
			} else {
				crane.Stop(terminal.ErrInternalError.With("failed to unload: %w", err))
			}
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

		// Build shipment.
		var shipmentBuf []byte
		leftovers := len(lenBuf) - n

		if leftovers == int(containerLen) {
			// We already have all the shipment data.
			shipmentBuf = lenBuf[n:]
		} else {
			// Create a shipment buffer, copy leftovers and read the rest from the connection.
			shipmentBuf = make([]byte, containerLen)
			if leftovers > 0 {
				copy(shipmentBuf, lenBuf[n:])
			}

			// Read remaining shipment.
			err = crane.unloadUntilFull(shipmentBuf[leftovers:])
			if err != nil {
				crane.Stop(terminal.ErrInternalError.With("failed to unload: %w", err))
				return nil
			}
		}

		// Submit to handler.
		select {
		case <-crane.ctx.Done():
			crane.Stop(nil)
			return nil
		case crane.unloading <- container.New(shipmentBuf):
		}
	}
}

func (crane *Crane) unloadUntilFull(buf []byte) error {
	var bytesRead int
	for {
		// Get shipment from ship.
		n, err := crane.ship.UnloadTo(buf[bytesRead:])
		if err != nil {
			return err
		}
		if n == 0 {
			log.Tracef("spn/docks: %s unloaded 0 bytes", crane)
		}
		bytesRead += n

		// Return if buffer has been fully filled.
		if bytesRead == len(buf) {
			// Submit metrics.
			crane.submitCraneTrafficStats(bytesRead)
			crane.NetState.ReportTraffic(uint64(bytesRead), true)

			return nil
		}
	}
}

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

	// Finalize data.
	c.PrependLength()
	readyToSend := c.CompileData()

	// Submit metrics.
	crane.submitCraneTrafficStats(len(readyToSend))
	crane.NetState.ReportTraffic(uint64(len(readyToSend)), false)

	// Load onto ship.
	err = crane.ship.Load(readyToSend)
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
		if err.IsOK() {
			log.Infof("spn/docks: %s is done", crane)
		} else {
			log.Warningf("spn/docks: %s is stopping: %s", crane, err)
		}
	}

	// Unregister crane.
	unregisterCrane(crane)

	// Stop controller.
	if crane.Controller != nil {
		crane.Controller.Abandon(err)
	}

	// Wait shortly in order for the controller end message to be sent.
	time.Sleep(loadingMaxWaitDuration * 10)

	// Close connection.
	crane.ship.Sink()

	// Stop all terminals.
	for _, t := range crane.allTerms() {
		t.Abandon(err)
	}

	// Cancel crane context.
	crane.cancelCtx()

	// Notify about change.
	crane.NotifyUpdate()
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
	remoteAddr := crane.ship.RemoteAddr()
	switch {
	case remoteAddr == nil:
		return fmt.Sprintf("crane %s", crane.ID)
	case crane.ship.IsMine():
		return fmt.Sprintf("crane %s to %s", crane.ID, crane.ship.MaskAddress(crane.ship.RemoteAddr()))
	default:
		return fmt.Sprintf("crane %s from %s", crane.ID, crane.ship.MaskAddress(crane.ship.RemoteAddr()))
	}
}

func (crane *Crane) Stopped() bool {
	return crane.stopped.IsSet()
}
