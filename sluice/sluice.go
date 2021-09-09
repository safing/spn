package sluice

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/netenv"
)

type Sluice struct {
	network        string
	address        string
	createListener ListenerFactory

	lock            sync.Mutex
	listener        net.Listener
	pendingRequests map[uint16]*Request
	abandoned       bool
}

type ListenerFactory func(network, address string) (net.Listener, error)

func StartSluice(network, address string) {
	s := &Sluice{
		network:         network,
		address:         address,
		pendingRequests: make(map[uint16]*Request),
	}

	switch s.network {
	case "tcp4", "tcp6":
		s.createListener = net.Listen
	case "udp4", "udp6":
		s.createListener = ListenPacket
	default:
		log.Errorf("spn/sluice: cannot start sluice for %s: unsupported network", network)
		return
	}

	// Start service worker.
	module.StartServiceWorker(
		fmt.Sprintf("%s sluice listener", s.network),
		10*time.Second,
		s.listenHandler,
	)
}

func (s *Sluice) AwaitRequest(r *Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.pendingRequests[r.Info.SrcPort] = r
}

func (s *Sluice) getRequest(port uint16) (r *Request, ok bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	r, ok = s.pendingRequests[port]
	return
}

func (s *Sluice) init() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.abandoned = false

	// start listening
	s.listener = nil
	ln, err := s.createListener(s.network, s.address)
	if err != nil {
		return fmt.Errorf("failed to listen: %s", err)
	}
	s.listener = ln

	// Add to registry.
	addSluice(s)

	return nil
}

func (s *Sluice) abandon() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.abandoned {
		return
	}
	s.abandoned = true

	// Remove from registry.
	removeSluice(s.network)

	// Close listener.
	if s.listener != nil {
		_ = s.listener.Close()
	}

	// Notify pending requests.
	for i, r := range s.pendingRequests {
		r.CallbackFn(r, nil)
		delete(s.pendingRequests, i)
	}
}

func (s *Sluice) handleConnection(conn net.Conn) {
	// Close the connection if handling is not successful.
	success := false
	defer func() {
		if !success {
			conn.Close()
		}
	}()

	// Get IP address and network.
	var remoteIP net.IP
	var remotePort int
	switch typedAddr := conn.RemoteAddr().(type) {
	case *net.TCPAddr:
		remoteIP = typedAddr.IP
		remotePort = typedAddr.Port
	case *net.UDPAddr:
		remoteIP = typedAddr.IP
		remotePort = typedAddr.Port
	default:
		log.Warningf("spn/sluice: cannot handle connection for unsupported network %s", conn.RemoteAddr().Network())
		return
	}

	// Check if the request is local.
	local, err := netenv.IsMyIP(remoteIP)
	if err != nil {
		log.Errorf("spn/sluice: failed to check if request from %s is local: %s", remoteIP, err)
		return
	}
	if !local {
		log.Errorf("spn/sluice: received external request from %s, ignoring", remoteIP)
		return
	}

	// Get waiting request.
	r, ok := s.getRequest(uint16(remotePort))
	if !ok {
		_, err := conn.Write(entrypointInfoMsg)
		if err != nil {
			log.Warningf("spn/sluice: new %s request from %s without pending request, but failed to reply with info msg: %s", s.network, conn.RemoteAddr(), err)
		} else {
			log.Debugf("spn/sluice: new %s request from %s without pending request, replied with info msg", s.network, conn.RemoteAddr())
		}
		return
	}

	// Hand over to callback.
	log.Tracef("spn/sluice: new %s request from %s for %s %s:%d", s.network, conn.RemoteAddr(), r.Domain, r.Info.Dst, r.Info.DstPort)
	r.CallbackFn(r, conn)
	success = true
}

func (s *Sluice) listenHandler(_ context.Context) error {
	defer s.abandon()
	err := s.init()
	if err != nil {
		return err
	}

	// Handle new connections.
	log.Infof("spn/sluice: started listening for %s requests on %s", s.network, s.listener.Addr())
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if module.IsStopping() {
				return nil
			}
			return fmt.Errorf("failed to accept connection: %s", err)
		}

		s.handleConnection(conn)
	}
}
