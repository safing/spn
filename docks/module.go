package docks

import (
	"sync"

	"github.com/safing/portbase/modules"
	"github.com/safing/spn/hub"
)

var (
	module *modules.Module

	docks     = make(map[string]*Crane)
	docksLock sync.RWMutex
)

func init() {
	module = modules.Register("docks", nil, nil, nil, "base", "cabin", "access-codes")
}

func GetAssignedCrane(hubID string) *Crane {
	docksLock.RLock()
	defer docksLock.RUnlock()
	crane, ok := docks[hubID]
	if ok {
		return crane
	}
	return nil
}

func AssignCrane(hubID string, crane *Crane) {
	docksLock.Lock()
	defer docksLock.Unlock()
	docks[hubID] = crane
}

func RetractCraneByDestination(hubID string) {
	docksLock.Lock()
	defer docksLock.Unlock()
	delete(docks, hubID)
}

func RetractCraneByID(craneID string) (connectedHub *hub.Hub) {
	docksLock.Lock()
	defer docksLock.Unlock()
	for hubID, crane := range docks {
		if crane.ID == craneID {
			delete(docks, hubID)
			return crane.ConnectedHub
		}
	}
	return nil
}

func GetAllControllers() map[string]*CraneController {
	new := make(map[string]*CraneController)
	docksLock.Lock()
	defer docksLock.Unlock()
	for destination, crane := range docks {
		new[destination] = crane.Controller
	}
	return new
}
