package docks

import (
	"sync"
	"time"
)

type NetworkOptimizationState struct {
	sync.Mutex

	// lastSuggestedAt holds the time when the connnection to the connected Hub was last suggested by the network optimization.
	lastSuggestedAt time.Time
}

func newNetworkOptimizationState() *NetworkOptimizationState {
	return &NetworkOptimizationState{}
}

func (netState *NetworkOptimizationState) UpdateLastSuggestedAt() {
	netState.Lock()
	defer netState.Unlock()

	netState.lastSuggestedAt = time.Now()
}

func (netState *NetworkOptimizationState) LastSuggestedAt() time.Time {
	netState.Lock()
	defer netState.Unlock()

	return netState.lastSuggestedAt
}
