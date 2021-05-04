package navigator

import (
	"context"
	"errors"
	"path"
	"time"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/spn/hub"
)

var (
	db = database.NewInterface(nil)
)

// InitializeFromDatabase loads all Hubs from the given database prefix and adds them to the Map.
func (m *Map) InitializeFromDatabase(databasePrefix string) {
	m.Lock()
	defer m.Unlock()

	// start query for Hubs
	iter, err := db.Query(query.New(databasePrefix))
	if err != nil {
		log.Warningf("spn/navigator: failed to start query for initialization feed of %s map: %s", m.Name, err)
		return
	}

	// update navigator
	var hubCount int
	log.Tracef("spn/navigator: starting to initialize %s map with data...", m.Name)
	for r := range iter.Next {
		h, err := hub.EnsureHub(r)
		if err != nil {
			log.Warningf("spn/navigator: could not parse Hub %q while initializing %s map: %s", r.Key(), m.Name, err)
			continue
		}

		hubCount += 1
		m.updateHub(h)
	}
	switch {
	case iter.Err() != nil:
		log.Warningf("spn/navigator: failed to (fully) initialize %s map: %s", m.Name, err)
	case hubCount == 0:
		log.Warningf("spn/navigator: no Hubs available for %s map - this is normal on first start", m.Name)
	default:
		log.Infof("spn/navigator: added %d Hubs to %s map", hubCount, m.Name)
	}
}

func (m *Map) SubscriptionFeeder(databasePrefix string) func(context.Context) error {
	return func(ctx context.Context) error {
		sub, err := db.Subscribe(query.New(databasePrefix))
		if err != nil {
			return err
		}

		for {
			select {
			case <-ctx.Done():
				sub.Cancel()
				return nil
			case r := <-sub.Feed:
				if r == nil {
					return errors.New("subscription ended")
				}

				if r.Meta().IsDeleted() {
					m.RemoveHub(path.Base(r.Key()))
					continue
				}

				// Get a fresh copy from the database in order to ensure that there are
				// no other references to it.
				fresh, err := hub.GetHubByKey(r.Key())
				if err != nil {
					log.Warningf("spn/navigator: subscription feeder on %s map failed to fetch fresh record of %s: %s", m.Name, r.Key(), err)
					continue
				}

				m.UpdateHub(fresh)
			}
		}
	}
}

// RemoveHub removes a Hub from the Map.
func (m *Map) RemoveHub(id string) {
	m.Lock()
	defer m.Lock()

	delete(m.All, id)
}

// UpdateHub updates a Hub on the Map.
func (m *Map) UpdateHub(h *hub.Hub) {
	m.Lock()
	defer m.Lock()

	m.updateHub(h)
}

func (m *Map) updateHub(h *hub.Hub) {
	h.Lock()
	defer h.Unlock()

	// Hub requires both Info and Status to be added to the Map.
	if h.Info == nil || h.Status == nil {
		return
	}

	// Create or update Pin.
	pin, ok := m.All[h.ID]
	if ok {
		pin.Hub = h
	} else {
		pin = &Pin{
			Hub:         h,
			ConnectedTo: make(map[string]*Lane),
		}
		m.All[h.ID] = pin
	}

	// Update the invalid status of the Pin.
	if pin.Hub.InvalidInfo || pin.Hub.InvalidStatus {
		pin.addStates(StateInvalid)
	} else {
		pin.removeStates(StateInvalid)
	}

	// Add/Update location data from IP addresses.
	pin.updateLocationData()

	// Update Statuses derived from Hub.
	m.updateStateSuperseded(pin)
	pin.updateStateHasRequiredInfo()
	pin.updateStateActive(time.Now().Unix())

	// Update Trust and Advisory Statuses.
	m.updateIntelStatuses(pin)

	// Update Hub load.
	pin.Load = pin.Hub.Status.Load

	// Mark all existing Lanes as inactive.
	for _, lane := range pin.ConnectedTo {
		lane.active = false
	}

	// Update Lanes (connections to other Hubs) from the Status.
laneLoop:
	for _, lane := range pin.Hub.Status.Lanes {
		// Check if this is a Lane to itself.
		if lane.ID == pin.Hub.ID {
			continue laneLoop
		}

		// First, get the Lane peer.
		peer, ok := m.All[lane.ID]
		if !ok {
			// We need to wait for peer to be added to the Map.
			continue laneLoop
		}

		m.updateHubLane(pin, lane, peer)
	}

	// Remove all inactive/abandoned Lanes from both Pins.
	var removedLanes bool
	for id, lane := range pin.ConnectedTo {
		if !lane.active {
			// Remove Lane from this Pin.
			delete(pin.ConnectedTo, id)
			removedLanes = true
			// Remove Lane from peer.
			peer, ok := m.All[id]
			if ok {
				delete(peer.ConnectedTo, pin.Hub.ID)
			}
		}
	}

	// Fully recalculate reachability if any Lanes were removed.
	if removedLanes {
		err := m.recalculateReachableHubs()
		if err != nil {
			log.Warningf("navigator: failed to recalculate reachable Hubs: %s", err)
		}
	}
}

// updateHubLane updates a lane between two Hubs on the Map.
// pin must already be locked, lane belongs to pin.
// peer will be locked by this function.
func (m *Map) updateHubLane(pin *Pin, lane *hub.Lane, peer *Pin) {
	peer.Hub.Lock()
	defer peer.Hub.Unlock()

	// Then get the corresponding Lane from that peer, if it exists.
	var peerLane *hub.Lane
	for _, possiblePeerLane := range peer.Hub.Status.Lanes {
		if possiblePeerLane.ID == pin.Hub.ID {
			peerLane = possiblePeerLane
			// We have found the corresponding peerLane, break the loop.
			break
		}
	}
	if peerLane == nil {
		// The peer obviously does not advertise a Lane to this Hub.
		// Maybe this is a fresh Lane, and the message has not yet reached us.
		// Alternatively, the Lane could have been recently removed.

		// Abandon this Lane for now.
		delete(pin.ConnectedTo, peer.Hub.ID)
		return
	}

	// Calculate matching Capacity and Latency values between both Hubs.
	combinedCapacity := lane.Capacity
	if peerLane.Capacity < combinedCapacity {
		// For Capacity, use the lesser value of both.
		combinedCapacity = peerLane.Capacity
	}
	combinedLatency := lane.Latency
	if peerLane.Latency > combinedLatency {
		// For Latency, use the greater value of both.
		combinedLatency = peerLane.Latency
	}

	// Add Lane to both Pins and override old values in the process.
	pin.ConnectedTo[peer.Hub.ID] = &Lane{
		Pin:      peer,
		Capacity: combinedCapacity,
		Latency:  combinedLatency,
		active:   true,
	}
	peer.ConnectedTo[pin.Hub.ID] = &Lane{
		Pin:      pin,
		Capacity: combinedCapacity,
		Latency:  combinedLatency,
		active:   true,
	}

	// Check for reachability.
	switch {
	case pin.State.has(StateReachable):
		peer.addStates(StateReachable)
	case peer.State.has(StateReachable):
		pin.addStates(StateReachable)
	}
}

func (m *Map) updateStates(ctx context.Context, task *modules.Task) error {
	now := time.Now()
	oneMonthAgo := now.Add(-33 * 24 * time.Hour).Unix()

	for _, pin := range m.All {
		// Update StateFailing
		if pin.State.has(StateFailing) && now.After(pin.FailingUntil) {
			pin.removeStates(StateFailing)
		}

		// Delete stale Hubs that haven't been updated in over a month.
		if pin.Hub.Info.Timestamp < oneMonthAgo &&
			pin.Hub.Status.Timestamp < oneMonthAgo {
			err := db.Delete(pin.Hub.Key())
			if err != nil {
				log.Warningf("spn/navigator: failed to delete stale %s: %s", pin.Hub, err)
			}
		}
	}

	// Update StateActive
	m.updateActiveHubs()

	// Update StateReachable
	return m.recalculateReachableHubs()
}
