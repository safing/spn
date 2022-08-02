package navigator

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/database/record"
	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/spn/hub"
)

var db = database.NewInterface(&database.Options{
	Local:    true,
	Internal: true,
})

// InitializeFromDatabase loads all Hubs from the given database prefix and adds them to the Map.
func (m *Map) InitializeFromDatabase() {
	m.Lock()
	defer m.Unlock()

	// start query for Hubs
	iter, err := db.Query(query.New(hub.MakeHubDBKey(m.Name, "")))
	if err != nil {
		log.Warningf("spn/navigator: failed to start query for initialization feed of %s map: %s", m.Name, err)
		return
	}

	// update navigator
	var hubCount int
	log.Tracef("spn/navigator: starting to initialize %s map from database", m.Name)
	for r := range iter.Next {
		h, err := hub.EnsureHub(r)
		if err != nil {
			log.Warningf("spn/navigator: could not parse hub %q while initializing %s map: %s", r.Key(), m.Name, err)
			continue
		}

		hubCount++
		m.updateHub(h, false, true)
	}
	switch {
	case iter.Err() != nil:
		log.Warningf("spn/navigator: failed to (fully) initialize %s map: %s", m.Name, iter.Err())
	case hubCount == 0:
		log.Warningf("spn/navigator: no hubs available for %s map - this is normal on first start", m.Name)
	default:
		log.Infof("spn/navigator: added %d hubs from database to %s map", hubCount, m.Name)
	}
}

// UpdateHook updates the a map from database changes.
type UpdateHook struct {
	database.HookBase
	m *Map
}

// UsesPrePut implements the Hook interface.
func (hook *UpdateHook) UsesPrePut() bool {
	return true
}

// PrePut implements the Hook interface.
func (hook *UpdateHook) PrePut(r record.Record) (record.Record, error) {
	// Remove deleted hubs from the map.
	if r.Meta().IsDeleted() {
		hook.m.RemoveHub(path.Base(r.Key()))
		return r, nil
	}

	// Ensure we have a hub and update it in navigation map.
	h, err := hub.EnsureHub(r)
	if err != nil {
		log.Debugf("spn/navigator: record %s is not a hub", r.Key())
	} else {
		hook.m.updateHub(h, true, false)
	}

	return r, nil
}

// RegisterHubUpdateHook registers a database pre-put hook that updates all
// Hubs saved at the given database prefix.
func (m *Map) RegisterHubUpdateHook() (err error) {
	m.hubUpdateHook, err = database.RegisterHook(
		query.New(hub.MakeHubDBKey(m.Name, "")),
		&UpdateHook{m: m},
	)
	return err
}

// CancelHubUpdateHook cancels the map's update hook.
func (m *Map) CancelHubUpdateHook() {
	if m.hubUpdateHook != nil {
		if err := m.hubUpdateHook.Cancel(); err != nil {
			log.Warningf("spn/navigator: failed to cancel update hook for map %s: %s", m.Name, err)
		}
	}
}

// RemoveHub removes a Hub from the Map.
func (m *Map) RemoveHub(id string) {
	m.Lock()
	defer m.Unlock()

	// Get pin and remove it from the map, if it exists.
	pin, ok := m.all[id]
	if !ok {
		return
	}
	delete(m.all, id)

	// Remove lanes from removed Pin.
	for id := range pin.ConnectedTo {
		// Remove Lane from peer.
		peer, ok := m.all[id]
		if ok {
			delete(peer.ConnectedTo, pin.Hub.ID)
			peer.pushChanges.Set()
		}
	}

	// Push update to subscriptions.
	export := pin.Export()
	export.Meta().Delete()
	mapDBController.PushUpdate(export)
	// Push lane changes.
	m.PushPinChanges()
}

// UpdateHub updates a Hub on the Map.
func (m *Map) UpdateHub(h *hub.Hub) {
	m.updateHub(h, true, true)
}

func (m *Map) updateHub(h *hub.Hub, lockMap, lockHub bool) {
	if lockMap {
		m.Lock()
		defer m.Unlock()
	}
	if lockHub {
		h.Lock()
		defer h.Unlock()
	}

	// Hub requires both Info and Status to be added to the Map.
	if h.Info == nil || h.Status == nil {
		return
	}

	// Create or update Pin.
	pin, ok := m.all[h.ID]
	if ok {
		pin.Hub = h
	} else {
		pin = &Pin{
			Hub:         h,
			ConnectedTo: make(map[string]*Lane),
			pushChanges: abool.New(),
		}
		m.all[h.ID] = pin
	}
	pin.pushChanges.Set()

	// 1. Update Pin Data.

	// Add/Update location data from IP addresses.
	pin.updateLocationData()

	// Override Pin Data.
	m.updateInfoOverrides(pin)

	// Update Hub cost.
	pin.Cost = CalculateHubCost(pin.Hub.Status.Load)

	// Ensure measurements are set when enabled.
	if m.measuringEnabled && pin.measurements == nil {
		// Get shared measurements.
		pin.measurements = pin.Hub.GetMeasurementsWithLockedHub()

		// Update cost calculation.
		latency, _ := pin.measurements.GetLatency()
		capacity, _ := pin.measurements.GetCapacity()
		pin.measurements.SetCalculatedCost(CalculateLaneCost(latency, capacity))

		// Update geo proximity.
		// Get own location.
		var myLocation *geoip.Location
		switch {
		case m.home != nil && m.home.LocationV4 != nil:
			myLocation = m.home.LocationV4
		case m.home != nil && m.home.LocationV6 != nil:
			myLocation = m.home.LocationV6
		default:
			locations, ok := netenv.GetInternetLocation()
			if ok {
				myLocation = locations.Best().LocationOrNil()
			}
		}
		// Calculate proximity with available location.
		if myLocation != nil {
			switch {
			case pin.LocationV4 != nil:
				pin.measurements.SetGeoProximity(
					myLocation.EstimateNetworkProximity(pin.LocationV4),
				)
			case pin.LocationV6 != nil:
				pin.measurements.SetGeoProximity(
					myLocation.EstimateNetworkProximity(pin.LocationV6),
				)
			}
		}
	}

	// 2. Update Pin States.

	// Update the invalid status of the Pin.
	if pin.Hub.InvalidInfo || pin.Hub.InvalidStatus {
		pin.addStates(StateInvalid)
	} else {
		pin.removeStates(StateInvalid)
	}

	// Update online status of the Pin.
	if pin.Hub.Status.Version == hub.VersionOffline {
		pin.addStates(StateOffline)
	} else {
		pin.removeStates(StateOffline)
	}

	// Update Trust and Advisory Statuses.
	m.updateIntelStatuses(pin)

	// Update Statuses derived from Hub.
	m.updateStateSuperseded(pin)
	pin.updateStateHasRequiredInfo()
	pin.updateStateActive(time.Now().Unix())

	// 3. Update Lanes.

	// Mark all existing Lanes as inactive.
	for _, lane := range pin.ConnectedTo {
		lane.active = false
	}

	// Update Lanes (connections to other Hubs) from the Status.
	for _, lane := range pin.Hub.Status.Lanes {
		// Check if this is a Lane to itself.
		if lane.ID == pin.Hub.ID {
			continue
		}

		// First, get the Lane peer.
		peer, ok := m.all[lane.ID]
		if !ok {
			// We need to wait for peer to be added to the Map.
			continue
		}

		m.updateHubLane(pin, lane, peer)
	}

	// Remove all inactive/abandoned Lanes from both Pins.
	var removedLanes bool
	for id, lane := range pin.ConnectedTo {
		if !lane.active {
			// Remove Lane from this Pin.
			delete(pin.ConnectedTo, id)
			pin.pushChanges.Set()
			removedLanes = true
			// Remove Lane from peer.
			peer, ok := m.all[id]
			if ok {
				delete(peer.ConnectedTo, pin.Hub.ID)
				peer.pushChanges.Set()
			}
		}
	}

	// Fully recalculate reachability if any Lanes were removed.
	if removedLanes {
		err := m.recalculateReachableHubs()
		if err != nil {
			log.Warningf("spn/navigator: failed to recalculate reachable Hubs: %s", err)
		}
	}

	// Push updates.
	m.PushPinChanges()
}

const (
	minUnconfirmedLatency  = 10 * time.Millisecond
	maxUnconfirmedCapacity = 100000000 // 100Mbit/s

	cap1Mbit   = 1000000
	cap10Mbit  = 10000000
	cap100Mbit = 100000000
	cap1Gbit   = 1000000000
	cap10Gbit  = 10000000000
)

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

	// Calculate combined latency, use the greater value.
	combinedLatency := lane.Latency
	if peerLane.Latency > combinedLatency {
		combinedLatency = peerLane.Latency
	}
	// Enforce minimum value if at least one side has no data.
	if (lane.Latency == 0 || peerLane.Latency == 0) && combinedLatency < minUnconfirmedLatency {
		combinedLatency = minUnconfirmedLatency
	}

	// Calculate combined capacity, use the lesser existing value.
	combinedCapacity := lane.Capacity
	if combinedCapacity == 0 || (peerLane.Capacity > 0 && peerLane.Capacity < combinedCapacity) {
		combinedCapacity = peerLane.Capacity
	}
	// Enforce maximum value if at least one side has no data.
	if (lane.Capacity == 0 || peerLane.Capacity == 0) && combinedCapacity > maxUnconfirmedCapacity {
		combinedCapacity = maxUnconfirmedCapacity
	}

	// Calculate lane cost.
	laneCost := CalculateLaneCost(combinedLatency, combinedCapacity)

	// Add Lane to both Pins and override old values in the process.
	pin.ConnectedTo[peer.Hub.ID] = &Lane{
		Pin:      peer,
		Capacity: combinedCapacity,
		Latency:  combinedLatency,
		Cost:     laneCost,
		active:   true,
	}
	peer.ConnectedTo[pin.Hub.ID] = &Lane{
		Pin:      pin,
		Capacity: combinedCapacity,
		Latency:  combinedLatency,
		Cost:     laneCost,
		active:   true,
	}
	peer.pushChanges.Set()

	// Check for reachability.

	if pin.State.has(StateReachable) {
		peer.markReachable(pin.HopDistance + 1)
	}
	if peer.State.has(StateReachable) {
		pin.markReachable(peer.HopDistance + 1)
	}
}

func (m *Map) updateStates(ctx context.Context, task *modules.Task) error {
	var toDelete []string
	now := time.Now()

	m.Lock()
	defer m.Unlock()

pinLoop:
	for _, pin := range m.all {
		// Update StateFailing.
		if pin.State.has(StateFailing) && now.After(pin.FailingUntil) {
			pin.removeStates(StateFailing)
		}

		// Check for discontinued Hubs.
		if m.intel != nil {
			for _, hubID := range m.intel.DiscontinuedHubs {
				if pin.Hub.ID == hubID {
					toDelete = append(toDelete, pin.Hub.ID)
					log.Infof("spn/navigator: deleting discontinued %s", pin.Hub)
					continue pinLoop
				}
			}
		}
		// Check for obsoleted Hubs.
		if pin.State.hasNoneOf(StateActive) && pin.Hub.Obsolete() {
			toDelete = append(toDelete, pin.Hub.ID)
			log.Infof("spn/navigator: deleting obsolete %s", pin.Hub)
		}

		// Delete hubs async, as deleting triggers a couple hooks that lock the map.
		if len(toDelete) > 0 {
			module.StartWorker("delete hubs", func(_ context.Context) error {
				for _, idToDelete := range toDelete {
					err := hub.RemoveHubAndMsgs(m.Name, idToDelete)
					if err != nil {
						log.Warningf("spn/navigator: failed to delete Hub %s: %s", idToDelete, err)
					}
				}
				return nil
			})
		}
	}

	// Update StateActive.
	m.updateActiveHubs()

	// Update StateReachable.
	return m.recalculateReachableHubs()
}

// AddBootstrapHubs adds the given bootstrap hubs to the map.
func (m *Map) AddBootstrapHubs(bootstrapTransports []string) error {
	m.Lock()
	defer m.Unlock()

	return m.addBootstrapHubs(bootstrapTransports)
}

func (m *Map) addBootstrapHubs(bootstrapTransports []string) error {
	var anyAdded bool
	var lastErr error
	var failed int
	for _, bootstrapTransport := range bootstrapTransports {
		err := m.addBootstrapHub(bootstrapTransport)
		if err != nil {
			log.Warningf("spn/navigator: failed to add bootstrap hub %q to map %s: %s", bootstrapTransport, m.Name, err)
			lastErr = err
			failed++
		} else {
			anyAdded = true
		}
	}

	if lastErr != nil && !anyAdded {
		return lastErr
	}
	return nil
}

func (m *Map) addBootstrapHub(bootstrapTransport string) error {
	// Parse bootstrap hub.
	bootstrapHub, err := hub.ParseBootstrapHub(bootstrapTransport, m.Name)
	if err != nil {
		return fmt.Errorf("invalid bootstrap hub: %w", err)
	}

	// Check if hub already exists.
	_, err = hub.GetHub(bootstrapHub.Map, bootstrapHub.ID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, database.ErrNotFound) {
		return err
	}

	// Add to map for bootstrapping.
	m.updateHub(bootstrapHub, false, false)
	log.Infof("spn/navigator: added bootstrap %s to map %s", bootstrapHub, m.Name)
	return nil
}
