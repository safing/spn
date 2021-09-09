package sluice

import "sync"

var (
	sluices     = make(map[string]*Sluice)
	sluicesLock sync.RWMutex
)

func getSluice(network string) (s *Sluice, ok bool) {
	sluicesLock.RLock()
	defer sluicesLock.RUnlock()

	s, ok = sluices[network]
	return
}

func addSluice(s *Sluice) {
	sluicesLock.Lock()
	defer sluicesLock.Unlock()

	sluices[s.network] = s
}

func removeSluice(network string) {
	sluicesLock.Lock()
	defer sluicesLock.Unlock()

	delete(sluices, network)
}

func stopAllSluices() {
	sluicesLock.Lock()
	defer sluicesLock.Unlock()

	for network, sluice := range sluices {
		sluice.abandon()
		delete(sluices, network)
	}
}
