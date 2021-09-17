package sluice

import (
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/conf"
)

var (
	module *modules.Module

	entrypointInfoMsg = []byte("You have reached the local SPN entry port, but your connection could not be matched to an SPN tunnel.\n")
)

func init() {
	module = modules.Register("sluice", nil, start, stop, "base")
}

func start() error {
	// TODO:
	// Listening on all interfaces for now, as we need this for Windows.
	// Handle similarly to the nameserver listener.

	if conf.Client() {
		StartSluice("tcp4", "0.0.0.0:717")
		StartSluice("udp4", "0.0.0.0:717")
		StartSluice("tcp6", "[::]:717")
		StartSluice("udp6", "[::]:717")
	}

	return nil
}

func stop() error {
	stopAllSluices()
	return nil
}
