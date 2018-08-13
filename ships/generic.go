package ships

import (
	"fmt"
	"net"
	"time"

	"github.com/tevino/abool"
)

type GenericShip struct {
	name         string
	mine         bool
	conn         net.Conn
	bufSize      int
	sinking      *abool.AtomicBool
	initial      []byte
	unloadErrCnt int
	loadErrCnt   int
}

func NewGenericShip(name string, conn net.Conn, mine bool) *GenericShip {
	new := GenericShip{
		name:    name,
		mine:    mine,
		conn:    conn,
		bufSize: 4096,
		sinking: abool.NewBool(false),
	}
	return &new
}

// UnloadTo gets data from a docked ship and puts it into the buf. It returns the amount of data written, a boolean if everything is ok and an optional error if something is not okay.
func (ship *GenericShip) UnloadTo(buf []byte) (int, bool, error) {
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
	n, err := ship.conn.Read(buf)
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

func (ship *GenericShip) Load(data []byte) (bool, error) {
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

func (ship *GenericShip) IsMine() bool {
	return ship.mine
}

func (ship *GenericShip) String() string {
	if ship.mine {
		return fmt.Sprintf("%s-Ship to %s", ship.name, ship.conn.RemoteAddr().String())
	}
	return fmt.Sprintf("%s-Ship from %s", ship.name, ship.conn.RemoteAddr().String())
}

func (ship *GenericShip) Sink() {
	if ship.sinking.SetToIf(false, true) {
		ship.conn.Close()
	}
}
