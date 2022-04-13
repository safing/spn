package crew

import (
	"context"
	"sync"
	"time"

	"github.com/safing/portbase/modules"
	"github.com/safing/portmaster/network"
	"github.com/safing/spn/navigator"
)

const (
	stickyTTL = 1 * time.Hour
)

var (
	stickyIPs     = make(map[string]*stickyHub)
	stickyDomains = make(map[string]*stickyHub)
	stickyLock    sync.Mutex
)

type stickyHub struct {
	Pin      *navigator.Pin
	Route    *navigator.Route
	LastSeen time.Time
	Avoid    bool
}

func (sh *stickyHub) isExpired() bool {
	return time.Now().Add(-stickyTTL).After(sh.LastSeen)
}

func getStickiedHub(conn *network.Connection) *stickyHub {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	// Check if IP is sticky.
	sticksTo, ok := stickyIPs[string(conn.Entity.IP)] // byte comparison
	if ok && !sticksTo.isExpired() {
		sticksTo.LastSeen = time.Now()
		return sticksTo
	}

	// Check if Domain is sticky, if present.
	if conn.Entity.Domain != "" {
		sticksTo, ok := stickyDomains[conn.Entity.Domain]
		if ok && !sticksTo.isExpired() {
			sticksTo.LastSeen = time.Now()
			return sticksTo
		}
	}

	return nil
}

func (t *Tunnel) stickDestinationToHub() {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	// Stick to IP.
	stickyIPs[string(t.connInfo.Entity.IP)] = &stickyHub{
		Pin:      t.dstPin,
		Route:    t.route,
		LastSeen: time.Now(),
	}

	// Stick to Domain, if present.
	if t.connInfo.Entity.Domain != "" {
		stickyDomains[t.connInfo.Entity.Domain] = &stickyHub{
			Pin:      t.dstPin,
			Route:    t.route,
			LastSeen: time.Now(),
		}
	}
}

func (t *Tunnel) avoidDestinationHub() {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	// Stick to Hub/IP Pair.
	stickyIPs[string(t.connInfo.Entity.IP)] = &stickyHub{
		Pin:      t.dstPin,
		LastSeen: time.Now(),
		Avoid:    true,
	}
}

func cleanStickyHubs(ctx context.Context, task *modules.Task) error {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	for _, stickyRegistry := range []map[string]*stickyHub{stickyIPs, stickyDomains} {
		for key, stickedEntry := range stickyRegistry {
			if stickedEntry.isExpired() {
				delete(stickyRegistry, key)
			}
		}
	}

	return nil
}

func clearStickyHubs() {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	for _, stickyRegistry := range []map[string]*stickyHub{stickyIPs, stickyDomains} {
		for key := range stickyRegistry {
			delete(stickyRegistry, key)
		}
	}
}
