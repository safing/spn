package docks

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/network/packet"
	"github.com/safing/spn/api"
)

type TunnelRequest struct {
	Domain   string
	IP       net.IP
	Protocol packet.IPProtocol
	Port     uint16
}

func (request *TunnelRequest) Network() string {
	return strings.ToLower(request.Protocol.String())
}

func (request *TunnelRequest) Address() string {
	if request.Protocol == packet.TCP || request.Protocol == packet.UDP {
		if v4 := request.IP.To4(); v4 != nil {
			return fmt.Sprintf("%s:%d", request.IP, request.Port)
		} else {
			return fmt.Sprintf("[%s]:%d", request.IP, request.Port)
		}
	}
	return request.IP.String()
}

func (request *TunnelRequest) String() string {
	if request.Domain != "" {
		return fmt.Sprintf("%s (%s:%s)", request.Domain, request.Network(), request.Address())
	}
	return fmt.Sprintf("%s:%s", request.Network(), request.Address())
}

func (portAPI *API) Tunnel(domain string, ip net.IP, protocol packet.IPProtocol, port uint16) (tunnel *api.Call, err error) {
	request := &TunnelRequest{
		Domain:   domain,
		IP:       ip,
		Protocol: protocol,
		Port:     port,
	}
	dumped, err := dsd.Dump(request, dsd.JSON)
	if err != nil {
		return nil, err
	}

	call := portAPI.Call(MsgTypeTunnel, container.New(dumped))
	return call, nil
}

func (portAPI *API) handleTunnel(call *api.Call, c *container.Container) {
	request := &TunnelRequest{}
	_, err := dsd.Load(c.CompileData(), request)
	if err != nil {
		log.Warningf("port17: failed to parse tunnel request: %s", err)
		call.SendError("failed to parse request")
		call.End()
		return
	}
	if request.IP == nil || request.Protocol == 0 || request.Port == 0 {
		call.SendError("invalid request")
		call.End()
		return
	}
	if request.Protocol != packet.TCP && request.Protocol != packet.UDP {
		call.SendError("unsupported protocol")
		call.End()
		return
	}

	log.Infof("port17: received tunnel request for %s", request)

	conn, err := net.Dial(request.Network(), request.Address())
	if err != nil {
		log.Warningf("port17: failed to connect tunnel: %s", err)
		call.SendError("could not connect to requested address")
		call.End()
		return
	}

	log.Infof("port17: connected tunnel to %s", request)

	go tunnelWriter(call, conn, request)
	go tunnelReader(call, conn, request)

}

func tunnelReader(call *api.Call, conn net.Conn, request *TunnelRequest) {

	var buf []byte
	var n int
	var err error

	for {
		buf = make([]byte, 4096)
		n, err = conn.Read(buf)
		if err != nil && !call.IsEnded() {
			if err == io.EOF {
				log.Infof("port17: tunnel to %s closed.", request)
			} else {
				log.Infof("port17: could not read packet, killing tunnel to %s: %s", request, err)
			}
			call.End()
			conn.Close()
			return
		}
		call.SendData(container.New(buf[:n]))
	}
}

func tunnelWriter(call *api.Call, conn net.Conn, request *TunnelRequest) {

	var msg *api.ApiMsg
	var packetLength int
	var written int
	var n int
	var err error

	for msg = range call.Msgs {
		switch msg.MsgType {
		case api.API_DATA:
			packetLength = msg.Container.Length()
			written = 0
			for written < packetLength {
				n, err = conn.Write(msg.Container.CompileData()[written:])
				if err != nil && !call.IsEnded() {
					if err == io.EOF {
						log.Infof("port17: tunnel to %s closed.", request)
					} else {
						log.Infof("port17: could not write packet, killing tunnel to %s: %s", request, err)
					}
					call.End()
					conn.Close()
					return
				}
				written += n
			}
		case api.API_END, api.API_ACK:
			conn.Close()
			return
		case api.API_ERR:
			log.Infof("port17: tunnel broke with tunnel error: %s", api.ParseError(msg.Container).Error())
			conn.Close()
			return
		}
	}
}
