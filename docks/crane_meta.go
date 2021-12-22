package docks

import (
	"time"

	"github.com/safing/portbase/log"
)

const (
	LaneLatencyTTL  = 1 * time.Hour
	LaneCapacityTTL = 1 * time.Hour
)

// SetLaneLatency sets the lane latency to the given value.
func (crane *Crane) SetLaneLatency(latency time.Duration) {
	crane.metaLock.Lock()
	defer crane.metaLock.Unlock()

	crane.laneLatency = latency
	crane.laneLatencyExpires = time.Now().Add(LaneLatencyTTL)

	log.Infof("docks: %s lane latency set to %s", crane, latency)
}

// GetLaneLatency returns the lane latency.
func (crane *Crane) GetLaneLatency() time.Duration {
	crane.metaLock.Lock()
	defer crane.metaLock.Unlock()

	return crane.laneLatency
}

// LaneLatencyExpiresAt returns when the lane latency expires and should be
// updated.
func (crane *Crane) LaneLatencyExpiresAt() time.Time {
	crane.metaLock.Lock()
	defer crane.metaLock.Unlock()

	return crane.laneLatencyExpires
}

// SetLaneCapacity sets the lane capacity to the given value.
// The capacity is measued in bit/s.
func (crane *Crane) SetLaneCapacity(capacity int) {
	crane.metaLock.Lock()
	defer crane.metaLock.Unlock()

	crane.laneCapacity = capacity
	crane.laneCapacityExpires = time.Now().Add(LaneCapacityTTL)

	log.Infof("docks: %s lane capacity set to %.2f mbit/s", crane, float64(capacity)/1000000)
}

// GetLaneCapacity returns the lane capacity.
// The capacity is measued in bit/s.
func (crane *Crane) GetLaneCapacity() int {
	crane.metaLock.Lock()
	defer crane.metaLock.Unlock()

	return crane.laneCapacity
}

// LaneCapacityExpiresAt returns when the lane capacity expires and should be
// updated.
func (crane *Crane) LaneCapacityExpiresAt() time.Time {
	crane.metaLock.Lock()
	defer crane.metaLock.Unlock()

	return crane.laneCapacityExpires
}
