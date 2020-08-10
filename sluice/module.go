package sluice

import (
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/conf"
)

var (
	module *modules.Module
)

func init() {
	module = modules.Register("sluice", nil, start, stop, "base", "docks")
}

func start() error {
	if conf.Client() {
		StartStreamSluice("tcp4", "0.0.0.0:717")
		// StartPacketSluice("udp4", "127.0.0.17:717")
		StartStreamSluice("tcp6", "[::]:717")
		// StartPacketSluice("udp6", "[fd17::17]:717")
	}

	return nil
}

func stop() error {
	// TODO: dont use goroutines directly
	sluicesLock.Lock()
	for _, sluice := range sluices {
		go sluice.Abandon()
	}
	sluicesLock.Unlock()

	return nil
}
