package entry

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/network/packet"
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
		LocalIP:  pkt.GetIPHeader().Src,
		DestIPs:  destIPs,
		Protocol: pkt.GetIPHeader().Protocol,
		Port:     pkt.GetTCPUDPHeader().DstPort,
	}

	tunnelID := fmt.Sprintf("%s-%s:%d", strings.ToLower(pkt.GetIPHeader().Protocol.String()), pkt.GetIPHeader().Src, pkt.GetTCPUDPHeader().SrcPort)
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
