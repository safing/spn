package access

import (
	"fmt"
	"sync"

	"github.com/safing/spn/terminal"
)

var (
	zones     = make(map[string]*Zone)
	zonesLock sync.Mutex
)

type Zone struct {
	handler CodeHandler
	grants  terminal.Permission
}

// RegisterZone registers a handler with the given zone name.
func RegisterZone(zone string, handler CodeHandler, grants terminal.Permission) {
	zonesLock.Lock()
	defer zonesLock.Unlock()

	zones[zone] = &Zone{
		handler: handler,
		grants:  grants,
	}
}

// GetZone returns the zone data for the given zone ID.
func GetZone(zoneID string) (*Zone, error) {
	zonesLock.Lock()
	defer zonesLock.Unlock()

	zone, ok := zones[zoneID]
	if !ok {
		return nil, fmt.Errorf("zone %q not registered", zone)
	}

	return zone, nil
}

// Generate generates a new code for the given zone.
func Generate(zoneID string) (*Code, error) {
	zone, err := GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	code, err := zone.handler.Generate()
	if err != nil {
		return nil, err
	}

	code.Zone = zoneID
	return code, nil
}

// Check checks if the given code is valid.
func Check(code *Code) (granted terminal.Permission, err error) {
	zone, err := GetZone(code.Zone)
	if err != nil {
		return 0, err
	}

	err = zone.handler.Check(code)
	if err != nil {
		return 0, err
	}

	return zone.grants, nil
}

// Import imports a code into the given zone.
func Import(code *Code) error {
	zone, err := GetZone(code.Zone)
	if err != nil {
		return err
	}

	return zone.handler.Import(code)
}

func Get() (code *Code, err error) {
	zonesLock.Lock()
	defer zonesLock.Unlock()

	// TODO: priorities preferred methods
	for _, zone := range zones {
		code, err = zone.handler.Get()
		if err == nil {
			return code, nil
		}
	}
	return nil, err
}
