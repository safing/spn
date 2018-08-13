package port17

import (
	"fmt"

	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/port17/ships"
)

func EstablishRoute(target *bottle.Bottle) {

	if GetAssignedCrane(target.PortName) != nil {
		log.Warningf("port17: tried to establish route to %s, but one already exists", target.PortName)
		return
	}

	log.Warningf("port17: establishing new route to %s", target)

	var ship ships.Ship
	var err error

	if target.IPv6 != nil {
		shipType := "TCP"
		address := fmt.Sprintf("[%s]:17", target.IPv6)
		ship, err = ships.SetSail(shipType, address)
		if err != nil {
			log.Warningf("port17: failed to set sail to %s with %s(%s)", target.PortName, shipType, address)
			ship = nil
		}
	}
	if ship == nil && target.IPv4 != nil {
		shipType := "TCP"
		address := fmt.Sprintf("%s:17", target.IPv4)
		ship, err = ships.SetSail(shipType, address)
		if err != nil {
			log.Warningf("port17: failed to set sail to %s with %s(%s)", target.PortName, shipType, address)
			ship = nil
		}
	}

	if ship == nil {
		log.Warningf("port17: unable to establish route to %s: could not set sail", target.PortName)
		return
	}

	crane, err := NewCrane(ship, target)
	if err != nil {
		log.Warningf("port17: unable to establish route to %s: failed to build crane", target.PortName)
		return
	}

	crane.Initialize()
	if crane.stopped.IsSet() {
		return
	}

	err = crane.Controller.PublishChannel()
	if err != nil {
		log.Warningf("port17: failed to publish channel: %s")
		return
	}

}
