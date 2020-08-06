package sluice

import (
	"net"
	"sync"
)

var (
	entrypointInfoMsg = []byte("You have reached the local SPN entry port, have a nice day!\n")

	sluices     = make(map[string]Sluice)
	sluicesLock sync.Mutex
)

type Sluice interface {
	AwaitRequest(r *Request)
	Abandon()
}

type SluiceBase struct {
	network string
	address string

	listener     net.Listener
	listenerLock sync.Mutex

	pendingRequests     map[uint16]*Request
	pendingRequestsLock sync.Mutex
}

func (s *SluiceBase) init(network, address string) {
	s.network = network
	s.address = address
	s.pendingRequests = make(map[uint16]*Request)
}

func (s *SluiceBase) AwaitRequest(r *Request) {
	s.pendingRequestsLock.Lock()
	defer s.pendingRequestsLock.Unlock()

	s.pendingRequests[r.Info.SrcPort] = r
}

func (s *SluiceBase) getRequest(port uint16) *Request {
	s.pendingRequestsLock.Lock()
	defer s.pendingRequestsLock.Unlock()

	return s.pendingRequests[port]
}

func (s *SluiceBase) Abandon() {
	// remove from registry
	sluicesLock.Lock()
	delete(sluices, s.network)
	sluicesLock.Unlock()

	// close listener
	s.listenerLock.Lock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.listener = nil
	s.listenerLock.Unlock()
}

func (s *SluiceBase) register(sluice Sluice) {
	sluicesLock.Lock()
	sluices[s.network] = sluice
	sluicesLock.Unlock()
}
