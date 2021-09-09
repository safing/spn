package sluice

import (
	"errors"
	"fmt"
	"net"

	"github.com/safing/portmaster/network/packet"
)

var (
	// ErrUnsupported is returned when a protocol is not supported.
	ErrUnsupported = errors.New("unsupported protocol")

	// ErrSluiceOffline is returned when the sluice for a network is offline.
	ErrSluiceOffline = errors.New("is offline")
)

type Request struct {
	Domain     string
	Network    string
	Info       *packet.Info
	CallbackFn RequestCallbackFunc
}

type RequestCallbackFunc func(r *Request, conn net.Conn)

func AwaitRequest(pkt *packet.Info, domain string, callbackFn RequestCallbackFunc) error {
	network := getNetworkFromPacket(pkt)
	if network == "" {
		return ErrUnsupported
	}

	sluice, ok := getSluice(network)
	if !ok {
		return fmt.Errorf("sluice for network %s %w", network, ErrSluiceOffline)
	}

	sluice.AwaitRequest(&Request{
		Domain:     domain,
		Network:    network,
		Info:       pkt,
		CallbackFn: callbackFn,
	})
	return nil
}

func getNetworkFromPacket(pkt *packet.Info) string {
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
