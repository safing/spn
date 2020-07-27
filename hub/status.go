package hub

import (
	"errors"

	"github.com/safing/jess"
)

// HubStatus is the message type used to update changing Hub Information. Changes are made automatically.
type HubStatus struct {
	Timestamp int64

	// Routing Information
	Keys        map[string]*HubKey // public keys (with type)
	Connections []*HubConnection

	// Load describes max(CPU, Memory) in percent, averages over the last hour
	// only update if change is significant in terms of impact on routing
	// do not update more often than once an hour
	Load int
}

// HubKey represents a semi-ephemeral public key used for 0-RTT connection establishment.
type HubKey struct {
	Scheme  string
	Key     []byte
	Expires int64
}

// HubConnection represents a link to another Hub.
type HubConnection struct {
	ID       string // ID of peer
	Capacity int    // max available bandwidth in Mbit/s (measure actively!)
	Latency  int    // ping in msecs
}

// GetSignet returns the public key identified by the given ID from the Hub Status.
func (status *HubStatus) GetSignet(id string, recipient bool) (*jess.Signet, error) {
	// check if public key is being requested
	if !recipient {
		return nil, jess.ErrSignetNotFound
	}
	// check if ID exists
	key, ok := status.Keys[id]
	if !ok {
		return nil, jess.ErrSignetNotFound
	}
	// transform and return
	return &jess.Signet{
		ID:     id,
		Scheme: key.Scheme,
		Key:    key.Key,
		Public: true,
	}, nil
}

// AddConnection adds a new Hub Connection to the Hub Status.
func (h *Hub) AddConnection(newConn *HubConnection) error {
	h.Lock()
	defer h.Unlock()

	// validity check
	if h.Status == nil {
		return ErrMissingInfo
	}

	// check if duplicate
	for _, connection := range h.Status.Connections {
		if newConn.ID == connection.ID {
			return errors.New("connection to this Hub already added")
		}
	}

	// add
	h.Status.Connections = append(h.Status.Connections, newConn)
	return nil
}

// RemoveConnection removes a Hub Connection from the Hub Status.
func (h *Hub) RemoveConnection(hubID string) error {
	h.Lock()
	defer h.Unlock()

	// validity check
	if h.Status == nil {
		return ErrMissingInfo
	}

	for key, connection := range h.Status.Connections {
		if connection.ID == hubID {
			h.Status.Connections = append(h.Status.Connections[:key], h.Status.Connections[key+1:]...)
			break
		}
	}

	return nil
}

// Equal returns whether the HubConnection is equal to the given one.
func (c *HubConnection) Equal(other *HubConnection) bool {
	switch {
	case c.ID != other.ID:
		return false
	case c.Capacity != other.Capacity:
		return false
	case c.Latency != other.Latency:
		return false
	}
	return true
}

// ConnectionsEqual returns whether the given []*HubConnection are equal.
func ConnectionsEqual(a, b []*HubConnection) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !c.Equal(b[i]) {
			return false
		}
	}

	return true
}
