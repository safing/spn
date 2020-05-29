package manager

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/api"
	"github.com/safing/spn/core"
	"github.com/safing/spn/entry"
	"github.com/safing/spn/mode"
	"github.com/safing/spn/navigator"
)

func init() {
	go func() {
		time.Sleep(time.Second)
		if mode.Client() {
			go handleTunnelRequests("tcp", "127.0.0.17:1117")
			go handleTunnelRequests("udp", "127.0.0.17:1117")
		}
	}()
}

var (
	tunnelBuildLock sync.Mutex
)

func handleTunnelRequests(network, address string) {
	if strings.HasPrefix(network, "udp") {
		ln, err := net.ListenPacket(network, address)
		if err != nil {
			log.Criticalf("port17/manager: could not listen for tunnel requests on %s %s (retrying in 1 sec): %s", network, address, err)
			time.Sleep(time.Second)
			go handleTunnelRequests(network, address)
			return
		}
		log.Infof("port17/manager: listening for tunnel requests on %s %s", network, address)
		for {
			buf := make([]byte, 4096)
			n, raddr, err := ln.ReadFrom(buf)
			if err != nil {
				log.Criticalf("port17: listener for tunnel requests on %s %s failed (retrying in 1 sec): %s", network, address, err)
				time.Sleep(time.Second)
				go handleTunnelRequests(network, address)
				return
			}
			go handlePacketTunnel(ln, raddr, buf[:n])
		}
	} else {
		ln, err := net.Listen(network, address)
		if err != nil {
			log.Criticalf("port17: could not listen on %s %s (retrying in 1 sec): %s", network, address, err)
			time.Sleep(time.Second)
			go handleTunnelRequests(network, address)
			return
		}
		log.Infof("port17/manager: listening for tunnel requests on %s %s", network, address)
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Criticalf("port17: listener for %s %s failed (retrying in 1 sec): %s", network, address, err)
				time.Sleep(time.Second)
				go handleTunnelRequests(network, address)
				return
			}
			go handleStreamTunnel(conn)
		}
	}
}

func packetReader(conn net.PacketConn, raddr net.Addr, recv chan *container.Container) {
	for {
		buf := make([]byte, 4096)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			c := container.New()
			c.SetError(err)
			recv <- c
			return
		}
		c := container.New(buf[:n])
		recv <- c
	}
}

func handlePacketTunnel(conn net.PacketConn, raddr net.Addr, firstPacket []byte) {
	// start reader
	recv := make(chan *container.Container, 100)
	go packetReader(conn, raddr, recv)

	// create tunnel
	tunnel, err := buildTunnel(raddr.Network(), raddr.String())
	if err != nil {
		log.Warningf("port17/manager: could not create tunnel: %s", err)
		return
	}
	if tunnel == nil {
		infoPacketReply(conn, raddr)
		return
	}

	// TODO: handle firewall stuff

	// send first packet
	tunnel.SendData(container.New(firstPacket))

	// start tunnelling
	var packetLength int
	var written int
	var n int

	for {
		select {
		case msg := <-tunnel.Msgs:
			switch msg.MsgType {
			case api.API_DATA:
				packetLength = msg.Container.Length()
				written = 0
				for written < packetLength {
					n, err = conn.WriteTo(msg.Container.CompileData()[written:], raddr)
					if err != nil {
						log.Warningf("port17/manager: could not write packet, killing tunnel: %s", err)
						tunnel.End()
					}
					written += n
				}
			case api.API_END, api.API_ACK:
				return
			case api.API_ERR:
				log.Warningf("port17/manager: tunnel broke with tunnel error: %s", api.ParseError(msg.Container).Error())
				conn.Close()
				return
			}
		case c := <-recv:
			if c.HasError() {
				log.Warningf("port17/manager: tunnel broke with local error: %s", c.ErrString())
				conn.Close()
			}
			tunnel.SendData(c)
		}
	}
}

func streamReader(conn net.Conn, recv chan *container.Container) {
	for {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				recv <- nil
				return
			}
			c := container.New()
			c.SetError(err)
			recv <- c
			return
		}
		c := container.New(buf[:n])
		recv <- c
	}
}

func handleStreamTunnel(conn net.Conn) {
	log.Tracef("port17/manager: handling new stream tunnel from %s %s", conn.RemoteAddr().Network(), conn.RemoteAddr().String())

	// start reader
	recv := make(chan *container.Container, 100)
	go streamReader(conn, recv)

	// create tunnel
	tunnel, err := buildTunnel(conn.RemoteAddr().Network(), conn.RemoteAddr().String())
	if err != nil {
		log.Warningf("port17/manager: could not create tunnel: %s", err)
		conn.Close()
		return
	}
	if tunnel == nil {
		infoStreamReply(conn)
		return
	}

	// TODO: handle firewall stuff

	// start tunnelling
	var packetLength int
	var written int
	var n int

	for {
		select {
		case msg := <-tunnel.Msgs:
			switch msg.MsgType {
			case api.API_DATA:
				packetLength = msg.Container.Length()
				written = 0
				for written < packetLength {
					n, err = conn.Write(msg.Container.CompileData()[written:])
					if err != nil {
						if err == io.EOF {
							log.Info("port17/manager: tunnel closed.")
							return
						} else {
							log.Warningf("port17/manager: could not write packet, killing tunnel: %s", err)
						}
						tunnel.End()
						return
					}
					written += n
				}
			case api.API_END, api.API_ACK:
				log.Info("port17/manager: tunnel closed.")
				conn.Close()
				return
			case api.API_ERR:
				log.Warningf("port17/manager: tunnel broke with tunnel error: %s", api.ParseError(msg.Container).Error())
				conn.Close()
				return
			}
		case c := <-recv:
			if c == nil {
				log.Info("port17/manager: tunnel closed.")
				tunnel.End()
				return
			}
			if c.HasError() {
				log.Warningf("port17/manager: tunnel broke with local error: %s", c.ErrString())
				conn.Close()
				return
			}
			tunnel.SendData(c)
		}
	}
}

func buildTunnel(network, address string) (*api.Call, error) {
	// only build one tunnel at a time - for now
	tunnelBuildLock.Lock()
	defer tunnelBuildLock.Unlock()

	// get destination IPs
	tunnelInfo := entry.GetTunnelInfo(network, address)
	if tunnelInfo == nil {
		return nil, nil
	}

	// get nearest ports
	col, err := navigator.FindNearestPorts(tunnelInfo.DestIPs)
	if err != nil {
		return nil, fmt.Errorf("failed to find nearest ports: %s", err)
	}

	if col.Len() == 0 {
		return nil, fmt.Errorf("no ports found near %s", tunnelInfo.DestIPs)
	}

	// DEBUG
	log.Info("port17/manager: nearest ports for new tunnel:")
	for _, result := range col.All {
		log.Infof("port17/manager: %s", result)
	}

	// find best path
	ports, err := navigator.FindPathToPorts([]*navigator.Port{col.All[0].Port})
	if err != nil {
		return nil, fmt.Errorf("failed to find path to port: %s", err)
	}
	if !ports[0].HasActiveRoute() {
		return nil, errors.New("first port in route is not active")
	}

	// build path
	currentPort := ports[0]
	for i := 1; i < len(ports); i++ {
		init, err := core.NewInitializerFromBottle(ports[i].Bottle)
		if err != nil {
			return nil, fmt.Errorf("could not create init from bottle: %s", err)
		}
		newAPI, err := currentPort.ActiveAPI.Hop(init, ports[i].Bottle)
		if err != nil {
			return nil, fmt.Errorf("failed to hop to %s: %s", ports[i], err)
		}
		currentPort = ports[i]
		currentPort.ActiveAPI = newAPI
		// TODO: let API be managed
	}

	tunnel, err := currentPort.ActiveAPI.Tunnel(tunnelInfo.Domain, col.All[0].IP, tunnelInfo.Protocol, tunnelInfo.Port)
	if err != nil {
		return nil, fmt.Errorf("failed to init tunnel: %s", err)
	}

	return tunnel, nil
}

var (
	EntrypointInfoMsg = []byte("You have reached the Port17 entry port, have a nice day!\n")
)

func infoPacketReply(conn net.PacketConn, raddr net.Addr) {
	conn.WriteTo(EntrypointInfoMsg, raddr)
	conn.Close()
}

func infoStreamReply(conn net.Conn) {
	conn.Write(EntrypointInfoMsg)
	conn.Close()
}
