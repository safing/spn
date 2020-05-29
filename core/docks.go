package core

import (
	"sync"

	"github.com/safing/spn/identity"
)

var (
	docks     = make(map[string]*Crane)
	docksLock sync.RWMutex
)

func GetAssignedCrane(portName string) *Crane {
	docksLock.RLock()
	defer docksLock.RUnlock()
	crane, ok := docks[portName]
	if ok {
		return crane
	}
	return nil
}

func AssignCrane(portName string, crane *Crane) {
	docksLock.Lock()
	defer docksLock.Unlock()
	docks[portName] = crane
}

func RetractCraneByDestination(portName string) {
	docksLock.Lock()
	defer docksLock.Unlock()
	delete(docks, portName)
	identity.RemoveConnection(portName)
}

func RetractCraneByID(craneID string) {
	docksLock.Lock()
	defer docksLock.Unlock()
	for destination, crane := range docks {
		if crane.ID == craneID {
			delete(docks, destination)
			identity.RemoveConnection(destination)
			return
		}
	}
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
