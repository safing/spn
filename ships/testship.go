package ships

import (
	"errors"
	"net"

	"github.com/safing/spn/hub"
	"github.com/tevino/abool"
)

// TestShip is a simulated ship that is used for testing higher level components.
type TestShip struct {
	mine      bool
	forward   chan []byte
	backward  chan []byte
	unloadTmp []byte
	sinking   *abool.AtomicBool
}

// NewTestShip returns a new TestShip for simulation.
func NewTestShip() *TestShip {
	return &TestShip{
		mine:     true,
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

// Reverse creates a connected TestShip. This is used to simulate a connection instead of using a Pier.
func (d *TestShip) Reverse() *TestShip {
	return &TestShip{
		mine:     !d.mine,
		forward:  d.backward,
		backward: d.forward,
		sinking:  abool.NewBool(false),
	}
}

// Load loads data into the ship - ie. sends the data via the connection. It returns the amount of data written, a boolean if everything is ok and an optional error if something is not okay and needs to be reported. If ok is false, stop using the ship.
func (d *TestShip) Load(data []byte) (ok bool, err error) {
	if d.sinking.IsSet() {
		return false, nil
	}
	d.forward <- data
	return true, nil
}

// UnloadTo unloads data from the ship - ie. receives data from the connection - puts it into the buf. It returns the amount of data written, a boolean if everything is ok and an optional error if something is not okay and needs to be reported. If ok is false, stop using the ship.
func (d *TestShip) UnloadTo(buf []byte) (n int, ok bool, err error) {
	if d.sinking.IsSet() {
		return 0, false, nil
	}
	var data []byte
	if len(d.unloadTmp) > 0 {
		data = d.unloadTmp
	} else {
		data = <-d.backward
	}
	if len(data) == 0 {
		return 0, false, errors.New("ship sunk.")
	}
	copy(buf, data)
	if len(buf) >= len(data) {
		d.unloadTmp = nil
		return len(data), true, nil
	}
	d.unloadTmp = data[len(buf):]
	return len(buf), true, nil
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
