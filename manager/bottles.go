package manager

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/bottle"
	"github.com/safing/spn/identity"
	"github.com/safing/spn/mode"
	"github.com/safing/spn/navigator"
)

var (
	handlingNewBottle sync.Mutex

	db = database.NewInterface(nil)
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
		if bottle.UpdateLocalBottle(newBottle) {
			ForwardLocalBottle(newBottle)
		}
	} else {
		okAndContinue, storedBottle := bottle.ComparePublicBottle(newBottle)
		if okAndContinue {
			if storedBottle == nil || !storedBottle.IPv4.Equal(newBottle.IPv4) {
				if !verifyIPAddress(newBottle, 4) {
					bottle.DeletePublicBottle(newBottle.PortName)
					return
				}
			}
			if storedBottle == nil || !storedBottle.IPv6.Equal(newBottle.IPv6) {
				if !verifyIPAddress(newBottle, 6) {
					bottle.DeletePublicBottle(newBottle.PortName)
					return
				}
			}

			// save changes
			newBottle.Save()
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

	// get own ID
	var me *bottle.Bottle
	nodeMode := mode.Node()
	if nodeMode {
		me = identity.Public()
		if me == nil {
			return errors.New("could not got own ID for feeding to navigator")
		}
	}

	iter, err := db.Query(query.New(bottle.PublicBottles))
	if err != nil {
		return fmt.Errorf("failed to initialized feed: %s", err)
	}

	// update navigator
	var bottleCount int
	log.Trace("port17/manager: start feed")
	navigator.StartPublicReset()
	defer navigator.FinishPublicReset()
	if nodeMode {
		navigator.FeedPublicBottle(me)
	}
	for r := range iter.Next {
		b, err := bottle.EnsureBottle(r)
		if err != nil {
			log.Warningf("spn/navigator: could not parse bottle while feeding: %s", err)
			continue
		}

		bottleCount += 1
		navigator.FeedPublicBottle(b)
	}
	if iter.Err() != nil {
		return fmt.Errorf("failed to feed all bottles: %s", iter.Err())
	}

	if bottleCount == 0 {
		return errors.New("PublicBottleFeed did not return any bottles")
	}
	log.Infof("port17/manager: fed %d bottles to navigator", bottleCount)

	return nil
}
