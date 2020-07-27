package hub

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/safing/portmaster/core/pmtesting"
)

func TestMain(m *testing.M) {
	pmtesting.TestMain(m, nil)
}

func TestEquality(t *testing.T) {
	// empty match
	a := &HubAnnouncement{}
	assert.True(t, a.Equal(a), "should match itself")

	// full match
	a = &HubAnnouncement{
		ID:             "a",
		Timestamp:      1,
		Name:           "a",
		ContactAddress: "a",
		ContactService: "a",
		Hosters:        []string{"a", "b"},
		Datacenter:     "a",
		IPv4:           net.IPv4(1, 2, 3, 4),
		IPv6:           net.ParseIP("::1"),
		Transports:     []string{"a", "b"},
		Entry:          []string{"a", "b"},
		Exit:           []string{"a", "b"},
	}
	assert.True(t, a.Equal(a), "should match itself")

	// no match
	b := &HubAnnouncement{ID: "b"}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{Timestamp: 2}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{Name: "b"}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{ContactAddress: "b"}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{ContactService: "b"}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{Hosters: []string{"b", "c"}}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{Datacenter: "b"}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{IPv4: net.IPv4(1, 2, 3, 5)}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{IPv6: net.ParseIP("::2")}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{Transports: []string{"b", "c"}}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{Entry: []string{"b", "c"}}
	assert.False(t, a.Equal(b), "should not match")
	b = &HubAnnouncement{Exit: []string{"b", "c"}}
	assert.False(t, a.Equal(b), "should not match")
}
