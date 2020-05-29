package entry

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/network/packet"
)

type TunnelInfo struct {
	Domain string

	LocalIP net.IP
	DestIPs []net.IP

	Protocol packet.IPProtocol
	Port     uint16
}

var (
	tunnelInfos    = make(map[string]*TunnelInfo)
	tunnelInfoLock sync.Mutex
)

func CreateTunnel(pkt packet.Packet, domain string, destIPs []net.IP) {
	tunnelInfoLock.Lock()
	defer tunnelInfoLock.Unlock()

	tunnelInfo := &TunnelInfo{
		Domain:   domain,
		LocalIP:  pkt.Info().Src,
		DestIPs:  destIPs,
		Protocol: pkt.Info().Protocol,
		Port:     pkt.Info().DstPort,
	}

	tunnelID := fmt.Sprintf("%s-%s:%d", strings.ToLower(pkt.Info().Protocol.String()), pkt.Info().Src, pkt.Info().SrcPort)
	log.Infof("port17/manager: incoming tunnel: %s", tunnelID)

	tunnelInfos[tunnelID] = tunnelInfo

	// info, ok := tunnelInfos[getLocalIdentifier(conn)]
}

func GetTunnelInfo(network, address string) *TunnelInfo {
	tunnelInfoLock.Lock()
	defer tunnelInfoLock.Unlock()
	tunnelID := fmt.Sprintf("%s-%s", network, address)
	log.Infof("port17/manager: checking for tunnel: %s", tunnelID)
	tunnelInfo, ok := tunnelInfos[tunnelID]
	if ok {
		return tunnelInfo
	}
	return nil
}
