package port17

import (
	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/api"
)

type PortRelay struct {
	LastConveyorBase

	call *api.Call
}

func StartPortRelay(call *api.Call, line *ConveyorLine) error {
	new := &PortRelay{
		call: call,
	}
	line.AddLastConveyor(new)

	return nil
}

func (pr *PortRelay) Run() {
	log.Tracef("relay: start")
	for {
		select {
		case msg := <-pr.call.Msgs:
			switch msg.MsgType {
			case api.API_DATA:
				log.Tracef("relay: forward %d bytes", msg.Container.Length())
				pr.toShip <- msg.Container
			case api.API_ERR:
				log.Warningf("port17: relay: got upstream error: %s", api.ParseError(msg.Container))
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
				log.Warningf("port17: relay: got downstream error: %s", c.ErrString())
				pr.call.SendError(c.ErrString())
				pr.call.End()
				return
			}
			log.Tracef("relay: return %d bytes", c.Length())
			pr.call.SendData(c)
		}
	}
}
