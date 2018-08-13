package manager

import (
	"bytes"
	"net"
	"sync"

	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/identity"
	"github.com/Safing/safing-core/utils"
)

type Flingable uint8

const (
	FlingSeagull Flingable = iota
	FlingBottle
)

func (f Flingable) String() string {
	switch f {
	case FlingSeagull:
		return "Seagull"
	case FlingBottle:
		return "Bottle"
	default:
		return "unknown"
	}
}

var (
	BottleIdentifier  = []byte("BOTTLE")
	SeagullIdentifier = []byte("SEAGULL")

	sling     net.PacketConn
	slingLock sync.Mutex
)

func HandleSeagull(conn net.PacketConn, raddr net.Addr, packet []byte) (isSeagull bool) {
	if bytes.HasPrefix(packet, SeagullIdentifier) {
		isSeagull = true
		log.Infof("port17: handling Seagull from %s", raddr.String())
		data, err := identity.Export()
		if err != nil {
			log.Warningf("port17: failed to export identity while handling seagull: %s", err)
			return
		}
		go fling("", conn, raddr, FlingBottle, data)
	}
	return
}

func HandleStreamSeagull(conn net.Conn, packet []byte) (isSeagull bool) {
	if bytes.HasPrefix(packet, SeagullIdentifier) {
		isSeagull = true
		log.Infof("port17: handling Seagull from %s", conn.RemoteAddr().String())
		data, err := identity.Export()
		if err != nil {
			log.Warningf("port17: failed to export identity while handling seagull: %s", err)
			conn.Close()
			return
		}
		written := 0
		for written < len(data) {
			n, err := conn.Write(data[written:])
			if err != nil {
				log.Warningf("port17: failed to respond to seagull: %s", err)
			}
			written += n
		}
		conn.Close()
	}
	return
}

func LetSeagullFly() {
	go flingToAll(FlingSeagull, nil)
}

func FlingMyBottle() {
	exportedBottle, err := identity.Export()
	if err != nil {
		log.Warningf("port17/manager: could not fling my bottle: %s", err)
		return
	}
	go flingToAll(FlingBottle, exportedBottle)
}

func flingToAll(what Flingable, data []byte) {
	switch what {
	case FlingBottle:
		if data == nil {
			return
		}
		data = append(utils.DuplicateBytes(BottleIdentifier), data...)
	case FlingSeagull:
		data = SeagullIdentifier
	default:
		return
	}

	fling("224.0.0.17:17", nil, nil, what, data)
	fling("224.0.2.17:17", nil, nil, what, data)
	fling("[ff05::17]:17", nil, nil, what, data)

	// TODO: figure out how to best handle interface names for: udp6 [ff02::17%enp3s0]:17
}

func fling(address string, conn net.PacketConn, raddr net.Addr, what Flingable, data []byte) {

	var n int
	var err error

	if conn != nil {
		n, err = conn.WriteTo(data, raddr)
		address = raddr.String()
	} else {
		raddr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			log.Errorf("port17: could not fling %s to %s: %s", what, address, err)
			return
		}

		slingLock.Lock()
		if sling == nil {
			log.Warningf("port17: could not fling %s to %s: sling not ready yet", what, address)
			return
		}
		n, err = sling.WriteTo(data, raddr)
		slingLock.Unlock()
	}

	if err != nil {
		log.Errorf("port17: could not fling %s to %s: %s", what, address, err)
		return
	}
	if n != len(data) {
		log.Errorf("port17: could not fully fling %s to %s: written %d out of %d bytes", what, address, n, len(data))
		return
	}

	log.Infof("port17: successfully flung %s to %s", what, address)
}
