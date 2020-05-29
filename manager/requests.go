package manager

import (
	"net"
	"strings"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/bottle"
	"github.com/safing/spn/core"
	"github.com/safing/spn/identity"
	"github.com/safing/spn/ships"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	QOTD = "Privacy is not an option, and it shouldn't be the price we accept for just getting on the Internet.\nGary Kovacs\n"
)

var (
	port17Module *modules.Module
	myBottle     *bottle.Bottle
)

func handleRequests(network, address string) {
	if strings.HasPrefix(network, "udp") {
		ln, err := net.ListenPacket(network, address)
		if err != nil {
			log.Criticalf("port17: could not listen on %s %s (retrying in 1 sec): %s", network, address, err)
			time.Sleep(time.Second)
			go handleRequests(network, address)
			return
		}

		slingLock.Lock()
		sling = ln
		slingLock.Unlock()

		// add ipv4 multicast
		p4 := ipv4.NewPacketConn(ln)
		if err := p4.JoinGroup(nil, &net.UDPAddr{IP: net.ParseIP("224.0.0.17")}); err != nil {
			log.Errorf("port17: cound not join multicast group %s: %s", "224.0.0.17", err)
		}
		if err := p4.JoinGroup(nil, &net.UDPAddr{IP: net.ParseIP("224.0.2.17")}); err != nil {
			log.Errorf("port17: cound not join multicast group %s: %s", "224.0.2.17", err)
		}

		// add ipv6 multicast
		p6 := ipv6.NewPacketConn(ln)
		if err := p6.JoinGroup(nil, &net.UDPAddr{IP: net.ParseIP("ff02::17")}); err != nil {
			// TODO: check for IPv6 support more intelligenty
			if !strings.HasSuffix(err.Error(), "no such device") {
				log.Errorf("port17: cound not join multicast group %s: %s", "ff02::17", err)
			}
		}
		if err := p6.JoinGroup(nil, &net.UDPAddr{IP: net.ParseIP("ff05::17")}); err != nil {
			// TODO: check for IPv6 support more intelligenty
			if !strings.HasSuffix(err.Error(), "no such device") {
				log.Errorf("port17: cound not join multicast group %s: %s", "ff05::17", err)
			}
		}

		log.Infof("port17/manager: listening on %s %s", network, address)
		for {
			buf := make([]byte, 1024)
			n, raddr, err := ln.ReadFrom(buf)
			if err != nil {
				log.Criticalf("port17: listener for %s %s failed (retrying in 1 sec): %s", network, address, err)
				time.Sleep(time.Second)
				go handleRequests(network, address)
				return
			}
			go welcomeAndDockPacketShip(ln, raddr, buf[:n])
		}
	} else {
		ln, err := net.Listen(network, address)
		if err != nil {
			log.Criticalf("port17: could not listen on %s %s (retrying in 1 sec): %s", network, address, err)
			time.Sleep(time.Second)
			go handleRequests(network, address)
			return
		}
		log.Infof("port17/manager: listening on %s %s", network, address)
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Criticalf("port17: listener for %s %s failed (retrying in 1 sec): %s", network, address, err)
				time.Sleep(time.Second)
				go handleRequests(network, address)
				return
			}
			go welcomeAndDockStreamShip(conn)
		}
	}
}

// welcomeAndDockStreamShip reads the first packet and determines the type of ship to use.
func welcomeAndDockStreamShip(conn net.Conn) {
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		if nerr, ok := err.(net.Error); ok && (nerr.Timeout()) {
			closeStreamWithQOTD(conn)
		} else {
			log.Warningf("port17: failed to read first packet from %s: %s", conn.RemoteAddr().String(), err)
		}
		return
	}

	// check if bottle
	if handleStreamBottle(conn, buf[:n]) {
		return
	}

	// check if seagull
	if HandleStreamSeagull(conn, buf[:n]) {
		return
	}

	ship := ships.IdentifyStreamShip(conn.RemoteAddr().Network(), buf[:n], conn)
	if ship == nil {
		closeStreamWithQOTD(conn)
		return
	}

	// reset deadline
	conn.SetReadDeadline(time.Time{})

	crane, err := core.NewCrane(ship, identity.Get())
	if err != nil {
		log.Warningf("port17: failed to create Crane for %s from %s: %s", ship.String(), conn.RemoteAddr().String(), err)
		return
	}

	crane.Initialize()
}

// welcomeAndDockPacketShip determines the type of ship to use.
func welcomeAndDockPacketShip(conn net.PacketConn, raddr net.Addr, firstPacket []byte) {
	// discard if we are sender
	// if conn.LocalAddr().String() == raddr.String() {
	// 	return
	// }

	// check if bottle
	if handleFlungBottle(conn, raddr, firstPacket) {
		return
	}

	// check if seagull
	if HandleSeagull(conn, raddr, firstPacket) {
		return
	}

	// check if ship
	ship := ships.IdentifyPacketShip(conn.LocalAddr().Network(), firstPacket, conn, raddr)
	if ship == nil {
		closePacketWithQOTD(conn, raddr)
		return
	}

	crane, err := core.NewCrane(ship, identity.Get())
	if err != nil {
		log.Warningf("port17: failed to create Crane for %s from %s: %s", ship.String(), raddr.String(), err)
		return
	}

	crane.Initialize()
}

func closeStreamWithQOTD(conn net.Conn) {
	_, err := conn.Write([]byte(QOTD))
	if err != nil {
		log.Warningf("port17: failed to send QOTD to %s: %s", conn.RemoteAddr().String(), err)
	}
	conn.Close()
}

func closePacketWithQOTD(conn net.PacketConn, raddr net.Addr) {
	_, err := conn.WriteTo([]byte(QOTD), raddr)
	if err != nil {
		log.Warningf("port17: failed to send QOTD to %s: %s", raddr.String(), err)
	}
}
