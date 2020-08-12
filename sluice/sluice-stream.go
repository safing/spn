package sluice

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/safing/portmaster/netenv"

	"github.com/tevino/abool"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/api"
)

type StreamSluice struct {
	SluiceBase
}

func StartStreamSluice(network, address string) {
	s := &StreamSluice{}
	s.init(network, address)

	module.StartServiceWorker(
		fmt.Sprintf("%s sluice", s.network),
		10*time.Second,
		s.handler,
	)
}

func (s *StreamSluice) handler(ctx context.Context) error {
	s.register(s)
	defer s.Abandon()

	// start listening
	ln, err := net.Listen(s.network, s.address)
	if err != nil {
		return fmt.Errorf("failed to listen: %s", err)
	}

	// set listener for shutdown
	s.listenerLock.Lock()
	s.listener = ln
	s.listenerLock.Unlock()

	log.Infof("spn/sluice: started listening for %s requests on %s", s.network, ln.Addr())

	// handle requests
	for {
		conn, err := ln.Accept()
		if err != nil {
			if module.IsStopping() {
				return nil
			}
			return fmt.Errorf("failed to accept connection: %s", err)
		}

		log.Tracef("spn/sluice: new %s request from %s", s.network, conn.RemoteAddr())
		module.StartWorker(
			fmt.Sprintf("%s sluice connection", s.network),
			func(ctx context.Context) error {
				return s.handleConnection(ctx, conn)
			},
		)
	}
}

func (s *StreamSluice) handleConnection(ctx context.Context, conn net.Conn) error {
	defer conn.Close()

	// get remote address
	remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("failed to get remote address, unexpected type: %T", conn.RemoteAddr())
	}

	// check if the request is local
	local, err := netenv.IsMyIP(remoteAddr.IP)
	if err != nil {
		log.Warningf("spn/sluice: failed to check if request from %s is local: %s", remoteAddr, err)
		return nil
	}
	if !local {
		log.Warningf("spn/sluice: received external request from %s, ignoring", remoteAddr)
		return nil
	}

	// get request
	r := s.getRequest(uint16(remoteAddr.Port))
	if r == nil {
		return s.replyInfoMsg(conn)
	}
	log.Tracef("spn/sluice: matched request from %s to %s %s:%d", conn.RemoteAddr(), r.Domain, r.Info.Dst, r.Info.DstPort)

	// build tunnel
	tunnel, err := buildTunnel(r)
	if err != nil {
		return fmt.Errorf("failed to build tunnel: %s", err)
	}
	defer tunnel.End()

	// start handling
	log.Infof("spn/sluice: handling connection from %s", conn.RemoteAddr())
	stopping := abool.New()
	defer stopping.Set()

	// start reader
	recv := make(chan *container.Container, 100)
	module.StartWorker(
		fmt.Sprintf("%s sluice connection reader", s.network),
		func(_ context.Context) error {
			s.connReader(conn, recv, stopping)
			return nil
		},
	)

	// start tunneling
	for {
		select {
		case <-ctx.Done():
			log.Infof("spn/sluice: closing connection from %s", conn.RemoteAddr())
			return nil
		case msg := <-tunnel.Msgs:
			switch msg.MsgType {
			case api.API_DATA:
				packetData := msg.Container.CompileData()
				packetLength := len(packetData)
				written := 0
				for written < packetLength {
					n, err := conn.Write(packetData[written:])
					switch {
					case err == nil:
						written += n
					case stopping.IsSet():
						return nil
					case err == io.EOF:
						log.Infof("spn/sluice: connection from %s closed", conn.RemoteAddr())
						return nil
					default:
						log.Warningf("spn/sluice: failed to write to sluice connection %s: %s", conn.RemoteAddr(), err)
						return nil
					}
				}
			case api.API_END, api.API_ACK:
				log.Infof("spn/sluice: tunnel closed - closing connection from %s", conn.RemoteAddr())
				return nil
			case api.API_ERR:
				log.Warningf(
					"spn/sluice: tunnel failed with error: %s - closing connection from %s",
					api.ParseError(msg.Container).Error(),
					conn.RemoteAddr(),
				)
				return nil
			}
		case c := <-recv:
			if c == nil {
				return nil
			}
			tunnel.SendData(c)
		}
	}
}

func (s *StreamSluice) connReader(
	conn net.Conn,
	recv chan *container.Container,
	stopping *abool.AtomicBool,
) {
	for {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		switch {
		case err == nil:
			c := container.New(buf[:n])
			recv <- c
		case stopping.IsSet():
			return
		case err == io.EOF:
			log.Infof("spn/sluice: connection from %s closed", conn.RemoteAddr())
			recv <- nil
			return
		default:
			log.Warningf("spn/sluice: failed to read from sluice connection %s: %s", conn.RemoteAddr(), err)
			recv <- nil
			return
		}
	}
}

func (s *StreamSluice) replyInfoMsg(conn net.Conn) error {
	_, err := conn.Write(entrypointInfoMsg)
	if err != nil {
		return fmt.Errorf("failed to write info msg: %s", err)
	}
	return nil
}
