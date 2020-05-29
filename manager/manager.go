package manager

import (
	"fmt"
	"net"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/spn/api"
	"github.com/safing/spn/bottle"
	"github.com/safing/spn/core"
	"github.com/safing/spn/identity"
	"github.com/safing/spn/mode"
	"github.com/safing/spn/navigator"
	"github.com/safing/spn/ships"
)

func init() {
	// log.Warning("run manager")
	go func() {
		time.Sleep(3 * time.Second)
		if mode.Client() {
			// log.Warning("run client")
			go primaryPortManager()
		}
		if mode.Node() {
			// log.Warning("run node")
			go networkOptimizer()
			go identityCleaner()
		}
	}()
}

func primaryPortManager() {
	var primaryPort *navigator.Port
	for {
		time.Sleep(1 * time.Second)

		if primaryPort == nil || !primaryPort.HasActiveRoute() {

			// find approximate network location
			ip, err := netenv.GetApproximateInternetLocation()
			if err != nil {
				log.Warningf("spn/manager: unable to get own location: %s", err)
				continue
			}

			col, err := navigator.FindNearestPorts([]net.IP{ip})
			if err != nil {
				log.Warningf("spn/manager: unable to find nearest port for primary port: %s", err)
				col = nil
			} else if col.Len() == 0 {
				log.Warning("spn/manager: no near ports could be found: will bootstrap.")
				col = nil
			}

			var port *navigator.Port
			if col != nil {
				port = col.All[0].Port
			} else {
				port, err = Bootstrap()
				if err != nil {
					log.Warningf("spn/manager: failed to bootstrap: %s", err)
					continue
				}
				log.Infof("spn/manager: bootstrap complete, will connect to %s: %s", port.Name(), port.Bottle)
			}

			// TODO: revamp start
			ship, err := ships.SetSail("TCP", fmt.Sprintf("%s:17", port.Bottle.IPv4))
			if err != nil {
				log.Warningf("spn/manager: could not set sail to %s:17: %s", port.Bottle.IPv4, err)
				continue
			}

			crane, err := core.NewCrane(ship, port.Bottle)
			if err != nil {
				log.Warningf("spn/manager: could not set up crane: %s", err)
				continue
			}
			crane.Initialize()
			core.AssignCrane(port.Name(), crane)
			// TODO: revamp end

			client, err := core.NewClient(core.NewInitializer(), port.Bottle)
			if err != nil {
				log.Warningf("spn/manager: unable to connect to primary port (%s): %s", port.Name(), err)
				continue
			}

			port.ActiveAPI = client
			// TODO: let API be managed
			primaryPort = port

			// set primary port in navigator
			navigator.SetPrimaryPort(port)
			log.Infof("spn/manager: set new primary port: %s", port.Name())

			// get bottles
			feeder(client)

		}
	}
}

func feeder(client *core.API) {
	call := client.BottleFeed()
	for {
		msg := <-call.Msgs
		switch msg.MsgType {
		case api.API_DATA:
			b, err := bottle.LoadTrustedBottle(msg.Container.CompileData())
			if err != nil {
				log.Warningf("failed to parse bottle from feed: %s", err)
			} else {
				b.Save()
				navigator.UpdatePublicBottle(b)
			}
		case api.API_END, api.API_ACK:
			return
		case api.API_ERR:
			log.Errorf("bottlefeed failed: %s", api.ParseError(msg.Container).Error())
			return
		}
	}
}

func networkOptimizer() {
	for {
		time.Sleep(10 * time.Second)
		myID := identity.Get()
		if myID != nil {
			newTarget, err := navigator.Optimize(myID.PortName)
			if err != nil {
				if err == navigator.ErrIAmLonely && bootstrapNode != "" {
					log.Warning("spn/manager: no known nodes, bootstrapping...")
					bsPort, err := Bootstrap()
					if err != nil {
						log.Warningf("spn/manager: failed to bootstrap: %s", err)
						continue
					}
					newTarget = bsPort.Bottle
				} else {
					log.Warningf("spn/manager: unable to optimize network: %s", err)
					continue
				}
			}
			if newTarget != nil {
				core.EstablishRoute(newTarget)
			}
		}
	}
}

func identityCleaner() {
	for {
		me := identity.Get()
		if me == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		active := core.GetAllControllers()

		// remove
		var remove []string

		for _, connection := range me.Connections {
			_, ok := active[connection.PortName]
			if !ok {
				remove = append(remove, connection.PortName)
			}
		}

		for _, toRemove := range remove {
			me.RemoveConnection(toRemove)
		}

		// add missing
		added := 0
		for remotePort, _ := range active {
			found := false
			for _, connection := range me.Connections {
				if connection.PortName == remotePort {
					found = true
					break
				}
			}
			if !found {
				// TODO: use provided function
				me.Connections = append(me.Connections, bottle.BottleConnection{
					PortName: remotePort,
					Cost:     0,
				})
				added += 1
			}
		}

		if len(remove) > 0 || added > 0 {
			identity.UpdateIdentity(me)
			if len(remove) > 0 {
				log.Warningf("spn/manager: removed %d inactive routes from own bottle", len(remove))
			}
			if added > 0 {
				log.Warningf("spn/manager: added %d missing routes to own bottle", added)
			}
		}

		time.Sleep(10 * time.Second)
	}
}
