package manager

import (
	"github.com/safing/portbase/log"
	"github.com/safing/spn/bottle"
	"github.com/safing/spn/core"
	"github.com/safing/spn/identity"
	"github.com/safing/spn/navigator"
)

func init() {
	core.RegisterCraneHooks(updateBottleHook, distrustBottleHook, publishChanHook)
	identity.RegisterPublishHook(publishIdentity)
}

func updateBottleHook(controller *core.CraneController, newBottle *bottle.Bottle, exportedBottle []byte) error {
	handleBottle(newBottle, exportedBottle, controller.Crane.ID)
	return nil
}

func distrustBottleHook(controller *core.CraneController, newBottle *bottle.Bottle, exportedBottle []byte) error {
	// TODO
	return nil
}

func publishChanHook(controller *core.CraneController, newBottle *bottle.Bottle, exportedBottle []byte) error {
	// AddPublicCrane(controller, newBottle.PortName)
	core.AssignCrane(newBottle.PortName, controller.Crane)
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
