package manager

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Safing/safing-core/meta"
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/port17/navigator"
)

func Bootstrap() (*navigator.Port, error) {

	address := meta.BootstrapNode()
	if address != "" {
		ip := net.ParseIP(address)
		if ip == nil {
			return nil, fmt.Errorf("could not parse IP '%s'", address)
		}

		var nodeAddress string
		if v4 := ip.To4(); v4 != nil {
			nodeAddress = fmt.Sprintf("%s:17", ip.String())
		} else {
			nodeAddress = fmt.Sprintf("[%s]:17", ip.String())
		}

		var n int
		var buf []byte
		var conn net.Conn
		var err error

		for i := 0; i < 10; i++ {
			conn, err = net.Dial("tcp", nodeAddress)
			if err != nil {
				continue
			}

			err = conn.SetDeadline(time.Now().Add(3 * time.Second))
			if err != nil {
				continue
			}

			n, err = conn.Write(SeagullIdentifier)
			if err != nil {
				continue
			}

			buf = make([]byte, 4096)
			n, err = conn.Read(buf)
			if err != nil && err != io.EOF {
				continue
			}

			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to contact bootstrap node: %s", err)
		}

		newBottle, err := bottle.LoadUntrustedBottle(buf[:n])
		if err != nil {
			return nil, fmt.Errorf("failed to parse bootstrap bottle: %s", err)
		}

		navigator.UpdatePublicBottle(newBottle)
		port := navigator.GetPublicPort(newBottle.PortName)
		if port == nil {
			return nil, errors.New("bootstrapping port didn't make it into navigator")
		}

		return port, nil

	}

	return nil, errors.New("failed to find bootstrap node")
}
