package docks

import (
	"sync"
	"sync/atomic"
	"time"
)

const NetStatePeriodInterval = 15 * time.Minute

type NetworkOptimizationState struct {
	lock sync.Mutex

	// lastSuggestedAt holds the time when the connection to the connected Hub was last suggested by the network optimization.
	lastSuggestedAt time.Time

	// markedStoppingAt holds the time when the crane was last marked as stopping.
	markedStoppingAt time.Time

	lifetimeBytesIn  *uint64
	lifetimeBytesOut *uint64
	lifetimeStarted  time.Time
	periodBytesIn    *uint64
	periodBytesOut   *uint64
	periodStarted    time.Time
}

func newNetworkOptimizationState() *NetworkOptimizationState {
	return &NetworkOptimizationState{
		lifetimeBytesIn:  new(uint64),
		lifetimeBytesOut: new(uint64),
		lifetimeStarted:  time.Now(),
		periodBytesIn:    new(uint64),
		periodBytesOut:   new(uint64),
		periodStarted:    time.Now(),
	}
}

func (netState *NetworkOptimizationState) UpdateLastSuggestedAt() {
	netState.lock.Lock()
	defer netState.lock.Unlock()

	netState.lastSuggestedAt = time.Now()
}

func (netState *NetworkOptimizationState) LastSuggestedAt() time.Time {
	netState.lock.Lock()
	defer netState.lock.Unlock()

	return netState.lastSuggestedAt
}

func (netState *NetworkOptimizationState) UpdateMarkedStoppingAt() {
	netState.lock.Lock()
	defer netState.lock.Unlock()

	netState.markedStoppingAt = time.Now()
}

func (netState *NetworkOptimizationState) MarkedStoppingAt() time.Time {
	netState.lock.Lock()
	defer netState.lock.Unlock()

	return netState.markedStoppingAt
}

func (netState *NetworkOptimizationState) ReportTraffic(bytes uint64, in bool) {
	if in {
		atomic.AddUint64(netState.lifetimeBytesIn, bytes)
		atomic.AddUint64(netState.periodBytesIn, bytes)
	} else {
		atomic.AddUint64(netState.lifetimeBytesOut, bytes)
		atomic.AddUint64(netState.periodBytesOut, bytes)
	}
}

func (netState *NetworkOptimizationState) LapsePeriod() {
	netState.lock.Lock()
	defer netState.lock.Unlock()

	// Reset period if interval elapsed.
	if time.Now().Add(-NetStatePeriodInterval).After(netState.periodStarted) {
		atomic.StoreUint64(netState.periodBytesIn, 0)
		atomic.StoreUint64(netState.periodBytesOut, 0)
		netState.periodStarted = time.Now()
	}
}

func (netState *NetworkOptimizationState) GetTrafficStats() (
	lifetimeBytesIn uint64,
	lifetimeBytesOut uint64,
	lifetimeStarted time.Time,
	periodBytesIn uint64,
	periodBytesOut uint64,
	periodStarted time.Time,
) {
	netState.lock.Lock()
	defer netState.lock.Unlock()

	return atomic.LoadUint64(netState.lifetimeBytesIn),
		atomic.LoadUint64(netState.lifetimeBytesOut),
		netState.lifetimeStarted,
		atomic.LoadUint64(netState.periodBytesIn),
		atomic.LoadUint64(netState.periodBytesOut),
		netState.periodStarted
}
