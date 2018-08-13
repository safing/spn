package ships

import (
	"errors"

	"github.com/tevino/abool"
)

type DummyShip struct {
	name      string
	mine      bool
	forward   chan []byte
	backward  chan []byte
	unloadTmp []byte
	sinking   *abool.AtomicBool
}

func NewDummyShip() *DummyShip {
	return &DummyShip{
		name:     "DummyShip",
		mine:     true,
		forward:  make(chan []byte, 10),
		backward: make(chan []byte, 10),
		sinking:  abool.NewBool(false),
	}
}

func (d *DummyShip) Reverse() *DummyShip {
	return &DummyShip{
		name:     "DummyShip",
		mine:     !d.mine,
		forward:  d.backward,
		backward: d.forward,
		sinking:  abool.NewBool(false),
	}
}

func (d *DummyShip) Load(data []byte) (ok bool, err error) {
	if d.sinking.IsSet() {
		return false, nil
	}
	d.forward <- data
	// log.Debugf("dummyship: loaded %d bytes", len(data))
	return true, nil
}

func (d *DummyShip) UnloadTo(buf []byte) (n int, ok bool, err error) {
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
		// log.Debugf("dummyship: unloaded %d bytes: '%s'", len(data), string(data))
		return len(data), true, nil
	}
	d.unloadTmp = data[len(buf):]
	// log.Debugf("dummyship: unloaded %d bytes: '%s'", len(buf), string(buf))
	return len(buf), true, nil
}

func (d *DummyShip) IsMine() bool {
	return d.mine
}

func (d *DummyShip) String() string {
	return d.name
}

func (d *DummyShip) Sink() {
	if d.sinking.SetToIf(false, true) {
		close(d.forward)
	}
}
