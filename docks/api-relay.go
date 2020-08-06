package docks

import (
	"github.com/safing/portbase/log"
	"github.com/safing/spn/api"
)

type HubRelay struct {
	LastConveyorBase

	call *api.Call
}

func (pr *HubRelay) Run() {
	log.Tracef("spn/docks: relay started")
	for {
		select {
		case msg := <-pr.call.Msgs:
			switch msg.MsgType {
			case api.API_DATA:
				log.Tracef("spn/docks: relay forwarded %d bytes", msg.Container.Length())
				pr.toShip <- msg.Container
			case api.API_ERR:
				log.Warningf("spn/docks: relay got upstream error: %s", api.ParseError(msg.Container))
				close(pr.toShip)
			default:
				close(pr.toShip)
			}
		case c := <-pr.fromShip:
			if c == nil {
				close(pr.toShip)
				pr.call.End()
				return
			}
			if c.HasError() {
				close(pr.toShip)
				log.Warningf("spn/docks: relay got downstream error: %s", c.ErrString())
				pr.call.SendError(c.ErrString())
				pr.call.End()
				return
			}
			log.Tracef("spn/docks: relay returned %d bytes", c.Length())
			pr.call.SendData(c)
		}
	}
}
