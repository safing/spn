package manager

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/port17/bottlerack"
	"github.com/Safing/safing-core/port17/identity"
	"github.com/Safing/safing-core/port17/mode"
	"github.com/Safing/safing-core/port17/navigator"
)

var (
	handlingNewBottle sync.Mutex
)

func init() {
	// port17.RegisterCraneHooks()

	go func() {
		time.Sleep(3 * time.Second)
		err := FeedTheNavigator()
		if err != nil {
			log.Infof("port17/manager: failed to feed the navigator: %s", err)
			time.Sleep(10 * time.Second)
			err := FeedTheNavigator()
			if err != nil {
				log.Warningf("port17/manager: failed to feed the navigator again: %s", err)
			}
		}
	}()
}

func handleFlungBottle(conn net.PacketConn, raddr net.Addr, packet []byte) (isBottle bool) {
	if bytes.HasPrefix(packet, BottleIdentifier) {
		isBottle = true
		log.Tracef("port17: handling Bottle from %s", raddr.String())

		// parse bottle
		newBottle, err := bottle.LoadUntrustedBottle(packet[len(BottleIdentifier):])
		if err != nil {
			log.Infof("port17: could not parse bottle: %s", err)
			return
		}
		// log.Tracef("port17: parsed bottle: %s", newBottle)

		if newBottle.Local != nil {
			// add IP
			host, _, err := net.SplitHostPort(raddr.String())
			if err != nil {
				log.Infof("port17: could not parse source hostport of bottle: %s", err)
				return
			}

			ip := net.ParseIP(host)
			if ip == nil {
				log.Infof("port17: could not parse IP of bottle raddr: %s", raddr.String())
				return
			}

			if ip.To4() != nil {
				newBottle.IPv4 = ip
			} else {
				newBottle.IPv6 = ip
			}
		}

		handleBottle(newBottle, packet[len(BottleIdentifier):], "")

	}
	return
}

func handleStreamBottle(conn net.Conn, packet []byte) (isBottle bool) {
	if bytes.HasPrefix(packet, BottleIdentifier) {
		isBottle = true
		log.Tracef("port17: handling Bottle from %s", conn.RemoteAddr().String())

		// parse bottle
		newBottle, err := bottle.LoadUntrustedBottle(packet[len(BottleIdentifier):])
		if err != nil {
			log.Infof("port17: could not parse bottle: %s", err)
			return
		}
		// log.Tracef("port17: parsed bottle: %s", newBottle)

		if newBottle.Local != nil {
			// add IP
			host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
			if err != nil {
				log.Infof("port17: could not parse source hostport of bottle: %s", err)
				return
			}

			ip := net.ParseIP(host)
			if ip == nil {
				log.Infof("port17: could not parse IP of bottle raddr: %s", conn.RemoteAddr().String())
				return
			}

			if ip.To4() != nil {
				newBottle.IPv4 = ip
			} else {
				newBottle.IPv6 = ip
			}
		}

		handleBottle(newBottle, packet[len(BottleIdentifier):], "")

	}
	return
}

func handleBottle(newBottle *bottle.Bottle, exportedBottle []byte, receivedByCrane string) {
	// only handle one new bottle at a time
	handlingNewBottle.Lock()
	defer handlingNewBottle.Unlock()

	// be sure we ignore our own updates
	myID := identity.Get()
	if myID.PortName == newBottle.PortName {
		log.Tracef("port17/manager: received own bottle, ignoring")
		return
	}

	if newBottle.Local != nil {
		if bottlerack.UpdateLocalBottle(newBottle) {
			ForwardLocalBottle(newBottle)
		}
	} else {
		okAndContinue, storedBottle := bottlerack.ComparePublicBottle(newBottle)
		if okAndContinue {
			if storedBottle == nil || !storedBottle.IPv4.Equal(newBottle.IPv4) {
				if !verifyIPAddress(newBottle, 4) {
					bottlerack.DiscardPublicBottle(newBottle.PortName)
					return
				}
			}
			if storedBottle == nil || !storedBottle.IPv6.Equal(newBottle.IPv6) {
				if !verifyIPAddress(newBottle, 6) {
					bottlerack.DiscardPublicBottle(newBottle.PortName)
					return
				}
			}

			// save changes
			bottlerack.SavePublicBottle(newBottle)
			// forward
			ForwardPublicBottle(exportedBottle, receivedByCrane)
			// update navigator
			navigator.UpdatePublicBottle(newBottle)
		}
	}

}

func verifyIPAddress(b *bottle.Bottle, ipVersion uint8) (ok bool) {

	// ship := ships.SetSail(b., address)
	// FIXME: actually do some checking here

	return true
}

// FeedTheNavigator loads all public bottles and feeds them to the navigator
func FeedTheNavigator() error {
	// get feed
	feed, err := bottlerack.PublicBottleFeed()
	if err != nil {
		return err
	}

	// get own ID
	var me *bottle.Bottle
	nodeMode := mode.Node()
	if nodeMode {
		me = identity.Public()
		if me == nil {
			return errors.New("could not got own ID for feeding to navigator")
		}
	}

	// update navigator
	var bottleCount int
	log.Trace("port17/manager: start feed")
	navigator.StartPublicReset()
	if nodeMode {
		navigator.FeedPublicBottle(me)
	}
	for b := range feed {
		log.Trace("port17/manager: feeding...")
		bottleCount += 1
		navigator.FeedPublicBottle(b)
	}
	navigator.FinishPublicReset()
	log.Trace("port17/manager: end feed")

	if bottleCount == 0 {
		return errors.New("PublicBottleFeed did not return any bottles")
	}
	log.Infof("port17/manager: fed %d bottles to navigator", bottleCount)

	return nil
}
