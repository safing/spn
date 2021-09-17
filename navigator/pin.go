package navigator

import (
	"context"
	"time"

	"github.com/safing/portmaster/intel"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/intel/geoip"
	"github.com/safing/spn/docks"
	"github.com/safing/spn/hub"
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
	State       PinState
	HopDistance int
	Load        int // estimated in microseconds this port adds to latency
	ConnectedTo map[string]*Lane

	// FailingUntil specifies until when this Hub should be regarded as failing.
	// This is connected to StateFailing.
	FailingUntil time.Time

	// Connection holds a information about a connection to the Hub of this Pin.
	Connection *PinConnection

	// Internal

	// pushChanges is set to true if something noteworthy on the Pin changed and
	// an update needs to be pushed by the database storage interface to whoever
	// is listening.
	pushChanges bool
}

// Session represents a terminal
type PinConnection struct {
	// Terminal holds the active terminal session.
	Terminal *docks.ExpansionTerminal

	// Route is the route built for this terminal.
	Route *Route

	// TODO: Next is the next alternative Router to the same Pin.
	// Next *PinRoute
}

// Lane is a connection to another Hub.
type Lane struct {
	// Pin is the Pin/Hub this Lane connects to.
	Pin *Pin

	// Capacity designates the available bandwidth between these Hubs.
	// It is specified in MBit/s.
	Capacity int

	// Lateny designates the latency between these Hubs.
	// It is specified in Milliseconds.
	Latency int

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
		pin.EntityV4 = &intel.Entity{
			IP: pin.Hub.Info.IPv4,
		}
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
		pin.EntityV6 = &intel.Entity{
			IP: pin.Hub.Info.IPv6,
		}
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

func (pin *Pin) HasActiveTerminal() bool {
	pin.Lock()
	defer pin.Unlock()

	return pin.hasActiveTerminal()
}

func (pin *Pin) hasActiveTerminal() bool {
	return pin.Connection != nil &&
		!pin.Connection.Terminal.IsAbandoned()
}
