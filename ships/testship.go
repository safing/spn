package ships

import (
	"net"

	"github.com/safing/spn/hub"
	"github.com/tevino/abool"
)

// TestShip is a simulated ship that is used for testing higher level components.
type TestShip struct {
	mine      bool
	secure    bool
	loadSize  int
	forward   chan []byte
	backward  chan []byte
	unloadTmp []byte
	sinking   *abool.AtomicBool
}

// NewTestShip returns a new TestShip for simulation.
func NewTestShip(secure bool, loadSize int) *TestShip {
	return &TestShip{
		mine:     true,
		secure:   secure,
		loadSize: loadSize,
		forward:  make(chan []byte, 100),
		backward: make(chan []byte, 100),
		sinking:  abool.NewBool(false),
	}
}

// String returns a human readable informational summary about the ship.
func (d *TestShip) String() string {
	if d.mine {
		return "<TestShip outbound>"
	}
	return "<TestShip inbound>"
}

// Transport returns the transport used for this ship.
func (d *TestShip) Transport() *hub.Transport {
	return &hub.Transport{
		Protocol: "dummy",
	}
}

// IsMine returns whether the ship was launched from here.
func (d *TestShip) IsMine() bool {
	return d.mine
}

// IsSecure returns whether the ship provides transport security.
func (d *TestShip) IsSecure() bool {
	return d.secure
}

// LoadSize returns the recommended data size that should be handed to Load().
// This value will be most likely somehow related to the connection's MTU.
// Alternatively, using a multiple of LoadSize is also recommended.
func (d *TestShip) LoadSize() int {
	return d.loadSize
}

// Reverse creates a connected TestShip. This is used to simulate a connection instead of using a Pier.
func (d *TestShip) Reverse() *TestShip {
	return &TestShip{
		mine:     !d.mine,
		secure:   d.secure,
		loadSize: d.loadSize,
		forward:  d.backward,
		backward: d.forward,
		sinking:  abool.NewBool(false),
	}
}

// Load loads data into the ship - ie. sends the data via the connection.
// Returns ErrSunk if the ship has already sunk earlier.
func (ship *TestShip) Load(data []byte) error {
	// Debugging:
	// log.Debugf("ship: loading %s", spew.Sdump(data))

	// Check if ship is alive.
	if ship.sinking.IsSet() {
		return ErrSunk
	}

	// Empty load is used as a signal to cease operaetion.
	if len(data) == 0 {
		ship.Sink()
		return nil
	}

	// Send all given data.
	ship.forward <- data

	return nil
}

// UnloadTo unloads data from the ship - ie. receives data from the
// connection - puts it into the buf. It returns the amount of data
// written and an optional error.
// Returns ErrSunk if the ship has already sunk earlier.
func (ship *TestShip) UnloadTo(buf []byte) (n int, err error) {
	// Process unload tmp data, if there is any.
	if ship.unloadTmp != nil {
		// Copy as much data as possible.
		copy(buf, ship.unloadTmp)

		// If buf was too small, skip the copied section.
		if len(buf) < len(ship.unloadTmp) {
			ship.unloadTmp = ship.unloadTmp[len(buf):]
			return len(buf), nil
		}

		// If everything was copied, unset the unloadTmp data.
		n := len(ship.unloadTmp)
		ship.unloadTmp = nil
		return n, nil
	}

	// Receive data.
	data := <-ship.backward
	if len(data) == 0 {
		return 0, ErrSunk
	}

	// Copy data, possibly save remainder for later.
	copy(buf, data)
	if len(buf) < len(data) {
		ship.unloadTmp = data[len(buf):]
		return len(buf), nil
	}
	return len(data), nil
}

// LocalAddr returns the underlying local net.Addr of the connection.
func (d *TestShip) LocalAddr() net.Addr {
	return nil
}

// RemoteAddr returns the underlying remote net.Addr of the connection.
func (d *TestShip) RemoteAddr() net.Addr {
	return nil
}

// Sink closes the underlying connection and cleans up any related resources.
func (d *TestShip) Sink() {
	if d.sinking.SetToIf(false, true) {
		close(d.forward)
	}
}
