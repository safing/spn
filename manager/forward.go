package manager

import (
	"github.com/safing/portbase/log"
	"github.com/safing/spn/bottle"
	"github.com/safing/spn/core"
)

func ForwardLocalBottle(new *bottle.Bottle) {
	// pack
	packedBottle, err := new.Pack()
	if err != nil {
		log.Warningf("port17: could forward local bottle, packing failed: %s", err)
	}

	// fling
	flingToAll(FlingBottle, packedBottle)

	// forward via local cranes
	// TODO: currently not handling local cranes
	// for _, craneController := range port17.GetAllControllers() {
	// 	craneController.UpdateBottle(packedBottle)
	// }
}

func ForwardPublicBottle(exportedBottle []byte, receivedByCrane string) {
	// forward via public cranes
	for _, craneController := range core.GetAllControllers() {
		if craneController.Crane.ID != receivedByCrane {
			craneController.UpdateBottle(exportedBottle)
		}
	}

	// forward in local network
	flingToAll(FlingBottle, exportedBottle)
}
