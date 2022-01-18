package navigator

import (
	"context"
	"time"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/intel"
	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
	"github.com/tevino/abool"
)

// Pin represents a Hub on a Map.
type Pin struct {
	// Hub Information
	Hub        *hub.Hub
	EntityV4   *intel.Entity
	EntityV6   *intel.Entity
	LocationV4 *geoip.Location
	LocationV6 *geoip.Location

	// Hub Status
	State PinState
	// HopDistance signifies the needed hops to reach this Hub.
	// HopDistance is measured from the view of a client.
	// A Hub itself will have itself at distance 1.
	HopDistance int
	// Cost is the routing cost of this Hub.
	Cost float32
	// ConnectedTo holds validated lanes.
	ConnectedTo map[string]*Lane // Key is Hub ID.

	// FailingUntil specifies until when this Hub should be regarded as failing.
	// This is connected to StateFailing.
	FailingUntil time.Time

	// Connection holds a information about a connection to the Hub of this Pin.
	Connection *PinConnection

	// Internal

	// pushChanges is set to true if something noteworthy on the Pin changed and
	// an update needs to be pushed by the database storage interface to whoever
	// is listening.
	pushChanges *abool.AtomicBool

	// measurements holds Measurements regarding this Pin.
	// It must always be set and the reference must not be changed when measuring
	// is enabled.
	// Access to fields within are coordinated by itself.
	measurements *hub.Measurements
}

// PinConnection represents a connection to a terminal on the Hub.
type PinConnection struct {
	// Terminal holds the active terminal session.
	Terminal *docks.ExpansionTerminal

	// Route is the route built for this terminal.
	Route *Route
}

// Lane is a connection to another Hub.
type Lane struct {
	// Pin is the Pin/Hub this Lane connects to.
	Pin *Pin

	// Capacity designates the available bandwidth between these Hubs.
	// It is specified in bit/s.
	Capacity int

	// Lateny designates the latency between these Hubs.
	// It is specified in nanoseconds.
	Latency time.Duration

	// Cost is the routing cost of this lane.
	Cost float32

	// active is a helper flag in order help remove abandoned Lanes.
	active bool
}

func (pin *Pin) Lock() {
	pin.Hub.Lock()
}

func (pin *Pin) Unlock() {
	pin.Hub.Unlock()
}

// String returns a human-readable representation of the Pin.
func (pin *Pin) String() string {
	return "<Pin " + pin.Hub.Name() + ">"
}

// updateLocationData fetches the necessary location data in order to correctly map out the Pin.
func (pin *Pin) updateLocationData() {
	if pin.Hub.Info.IPv4 != nil {
		pin.EntityV4 = &intel.Entity{}
		pin.EntityV4.SetIP(pin.Hub.Info.IPv4)

		var ok bool
		pin.LocationV4, ok = pin.EntityV4.GetLocation(context.TODO())
		if !ok {
			log.Warningf("navigator: failed to get location of %s of %s", pin.Hub.Info.IPv4, pin.Hub.StringWithoutLocking())
			return
		}
	} else {
		pin.EntityV4 = nil
		pin.LocationV4 = nil
	}

	if pin.Hub.Info.IPv6 != nil {
		pin.EntityV6 = &intel.Entity{}
		pin.EntityV6.SetIP(pin.Hub.Info.IPv6)

		var ok bool
		pin.LocationV6, ok = pin.EntityV6.GetLocation(context.TODO())
		if !ok {
			log.Warningf("navigator: failed to get location of %s of %s", pin.Hub.Info.IPv6, pin.Hub.StringWithoutLocking())
			return
		}
	} else {
		pin.EntityV6 = nil
		pin.LocationV6 = nil
	}
}

func (pin *Pin) SetActiveTerminal(pc *PinConnection) {
	pin.Lock()
	defer pin.Unlock()

	pin.Connection = pc
	if pin.Connection.Terminal != nil {
		pin.Connection.Terminal.SetChangeNotifyFunc(pin.NotifyTerminalChange)
	}

	pin.pushChanges.Set()
}

func (pin *Pin) GetActiveTerminal() *docks.ExpansionTerminal {
	pin.Lock()
	defer pin.Unlock()

	if !pin.hasActiveTerminal() {
		return nil
	}
	return pin.Connection.Terminal
}

func (pin *Pin) HasActiveTerminal() bool {
	pin.Lock()
	defer pin.Unlock()

	return pin.hasActiveTerminal()
}

func (pin *Pin) hasActiveTerminal() bool {
	return pin.Connection != nil &&
		!pin.Connection.Terminal.IsAbandoned()
}

func (pin *Pin) NotifyTerminalChange() {
	if !pin.HasActiveTerminal() {
		pin.pushChanges.Set()
	}

	pin.pushChange()
}
