package ships

import (
	"fmt"
	"net"
	"sync"

	"github.com/safing/portbase/log"
)

var (
	streamShipRegistry     = make(map[string]StreamShipFactory)
	streamShipRegistryLock sync.RWMutex
	packetShipRegistry     = make(map[string]PacketShipFactory)
	packetShipRegistryLock sync.RWMutex
)

type Ship interface {
	Load(data []byte) (ok bool, err error)
	UnloadTo(buf []byte) (n int, ok bool, err error)
	IsMine() bool
	String() string
	Sink()
}

type StreamShipFactory interface {
	SetSail(address string) (Ship, error)
	IdentifyShip(network string, firstPacket []byte, conn net.Conn) (Ship, error)
}

type PacketShipFactory interface {
	SetSail(address string) (Ship, error)
	IdentifyShip(network string, firstPacket []byte, conn net.PacketConn, raddr net.Addr) (Ship, error)
}

func RegisterStreamShipFactory(name string, factory StreamShipFactory) {
	streamShipRegistryLock.Lock()
	streamShipRegistry[name] = factory
	streamShipRegistryLock.Unlock()
}

func RegisterPacketShipFactory(name string, factory PacketShipFactory) {
	packetShipRegistryLock.Lock()
	packetShipRegistry[name] = factory
	packetShipRegistryLock.Unlock()
}

func IdentifyStreamShip(network string, firstPacket []byte, conn net.Conn) Ship {
	streamShipRegistryLock.RLock()
	defer streamShipRegistryLock.RUnlock()
	for name, factory := range streamShipRegistry {
		ship, err := factory.IdentifyShip(network, firstPacket, conn)
		if err != nil {
			log.Warningf("port17: failed to dock %s-Ship from %s: %s", name, conn.RemoteAddr().String(), err)
			return nil
		}
		if ship != nil {
			log.Infof("port17: new %s docked.", ship)
			return ship
		}
	}
	return nil
}

func IdentifyPacketShip(network string, firstPacket []byte, conn net.PacketConn, raddr net.Addr) Ship {
	packetShipRegistryLock.RLock()
	defer packetShipRegistryLock.RUnlock()
	for name, factory := range packetShipRegistry {
		ship, err := factory.IdentifyShip(network, firstPacket, conn, raddr)
		if err != nil {
			log.Warningf("port17: failed to dock %s from %s: %s", name, raddr.String(), err)
			return nil
		}
		if ship != nil {
			log.Infof("port17: new ship (%s) from %s docked.", name, raddr.String())
			return ship
		}
	}
	return nil
}

func SetSail(shipName, address string) (Ship, error) {
	streamShipRegistryLock.RLock()
	defer streamShipRegistryLock.RUnlock()
	streamFactory, ok := streamShipRegistry[shipName]
	if ok {
		return streamFactory.SetSail(address)
	}

	packetShipRegistryLock.RLock()
	defer packetShipRegistryLock.RUnlock()
	packetFactory, ok := packetShipRegistry[shipName]
	if ok {
		return packetFactory.SetSail(address)
	}

	return nil, fmt.Errorf("port17/ships: could not set sail: unknown ship type %s", shipName)
}
