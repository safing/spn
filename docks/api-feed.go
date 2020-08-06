package docks

import (
	"fmt"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/api"
	"github.com/safing/spn/hub"
)

const (
	HubFeedAnnouncement = 1
	HubFeedStatus       = 2
	HubFeedDistrust     = 3 // TODO
)

var (
	db = database.NewInterface(nil)
)

func (portAPI *API) PublicHubFeed() *api.Call {
	return portAPI.Call(MsgTypePublicHubFeed, container.New())
}

func (portAPI *API) handlePublicHubFeed(call *api.Call, c *container.Container) {
	go publicHubFeeder(call)
}

func publicHubFeeder(call *api.Call) {
	// announcement feed
	ok := hubMsgFeeder(call, hub.ScopePublic, "announcement", HubFeedAnnouncement)

	// status feed
	if ok {
		hubMsgFeeder(call, hub.ScopePublic, "status", HubFeedStatus)
	}

	call.End()
}

func hubMsgFeeder(call *api.Call, scope hub.Scope, dataType string, msgType int) (ok bool) {
	iter, err := db.Query(query.New(fmt.Sprintf(
		"%s%s/%s/",
		hub.RawMsgsScope,
		scope,
		dataType,
	)))
	if err != nil {
		call.SendError(fmt.Sprintf("could not initialize %s hub %s feed", scope, dataType))
		log.Warningf("spn/api: failed to initialize %s hub %s feed: %s", scope, dataType, err)
		return false
	}

	// feed
	for r := range iter.Next {
		if call.IsEnded() {
			iter.Cancel()
			return false
		}

		hubMsg, err := hub.EnsureHubMsg(r)
		if err != nil {
			log.Warningf("spn/api: failed to pack bottle for feed: %s", err)
			continue
		}

		data := container.New()
		data.AppendInt(msgType)
		data.AppendAsBlock(hubMsg.Data)
		call.SendData(data)
	}
	if iter.Err() != nil {
		call.SendError("error during hub msg feed")
		log.Warningf("spn/api: error during hub msg feed: %s", iter.Err())
		return false
	}

	return true
}
