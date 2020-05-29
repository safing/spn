package manager

import (
	"time"
)

func init() {
	go slingMaster()
}

func slingMaster() {
	time.Sleep(3 * time.Second)
	LetSeagullFly()
	for {
		select {
		case <-time.After(1 * time.Minute): // FIXME: on network change
			LetSeagullFly()
		case <-time.After(10 * time.Minute):
			FlingMyBottle()
		}
	}
}
