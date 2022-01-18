package navigator

import (
	"sort"
	"time"

	"github.com/safing/spn/hub"
)

const (
	optimizationHopDistanceTarget = 3
	waitUntilMeasuredUpToPercent  = 0 // FIXME: Change back to 0.8 after migration.

	desegrationAttemptBackoff = time.Hour

	OptimizePurposeBootstrap       = "bootstrap"
	OptimizePurposeDesegregate     = "desegregate"
	OptimizePurposeWait            = "wait"
	OptimizePurposeTargetStructure = "target-structure"
)

type AnalysisState struct {
	// Suggested signifies that a direct connection to this Hub is suggested by the optimization algorithm.
	Suggested bool

	// SuggestedHopDistance holds the hop distance to this Hub when only considering the suggested Hubs as connected.
	SuggestedHopDistance int
}

// initAnalysis creates all Pin.analysis fields.
// The caller needs to hold the map and analysis lock..
func (m *Map) initAnalysis() {
	for _, pin := range m.all {
		pin.analysis = &AnalysisState{}
	}
}

// clearAnalysis reset all Pin.analysis fields.
// The caller needs to hold the map and analysis lock.
func (m *Map) clearAnalysis() {
	for _, pin := range m.all {
		pin.analysis = nil
	}
}

// OptimizationResult holds the result of an optimizaion analysis.
type OptimizationResult struct {
	// Purpose holds a semi-human readable constant of the optimization purpose.
	Purpose string

	// Approach holds a human readable description of how the stated purpose
	// should be achieved.
	Approach string

	// SuggestedConnections holds the Hubs to which connections are suggested.
	SuggestedConnections []*hub.Hub

	// MaxConnect specifies how many connections should be created at maximum
	// based on this optimization.
	MaxConnect int

	// StopOthers specifies if other connections than the suggested ones may
	// be stopped.
	StopOthers bool

	// regardedPins holds a list of Pins regarded for this optimization.
	regardedPins []*Pin

	// matcher is the matcher used to create the regarded Pins.
	// Required for updating suggested hop distance.
	matcher PinMatcher
}

func (or *OptimizationResult) addSuggested(pin *Pin) {
	or.SuggestedConnections = append(or.SuggestedConnections, pin.Hub)

	// Update hop distances if we have a matcher.
	if or.matcher != nil {
		or.markSuggestedReachable(pin, 2)
	}
}

func (or *OptimizationResult) addSuggestedBatch(pins []*Pin) {
	for _, pin := range pins {
		or.addSuggested(pin)
	}
}

func (or *OptimizationResult) markSuggestedReachable(suggested *Pin, hopDistance int) {
	// Don't update if distance is greater or equal than than current one.
	if hopDistance >= suggested.analysis.SuggestedHopDistance {
		return
	}

	// Set suggested hop distance.
	suggested.analysis.SuggestedHopDistance = hopDistance

	// Increase distance and apply to matching Pins.
	hopDistance++
	for _, lane := range suggested.ConnectedTo {
		if or.matcher(lane.Pin) {
			or.markSuggestedReachable(lane.Pin, hopDistance)
		}
	}
}

// Optimize analyzes the map and suggests changes.
func (m *Map) Optimize(opts *Options) (result *OptimizationResult, err error) {
	m.RLock()
	defer m.RUnlock()

	// Check if the map is empty.
	if m.isEmpty() {
		return nil, ErrEmptyMap
	}

	// Set default options if unset.
	if opts == nil {
		opts = m.defaultOptions()
	}

	return m.optimize(opts)
}

func (m *Map) optimize(opts *Options) (result *OptimizationResult, err error) {
	if m.home == nil {
		return nil, ErrHomeHubUnset
	}

	// Setup analyis.
	m.analysisLock.Lock()
	defer m.analysisLock.Unlock()
	m.initAnalysis()
	defer m.clearAnalysis()

	// Compile list of regarded pins.
	var validMeasurements float32
	regardedPins := make([]*Pin, 0, len(m.all))
	matcher := opts.Matcher(TransitHub)
	for _, pin := range m.all {
		if matcher(pin) {
			regardedPins = append(regardedPins, pin)
			if pin.measurements.Valid() {
				validMeasurements++
			}
		}
	}

	// Bootstrap to the network and desegregate map.
	// If there is a result, return it immediately.
	result, err = m.optimizeBootstrapAndDesegregate(opts, regardedPins)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}

	// Check if we have the measurements we need.
	if m.measuringEnabled &&
		validMeasurements/float32(len(regardedPins)) < waitUntilMeasuredUpToPercent {
		// Less than 80% of regarded Pins have valid measurements, let's wait until we have that.
		return &OptimizationResult{
			Purpose:  OptimizePurposeWait,
			Approach: "Wait for measurements of 80% of regarded nodes for better optimization.",
		}, nil
	}

	// Create a shared result to add everything together from now on.
	result = &OptimizationResult{
		Purpose:      OptimizePurposeTargetStructure,
		Approach:     "Connect to 3 Hubs with lowest connect cost, then up to 3 Hubs to get everywhere with 3 client hops.",
		MaxConnect:   3,
		StopOthers:   true,
		regardedPins: regardedPins,
		matcher:      matcher,
	}

	// Connect by lowest cost.
	err = m.optimizeConnectLowestCost(result, 3)
	if err != nil {
		return nil, err
	}

	// Connect to fulfill distance constraint.
	err = m.optimizeDistanceConstraint(result, 3)
	if err != nil {
		return nil, err
	}

	// Clean and return.
	result.regardedPins = nil
	return result, nil
}

func (m *Map) optimizeBootstrapAndDesegregate(opts *Options, regardedPins []*Pin) (result *OptimizationResult, err error) {
	// All regarded Pins are reachable.
	reachable := len(regardedPins)

	// Count Pins that may be connectable.
	connectable := make([]*Pin, 0, len(m.all))
	// Copy opts as we are going to make changes.
	opts = opts.Copy()
	opts.NoDefaults = true
	opts.Regard = StateNone
	opts.Disregard = StateSummaryDisregard
	// Collect Pins with matcher.
	matcher := opts.Matcher(TransitHub)
	for _, pin := range m.all {
		if matcher(pin) {
			connectable = append(connectable, pin)
		}
	}

	switch {
	case reachable == 0:
		// Sort by lowest cost.
		sort.Sort(sortByLowestMeasuredCost(connectable))

		// Return bootstrap optimization.
		result = &OptimizationResult{
			Purpose:              OptimizePurposeBootstrap,
			Approach:             "Connect to a near Hub to connect to the network.",
			SuggestedConnections: make([]*hub.Hub, 0, len(connectable)),
			MaxConnect:           1,
		}
		result.addSuggestedBatch(connectable)
		return result, nil

	case reachable > len(connectable)/2:
		// We are part of the majority network, continue with regular optimization.

	case time.Now().Add(-desegrationAttemptBackoff).Before(m.lastDesegrationAttempt):
		// We tried to desegregate recently, continue with regular optimization.

	default:
		// We are in a network comprised of less than half of the known nodes.
		// Attempt to connect to an unconnected one to desegregate the network.

		// Copy opts as we are going to make changes.
		opts = opts.Copy()
		opts.NoDefaults = true
		opts.Regard = StateNone
		opts.Disregard = StateSummaryDisregard | StateReachable

		// Iterate over all Pins to find any matching Pin.
		desegregateWith := make([]*Pin, 0, len(m.all)-reachable)
		matcher := opts.Matcher(TransitHub)
		for _, pin := range m.all {
			if matcher(pin) {
				desegregateWith = append(desegregateWith, pin)
			}
		}

		// Sort by lowest connection cost.
		sort.Sort(sortByLowestMeasuredCost(connectable))

		// Build desegration optimization.
		result = &OptimizationResult{
			Purpose:              OptimizePurposeDesegregate,
			Approach:             "Attempt to desegregate network by connection to an unreachable Hub.",
			SuggestedConnections: make([]*hub.Hub, 0, len(desegregateWith)),
			MaxConnect:           1,
		}
		result.addSuggestedBatch(desegregateWith)

		// Record desegregation attempt.
		m.lastDesegrationAttempt = time.Now()

		return result, nil
	}

	return nil, nil
}

func (m *Map) optimizeConnectLowestCost(result *OptimizationResult, max int) error {
	// Sort by lowest cost.
	sort.Sort(sortByLowestMeasuredCost(result.regardedPins))

	// Connect to Pins with the lowest connection cost.
	for i, pin := range result.regardedPins {
		// Stop after looking at the first [max] Hubs.
		if i >= max {
			break
		}

		result.addSuggested(pin)

		// Mark as suggested for analysis.
		pin.analysis.Suggested = true
	}

	return nil
}

func (m *Map) optimizeDistanceConstraint(result *OptimizationResult, max int) error {
	for i := 0; i < max; i++ {
		// Sort by lowest cost.
		sort.Sort(sortBySuggestedHopDistanceAndLowestMeasuredCost(result.regardedPins))

		// Return when all regarded Pins are within the distance constraint.
		if result.regardedPins[0].analysis.SuggestedHopDistance <= optimizationHopDistanceTarget {
			return nil
		}

		// If not, suggest a connection to the best match.
		result.addSuggested(result.regardedPins[0])
	}

	return nil
}
