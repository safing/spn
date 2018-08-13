package ships

import (
	"bytes"
	"net"
	"strings"

	"github.com/Safing/safing-core/log"
)

type TCPShipFactory struct{}

var (
	TCPShipIdentifier = []byte("Port17 TCP")
)

func init() {
	RegisterStreamShipFactory("TCP", &TCPShipFactory{})
}

func (factory *TCPShipFactory) SetSail(address string) (Ship, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	_, err = conn.Write(TCPShipIdentifier)
	if err != nil {
		return nil, err
	}

	log.Infof("port17: new ship (TCP) to %s docked.", address)
	return NewTCPShip(conn, true), nil
}

func (factory *TCPShipFactory) IdentifyShip(network string, firstPacket []byte, conn net.Conn) (Ship, error) {
	if strings.HasPrefix(network, "tcp") && bytes.HasPrefix(firstPacket, TCPShipIdentifier) {
		ship := NewTCPShip(conn, false)
		ship.initial = firstPacket[len(TCPShipIdentifier):]
		return ship, nil
	}
	return nil, nil
}

func NewTCPShip(conn net.Conn, mine bool) *GenericShip {
	return NewGenericShip("TCP", conn, mine)
}
