package sluice

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/safing/portmaster/network"
	"github.com/safing/portmaster/network/packet"
)

var (
	// ErrUnsupported is returned when a protocol is not supported.
	ErrUnsupported = errors.New("unsupported protocol")

	// ErrSluiceOffline is returned when the sluice for a network is offline.
	ErrSluiceOffline = errors.New("is offline")
)

type Request struct {
	ConnInfo   *network.Connection
	CallbackFn RequestCallbackFunc
	Expires    time.Time
}

type RequestCallbackFunc func(connInfo *network.Connection, conn net.Conn)

func AwaitRequest(connInfo *network.Connection, callbackFn RequestCallbackFunc) error {
	network := getNetworkFromConnInfo(connInfo)
	if network == "" {
		return ErrUnsupported
	}

	sluice, ok := getSluice(network)
	if !ok {
		return fmt.Errorf("sluice for network %s %w", network, ErrSluiceOffline)
	}

	sluice.AwaitRequest(&Request{
		ConnInfo:   connInfo,
		CallbackFn: callbackFn,
	})
	return nil
}

func getNetworkFromConnInfo(connInfo *network.Connection) string {
	var network string

	// protocol
	switch connInfo.IPProtocol {
	case packet.TCP:
		network = "tcp"
	case packet.UDP:
		network = "udp"
	default:
		return ""
	}

	// IP version
	switch connInfo.IPVersion {
	case packet.IPv4:
		network += "4"
	case packet.IPv6:
		network += "6"
	default:
		return ""
	}

	return network
}
