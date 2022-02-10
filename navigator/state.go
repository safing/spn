package navigator

import (
	"strings"
	"time"
)

// PinState holds a bit-mapped collection of Pin states, or a single state used
// for assigment and matching.
type PinState uint16

const (
	// StateNone represents an empty state.
	StateNone PinState = 0

	// Negative States.

	// StateInvalid signifies that there was an error while processing or
	// handling this Hub.
	StateInvalid PinState = 1 << (iota - 1) // 1 << 0 => 00000001 => 0x01

	// StateSuperseded signifies that this Hub was superseded by another. This is
	// the case if any other Hub with a matching IP was verified after this one.
	// Verification timestamp equals Hub.FirstSeen.
	StateSuperseded // 0x02

	// StateFailing signifies that a recent error was encountered while
	// communicating with this Hub. Pin.IgnoreUntil specifies when this state is
	// re-evaluated at earliest.
	StateFailing // 0x04

	// StateOffline signifies that the Hub is offline.
	StateOffline // 0x08

	// Positive States.

	// StateHasRequiredInfo signifies that the Hub announces the minimum required
	// information about itself.
	StateHasRequiredInfo // 0x10

	// StateReachable signifies that the Hub is reachable via the network from
	// the currently connected primary Hub.
	StateReachable // 0x20

	// StateActive signifies that everything seems fine with the Hub and
	// connections to it should succeed. This is tested by checking if a valid
	// semi-ephemeral public key is available.
	StateActive // 0x40

	_ // 0x80: Reserved

	// Trust and Advisory States.

	// StateTrusted signifies the Hub has the special trusted status.
	StateTrusted // 0x0100

	// StateUsageDiscouraged signifies that usage of the Hub is discouraged for any task.
	StateUsageDiscouraged // 0x0200

	// StateUsageAsHomeDiscouraged signifies that usage of the Hub as a Home Hub is discouraged.
	StateUsageAsHomeDiscouraged // 0x0400

	// StateUsageAsDestinationDiscouraged signifies that usage of the Hub as a Destination Hub is discouraged.
	StateUsageAsDestinationDiscouraged // 0x0800

	// Special States.

	// StateIsHomeHub signifies that the Hub is the current Home Hub. While not
	// negative in itself, selecting the Home Hub does not make sense in almost
	// all cases.
	StateIsHomeHub // 0x1000

	// State Summaries.

	// StateSummaryRegard summarizes all states that must always be set in order to take a Hub into consideration for any task.
	// TODO: Add StateHasRequiredInfo when we start enforcing Hub information.
	StateSummaryRegard = StateReachable | StateActive

	// StateSummaryDisregard summarizes all states that must not be set in order to take a Hub into consideration for any task.
	StateSummaryDisregard = StateInvalid |
		StateSuperseded |
		StateFailing |
		StateOffline |
		StateUsageDiscouraged |
		StateIsHomeHub
)

var allStates = []PinState{
	StateInvalid,
	StateSuperseded,
	StateFailing,
	StateOffline,
	StateHasRequiredInfo,
	StateReachable,
	StateActive,
	StateTrusted,
	StateUsageDiscouraged,
	StateUsageAsHomeDiscouraged,
	StateUsageAsDestinationDiscouraged,
	StateIsHomeHub,
}

// add returns a new PinState with the given states added.
func (pinState PinState) add(states PinState) PinState {
	// OR:
	//   0011
	// | 0101
	// = 0111
	return pinState | states
}

// remove returns a new PinState with the given states removed.
func (pinState PinState) remove(states PinState) PinState {
	// AND NOT:
	//    0011
	// &^ 0101
	// =  0010
	return pinState &^ states
}

// has returns whether the state has all of the given states.
func (pinState PinState) has(states PinState) bool {
	// AND:
	//   0011
	// & 0101
	// = 0001

	return pinState&states == states
}

// hasAnyOf returns whether the state has any of the given states.
func (pinState PinState) hasAnyOf(states PinState) bool {
	// AND:
	//   0011
	// & 0101
	// = 0001

	return (pinState & states) != 0
}

// hasNoneOf returns whether the state does not have any of the given states.
func (pinState PinState) hasNoneOf(states PinState) bool {
	// AND:
	//   0011
	// & 0101
	// = 0001

	return (pinState & states) == 0
}

// addStates adds the given states on the Pin.
func (pin *Pin) addStates(states PinState) {
	pin.State = pin.State.add(states)
}

// removeStates removes the given states on the Pin.
func (pin *Pin) removeStates(states PinState) {
	pin.State = pin.State.remove(states)
}

func (m *Map) updateStateSuperseded(pin *Pin) {
	pin.removeStates(StateSuperseded)

	// Update StateSuperseded
	// Iterate over all Pins in order to find a matching IP address.
	// In order to prevent false positive matching, we have to go through IPv4
	// and IPv6 separately.
	// TODO: This will not scale well beyond about 1000 Hubs.

	// IPv4 Loop
	if pin.Hub.Info.IPv4 != nil {
		for _, mapPin := range m.all {
			// Skip Pin itself
			if mapPin.Hub.ID == pin.Hub.ID {
				continue
			}

			// Check for a matching IPv4 address.
			if mapPin.Hub.Info.IPv4 != nil && pin.Hub.Info.IPv4.Equal(mapPin.Hub.Info.IPv4) {
				// If there is a match assign the older Hub the Superseded status.
				if pin.Hub.FirstSeen.After(mapPin.Hub.FirstSeen) {
					mapPin.addStates(StateSuperseded)
					mapPin.pushChanges.Set()
					// This Pin is newer and superseeds another, keep looking for more.
				} else {
					pin.addStates(StateSuperseded)
					// This Pin is older than an existing one, don't keep looking, as this
					// results in incorrect data.
					break
				}
			}
		}
	}

	// IPv6 Loop
	if pin.Hub.Info.IPv6 != nil {
		for _, mapPin := range m.all {
			// Skip Pin itself
			if mapPin.Hub.ID == pin.Hub.ID {
				continue
			}

			// Check for a matching IPv6 address.
			if mapPin.Hub.Info.IPv6 != nil && pin.Hub.Info.IPv6.Equal(mapPin.Hub.Info.IPv6) {
				// If there is a match assign the older Hub the Superseded status.
				if pin.Hub.FirstSeen.After(mapPin.Hub.FirstSeen) {
					mapPin.addStates(StateSuperseded)
					mapPin.pushChanges.Set()
					// This Pin is newer and superseeds another, keep looking for more.
				} else {
					pin.addStates(StateSuperseded)
					// This Pin is older than an existing one, don't keep looking, as this
					// results in incorrect data.
					break
				}
			}
		}
	}
}

func (pin *Pin) updateStateHasRequiredInfo() {
	pin.removeStates(StateHasRequiredInfo)

	// Check for required Hub Information.
	switch {
	case len(pin.Hub.Info.Name) == 0:
	case len(pin.Hub.Info.Group) == 0:
	case len(pin.Hub.Info.ContactAddress) == 0:
	case len(pin.Hub.Info.ContactService) == 0:
	case len(pin.Hub.Info.Hosters) == 0:
	case len(pin.Hub.Info.Hosters[0]) == 0:
	case len(pin.Hub.Info.Datacenter) == 0:
	default:
		pin.addStates(StateHasRequiredInfo)
	}
}

func (m *Map) updateActiveHubs() {
	now := time.Now().Unix()
	for _, pin := range m.all {
		pin.updateStateActive(now)
	}
}

func (pin *Pin) updateStateActive(now int64) {
	pin.removeStates(StateActive)

	// Check for active key.
	for _, key := range pin.Hub.Status.Keys {
		if now < key.Expires {
			pin.addStates(StateActive)
			return
		}
	}
}

func (m *Map) recalculateReachableHubs() error {
	if m.home == nil {
		return ErrHomeHubUnset
	}

	// reset
	for _, pin := range m.all {
		pin.removeStates(StateReachable)
		pin.HopDistance = 0
		pin.pushChanges.Set()
	}

	// find all connected Hubs
	m.home.markReachable(1)
	return nil
}

func (pin *Pin) markReachable(hopDistance int) {
	switch {
	case !pin.State.has(StateReachable):
		// Pin wasn't reachable before.
	case hopDistance < pin.HopDistance:
		// New path has a shorter distance.
	case pin.State.hasAnyOf(StateSummaryDisregard): //nolint:staticcheck
		// Ignore disregarded pins for reachability calculation.
		return
	default:
		// Pin is already reachable at same or better distance.
		return
	}

	// Update reachability.
	pin.addStates(StateReachable)
	pin.HopDistance = hopDistance
	pin.pushChanges.Set()

	// Propagate to connected Pins.
	hopDistance++
	for _, lane := range pin.ConnectedTo {
		lane.Pin.markReachable(hopDistance)
	}
}

// Export returns a list of all state names.
func (pinState PinState) Export() []string {
	// Check if there are no states.
	if pinState == StateNone {
		return nil
	}

	// Collect state names.
	var stateNames []string
	for _, state := range allStates {
		if pinState.has(state) {
			stateNames = append(stateNames, state.Name())
		}
	}

	return stateNames
}

// String returns the states as a human readable string.
func (pinState PinState) String() string {
	stateNames := pinState.Export()
	if len(stateNames) == 0 {
		return "None"
	}

	return strings.Join(stateNames, ", ")
}

// Name returns the name of a single state flag.
func (pinState PinState) Name() string {
	switch pinState {
	case StateNone:
		return "None"
	case StateInvalid:
		return "Invalid"
	case StateSuperseded:
		return "Superseded"
	case StateFailing:
		return "Failing"
	case StateOffline:
		return "Offline"
	case StateHasRequiredInfo:
		return "HasRequiredInfo"
	case StateReachable:
		return "Reachable"
	case StateActive:
		return "Active"
	case StateTrusted:
		return "Trusted"
	case StateUsageDiscouraged:
		return "UsageDiscouraged"
	case StateUsageAsHomeDiscouraged:
		return "UsageAsHomeDiscouraged"
	case StateUsageAsDestinationDiscouraged:
		return "UsageAsDestinationDiscouraged"
	case StateIsHomeHub:
		return "IsHomeHub"
	case StateSummaryRegard, StateSummaryDisregard:
		// Satisfy exhaustive linter.
		fallthrough
	default:
		return "Unknown"
	}
}
