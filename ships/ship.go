package ships

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/safing/spn/hub"
	"github.com/tevino/abool"
)

const (
	defaultBufSize = 4096
)

// Ship represents a network layer connection.
type Ship interface {
	// String returns a human readable informational summary about the ship.
	String() string

	// Transport returns the transport used for this ship.
	Transport() *hub.Transport

	// IsMine returns whether the ship was launched from here.
	IsMine() bool

	// Load loads data into the ship - ie. sends the data via the connection. It returns the amount of data written, a boolean if everything is ok and an optional error if something is not okay and needs to be reported. If ok is false, stop using the ship.
	Load(data []byte) (ok bool, err error)

	// UnloadTo unloads data from the ship - ie. receives data from the connection - puts it into the buf. It returns the amount of data written, a boolean if everything is ok and an optional error if something is not okay and needs to be reported. If ok is false, stop using the ship.
	UnloadTo(buf []byte) (n int, ok bool, err error)

	// LocalAddr returns the underlying local net.Addr of the connection.
	LocalAddr() net.Addr

	// RemoteAddr returns the underlying remote net.Addr of the connection.
	RemoteAddr() net.Addr

	// Sink closes the underlying connection and cleans up any related resources.
	Sink()
}

// ShipBase implements common functions to comply with the Ship interface.
type ShipBase struct {
	ctx          context.Context
	transport    *hub.Transport
	mine         bool
	conn         net.Conn
	bufSize      int
	sinking      *abool.AtomicBool
	initial      []byte
	unloadErrCnt int
	loadErrCnt   int
}

func (ship *ShipBase) initBase(
	ctx context.Context,
	transport *hub.Transport,
	mine bool,
	conn net.Conn,
) {
	// populate
	ship.ctx = ctx
	ship.transport = transport
	ship.mine = mine
	ship.conn = conn

	// init
	ship.sinking = abool.New()

	// check buf size
	if ship.bufSize == 0 {
		ship.bufSize = defaultBufSize
	}
}

// String returns a human readable informational summary about the ship.
func (ship *ShipBase) String() string {
	if ship.mine {
		return fmt.Sprintf("<Ship to %s using %s>", ship.RemoteAddr(), ship.transport)
	}
	return fmt.Sprintf("<Ship from %s using %s>", ship.RemoteAddr(), ship.transport)
}

// Transport returns the transport used for this ship.
func (ship *ShipBase) Transport() *hub.Transport {
	return ship.transport
}

// IsMine returns whether the ship was launched from here.
func (ship *ShipBase) IsMine() bool {
	return ship.mine
}

// Load loads data into the ship - ie. sends the data via the connection. It returns the amount of data written, a boolean if everything is ok and an optional error if something is not okay and needs to be reported. If ok is false, stop using the ship.
func (ship *ShipBase) Load(data []byte) (ok bool, err error) {
	// log.Debugf("ship: loading %s", string(data))
	// quit
	if len(data) == 0 {
		if ship.sinking.SetToIf(false, true) {
			ship.conn.Close()
		}
		return false, nil
	}
	// send all data
	for len(data) != 0 {
		n, err := ship.conn.Write(data)
		if err != nil {
			if ship.sinking.IsSet() {
				return false, nil
			}
			if nerr, ok := err.(net.Error); ok && (nerr.Temporary()) {
				time.Sleep(time.Millisecond)
				ship.loadErrCnt += 1
				if ship.loadErrCnt > 1000 {
					// fail if temporary error persists
					return false, err
				}
				continue
			}
			if ship.sinking.SetToIf(false, true) {
				ship.conn.Close()
			}
			return false, err
		}
		data = data[n:]
	}
	return true, nil
}

// UnloadTo unloads data from the ship - ie. receives data from the connection - puts it into the buf. It returns the amount of data written, a boolean if everything is ok and an optional error if something is not okay and needs to be reported. If ok is false, stop using the ship.
func (ship *ShipBase) UnloadTo(buf []byte) (n int, ok bool, err error) {
	if ship.initial != nil {
		// log.Debugf("ship: unloading initial %s", string(ship.initial))
		copy(buf, ship.initial)
		if len(buf) < len(ship.initial) {
			ship.initial = ship.initial[len(buf):]
			return len(buf), true, nil
		}
		defer func() {
			ship.initial = nil
		}()
		return len(ship.initial), true, nil
	}
	n, err = ship.conn.Read(buf)
	// log.Debugf("ship: unloading %v", string(buf[:n]))
	if err != nil {
		if ship.sinking.IsSet() {
			return 0, false, nil
		}
		if nerr, ok := err.(net.Error); ok && (nerr.Temporary()) {
			time.Sleep(time.Millisecond)
			ship.unloadErrCnt += 1
			if ship.unloadErrCnt > 1000 {
				// fail if temporary error persists
				return 0, false, err
			}
			return 0, true, nil
		}
		if ship.sinking.SetToIf(false, true) {
			ship.conn.Close()
		}
		return 0, false, err
	}
	return n, true, nil
}

// LocalAddr returns the underlying local net.Addr of the connection.
func (ship *ShipBase) LocalAddr() net.Addr {
	return ship.conn.LocalAddr()
}

// RemoteAddr returns the underlying remote net.Addr of the connection.
func (ship *ShipBase) RemoteAddr() net.Addr {
	return ship.conn.RemoteAddr()
}

// Sink closes the underlying connection and cleans up any related resources.
func (ship *ShipBase) Sink() {
	if ship.sinking.SetToIf(false, true) {
		_ = ship.conn.Close()
	}
}
