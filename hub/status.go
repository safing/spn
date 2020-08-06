package hub

import (
	"errors"
	"fmt"
	"sort"
	"time"

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

// SelectSignet selects the public key to use for initiating connections to that Hub.
func (h *Hub) SelectSignet() *jess.Signet {
	h.Lock()
	defer h.Unlock()

	// TODO: select key based preferred alg?
	for id, key := range h.Status.Keys {
		if time.Now().Unix() < key.Expires {
			return &jess.Signet{
				ID:     id,
				Scheme: key.Scheme,
				Key:    key.Key,
				Public: true,
			}
		}
	}

	return nil
}

// GetSignet returns the public key identified by the given ID from the Hub Status.
func (h *Hub) GetSignet(id string, recipient bool) (*jess.Signet, error) {
	h.Lock()
	defer h.Unlock()

	// check if public key is being requested
	if !recipient {
		return nil, jess.ErrSignetNotFound
	}
	// check if ID exists
	key, ok := h.Status.Keys[id]
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

func (c *HubConnection) String() string {
	return fmt.Sprintf("<%s cap=%d lat=%d>", c.ID, c.Capacity, c.Latency)
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

type connections []*HubConnection

func (c connections) Len() int           { return len(c) }
func (c connections) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c connections) Less(i, j int) bool { return c[i].ID < c[j].ID }

// SortConnections sorts a slice of HubConnections.
func SortConnections(c []*HubConnection) {
	sort.Sort(connections(c))
}
