package manager

import (
	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17"
	"github.com/Safing/safing-core/port17/bottle"
	"github.com/Safing/safing-core/port17/identity"
	"github.com/Safing/safing-core/port17/navigator"
)

func init() {
	port17.RegisterCraneHooks(updateBottleHook, distrustBottleHook, publishChanHook)
	identity.RegisterPublishHook(publishIdentity)
}

func updateBottleHook(controller *port17.CraneController, newBottle *bottle.Bottle, exportedBottle []byte) error {
	handleBottle(newBottle, exportedBottle, controller.Crane.ID)
	return nil
}

func distrustBottleHook(controller *port17.CraneController, newBottle *bottle.Bottle, exportedBottle []byte) error {
	// TODO
	return nil
}

func publishChanHook(controller *port17.CraneController, newBottle *bottle.Bottle, exportedBottle []byte) error {
	// AddPublicCrane(controller, newBottle.PortName)
	port17.AssignCrane(newBottle.PortName, controller.Crane)
	myID := identity.Get()
	myID.AddConnection(newBottle, 0)
	identity.UpdateIdentity(myID)

	log.Infof("port17/manager: established and published route to %s", newBottle.PortName)

	return nil
}

func publishIdentity() {
	pubID := identity.Public()
	if pubID != nil {
		navigator.UpdatePublicBottle(pubID)
	}

	exportedIdentity, err := identity.Export()
	if err != nil {
		log.Warningf("port17/manager: could not export identity: %s", err)
		return
	}
	if len(exportedIdentity) == 0 {
		log.Warning("port17/manager: identity export is empty")
		return
	}
	ForwardPublicBottle(exportedIdentity, "")
}
