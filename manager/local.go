package manager

import (
	"time"

	"github.com/Safing/safing-core/network/environment"
)

func init() {
	go slingMaster()
}

func slingMaster() {
	time.Sleep(3 * time.Second)
	LetSeagullFly()
	for {
		select {
		case <-environment.NetworkChanged():
			LetSeagullFly()
		case <-time.After(10 * time.Minute):
			FlingMyBottle()
		}
	}
}
