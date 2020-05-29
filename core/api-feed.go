package core

import (
	"github.com/safing/portbase/container"
	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/api"
	"github.com/safing/spn/bottle"
)

var (
	db = database.NewInterface(nil)
)

func (portAPI *API) BottleFeed() *api.Call {
	return portAPI.Call(MsgTypeBottleFeed, container.New())
}

func (portAPI *API) handleBottleFeed(call *api.Call, c *container.Container) {
	go bottleFeeder(call)
}

func bottleFeeder(call *api.Call) {
	// get feed

	iter, err := db.Query(query.New(bottle.PublicBottles))
	if err != nil {
		call.SendError("could not initialize bottle feed")
		call.End()
		log.Warningf("spn/api: failed to initialize bottle feed: %s", err)
		return
	}

	// feed
	for r := range iter.Next {
		if call.IsEnded() {
			iter.Cancel()
			return
		}

		packed, err := r.Marshal(r, dsd.JSON)
		if err != nil {
			log.Warningf("spn/api: failed to pack bottle for feed: %s", err)
			continue
		}

		call.SendData(container.New(packed))
	}
	if iter.Err() != nil {
		call.SendError("error during feeding")
		log.Warningf("spn/api: error during bottle feed: %s", iter.Err())
	}

	call.End()
}
