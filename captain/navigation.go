package captain

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/safing/spn/access"
	"github.com/safing/spn/hub"

	"github.com/safing/spn/conf"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/navigator"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/spn/api"
)

func primaryHubManager(ctx context.Context) (err error) {
	var primaryPort *navigator.Port
	defer ready.UnSet()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(1 * time.Second):
		}

		if primaryPort == nil || !primaryPort.HasActiveRoute() {
			if ready.SetToIf(true, false) {
				log.Infof("spn/captain: client not ready")
			}

			module.Hint(
				"spn:establishing-primary-hub",
				"SPN Starting",
				"Connecting to the SPN network is in progress.",
			)
			primaryPort, err = establishPrimaryHub(ctx)
			if err != nil {
				log.Warningf("failed to establish connection to primary hub: %s", err)
				module.Warning(
					"spn:primary-hub-failure",
					"SPN Failed to Start",
					fmt.Sprintf("Failed to connect to a primary hub: %s. The Portmaster will retry to connect automatically.", err),
				)
				select {
				case <-ctx.Done():
				case <-time.After(5 * time.Second):
				}
				continue
			}

			// success!
			module.Hint(
				"spn:connected-to-primary-hub",
				"SPN Online",
				fmt.Sprintf("You are connected to the SPN network with the Hub at %s. This notification is for awareness that the SPN is active during the alpha testing phase.", primaryPort.Hub.Info.IPv4),
			)
			ready.Set()
			log.Infof("spn/captain: established new primary %s", primaryPort.Hub)
			log.Infof("spn/captain: client is ready")
		}
	}
}

func establishPrimaryHub(ctx context.Context) (*navigator.Port, error) {
	var primaryPortCandidate *navigator.Port
	var bootstrapped bool

findCandidate:
	// find approximate network location
	ip, err := netenv.GetApproximateInternetLocation()
	if err != nil {
		log.Warningf("unable to get own location: %s", err)

		// could not get location, use random port instead
		primaryPortCandidate = navigator.GetRandomPort()

	} else {
		// find nearest ports to location
		col, err := navigator.FindNearestPorts([]net.IP{ip})
		if err != nil {
			log.Warningf("spn/captain: unable to find nearest port for primary port: %s", err)
			col = nil
		} else if col.Len() == 0 {
			log.Warning("spn/captain: no near ports could be found: will bootstrap.")
			col = nil
		}

		// set candidate if there is a result
		if col != nil {
			primaryPortCandidate = col.All[0].Port
		}
	}

	// bootstrap if no Port could be found
	if primaryPortCandidate == nil {
		if bootstrapped {
			return nil, fmt.Errorf("unable to find a primary hub")
		}

		// bootstrap to the network!
		err := bootstrapWithUpdates()
		if err != nil {
			return nil, fmt.Errorf("failed to bootstrap: %w", err)
		}
		log.Infof("spn/captain: bootstrap successful")

		// try again
		bootstrapped = true
		goto findCandidate
	}

	// connect
	ship, err := docks.LaunchShip(ctx, primaryPortCandidate.Hub, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to primary %s: %w", primaryPortCandidate.Hub, err)
	}

	// establish crane
	crane, err := docks.NewCrane(ship, publicIdentity, primaryPortCandidate.Hub)
	if err != nil {
		return nil, fmt.Errorf("failed to create crane for primary Hub: %w", err)
	}
	err = crane.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start crane for primary Hub: %w", err)
	}
	docks.AssignCrane(primaryPortCandidate.Hub.ID, crane)

	// make client
	client, err := docks.NewClient(conf.CurrentVersion, primaryPortCandidate.Hub)
	if err != nil {
		return nil, fmt.Errorf("unable to create client at primary %s: %w", primaryPortCandidate.Hub, err)
	}

	// get access code
	accessCode, err := access.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get access code: %w", err)
	}

	// authenticate
	err = client.UserAuth(accessCode)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate to primary %s: %w", primaryPortCandidate.Hub, err)
	}

	primaryPortCandidate.ActiveAPI = client
	// TODO: let API be managed

	// set primary port in navigator
	navigator.SetPrimaryPort(primaryPortCandidate)

	// get bottles
	feedHubs(client)

	return primaryPortCandidate, nil
}

func feedHubs(client *docks.API) {
	// feed
	call := client.PublicHubFeed()
	for {
		select {
		case <-time.After(5 * time.Second):
			call.End()
			log.Warning("spn/captain: feeding Hubs: timed out")
			return
		case msg := <-call.Msgs:
			switch msg.MsgType {
			case api.API_DATA:

				msgType, err := msg.Container.GetNextN64()
				if err != nil {
					log.Warningf("spn/captain: feeding Hubs: failed to get message type: %s", err)
					continue
				}
				msgData, err := msg.Container.GetNextBlock()
				if err != nil {
					log.Warningf("spn/captain: feeding Hubs: failed to get message data: %s", err)
					continue
				}

				switch msgType {
				case docks.HubFeedAnnouncement:
					err = hub.ImportAnnouncement(msgData, hub.ScopePublic)
					if err != nil {
						log.Warningf("spn/captain: feeding Hubs: failed to import announcement: %s", err)
					}
				case docks.HubFeedStatus:
					err = hub.ImportStatus(msgData, hub.ScopePublic)
					if err != nil {
						log.Warningf("spn/captain: feeding Hubs: failed to import status: %s", err)
					}
				default:
					log.Warningf("spn/captain: feeding Hubs: unknown msg type: %d", msgType)
				}

			case api.API_END, api.API_ACK:
				return
			case api.API_ERR:
				log.Errorf("spn/captain: feeding Hubs failed: %s", api.ParseError(msg.Container).Error())
				return
			}
		}
	}
}

func optimizeNetwork(ctx context.Context, task *modules.Task) error {
	if publicIdentity == nil {
		return nil
	}

	newDst, err := navigator.Optimize(publicIdentity.Hub())
	switch err {
	case nil:
		// continue
	case navigator.ErrIAmLonely:
		// bootstrap to the network!
		err := bootstrapWithUpdates()
		if err != nil {
			return err
		}
		// try again
		newDst, err = navigator.Optimize(publicIdentity.Hub())
		if err != nil {
			return err
		}
	default:
		return err
	}

	if newDst != nil {
		log.Infof("spn/captain: network optimization suggests new connection to %s", newDst)
		docks.EstablishRoute(publicIdentity, newDst)
	} else {
		log.Info("spn/captain: network optimization suggests no further action")
	}

	return nil
}
