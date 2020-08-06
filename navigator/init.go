package navigator

import (
	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/hub"
)

var (
	db = database.NewInterface(nil)
)

// InitializeNavigator loads all Hubs and adds them to the navigator.
func InitializeNavigator() {
	publicPortsLock.Lock()
	defer publicPortsLock.Unlock()

	// start query for Hubs
	iter, err := db.Query(query.New(hub.PublicHubs))
	if err != nil {
		log.Warningf("spn/navigator: failed to start query for initialization: %s", err)
		return
	}

	// update navigator
	var hubCount int
	log.Trace("spn/navigator: starting to feed navigator with data...")
	for r := range iter.Next {
		h, err := hub.EnsureHub(r)
		if err != nil {
			log.Warningf("spn/navigator: could not parse Hub %q while feeding: %s", r.Key(), err)
			continue
		}

		hubCount += 1
		updateHub(publicPorts, nil, h)
	}
	switch {
	case iter.Err() != nil:
		log.Warningf("spn/navigator: failed to add Hubs to the navigator: %s", err)
	case hubCount == 0:
		log.Warningf("spn/navigator: no Hubs available for the navigator - this is normal on first start")
	default:
		log.Infof("spn/navigator: added %d Hubs to the navigator", hubCount)
	}
}
