package sluice

import (
	"errors"
	"fmt"

	"github.com/safing/portmaster/network/packet"
)

var (
	// ErrUnsupported is returned when a protocol is not supported.
	ErrUnsupported = errors.New("unsupported protocol")

	// ErrSluiceOffline is returned when the sluice for a network is offline.
	ErrSluiceOffline = errors.New("is offline")
)

type Request struct {
	Domain string
	Info   *packet.Info
}

func AwaitRequest(pkt *packet.Info, domain string) error {
	network := getNetwork(pkt)
	if network == "" {
		return ErrUnsupported
	}

	sluicesLock.Lock()
	sluice, ok := sluices[network]
	sluicesLock.Unlock()
	if !ok {
		return fmt.Errorf("sluice for network %s %w", network, ErrSluiceOffline)
	}

	sluice.AwaitRequest(&Request{
		Domain: domain,
		Info:   pkt,
	})
	return nil
}

func getNetwork(pkt *packet.Info) string {
	var network string

	// protocol
	switch pkt.Protocol {
	case packet.TCP:
		network = "tcp"
	case packet.UDP:
		network = "udp"
	default:
		return ""
	}

	// IP version
	switch pkt.Version {
	case packet.IPv4:
		network += "4"
	case packet.IPv6:
		network += "6"
	default:
		return ""
	}

	return network
}
