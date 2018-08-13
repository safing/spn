package port17

import (
	"github.com/Safing/safing-core/container"
	"github.com/Safing/safing-core/log"
	"github.com/Safing/safing-core/port17/api"
	"github.com/Safing/safing-core/port17/bottlerack"
)

func (portAPI *API) BottleFeed() *api.Call {
	return portAPI.Call(MsgTypeBottleFeed, container.New())
}

func (portAPI *API) handleBottleFeed(call *api.Call, c *container.Container) {
	go bottleFeeder(call)
}

func bottleFeeder(call *api.Call) {
	// get feed
	feed, err := bottlerack.PublicBottleFeed()
	if err != nil {
		call.SendError("could not initialize bottle feed")
		call.End()
		log.Warningf("port17: failed to initialize bottle feed: %s", err)
		return
	}

	// feed
	for b := range feed {
		if call.IsEnded() {
			return
		}

		packed, err := b.Pack()
		if err != nil {
			log.Warningf("port17: failed to pack bottle for feed: %s", err)
			continue
		}

		call.SendData(container.New(packed))
	}

	call.End()
}
