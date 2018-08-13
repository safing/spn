package ships

import (
	"bytes"
	"net"
	"strings"

	"github.com/Safing/safing-core/log"
	"github.com/xtaci/kcp-go"
)

type KCPShipFactory struct{}

var (
	KCPShipIdentifier = []byte("Port17 KCP")
)

func init() {
	RegisterPacketShipFactory("KCP", &KCPShipFactory{})
}

func (factory *KCPShipFactory) SetSail(address string) (Ship, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}

	_, err = conn.Write(KCPShipIdentifier)
	if err != nil {
		return nil, err
	}

	log.Infof("port17: new ship (KCP) to %s docked.", address)
	return NewKCPShip(conn, address, true)
}

func (factory *KCPShipFactory) IdentifyShip(network string, firstPacket []byte, conn net.PacketConn, raddr net.Addr) (Ship, error) {
	if strings.HasPrefix(network, "udp") && bytes.HasPrefix(firstPacket, KCPShipIdentifier) {
		return NewKCPShip(conn, raddr.String(), false)
	}
	return nil, nil
}

func NewKCPShip(conn net.PacketConn, raddr string, mine bool) (Ship, error) {
	kcpSession, err := kcp.NewConn(raddr, nil, 16, 16, conn)
	if err != nil {
		return nil, err
	}
	return NewGenericShip("KCP", kcpSession, mine), nil
}
