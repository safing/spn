package access

import (
	"fmt"
	"sync"
)

var (
	zones     = make(map[string]CodeHandler)
	zonesLock sync.Mutex
)

// RegisterZone registers a handler with the given zone name.
func RegisterZone(zone string, handler CodeHandler) {
	zonesLock.Lock()
	defer zonesLock.Unlock()

	zones[zone] = handler
}

// GetZoneHandler returns the handler for the given zone.
func GetZoneHandler(zone string) (CodeHandler, error) {
	zonesLock.Lock()
	defer zonesLock.Unlock()

	handler, ok := zones[zone]
	if !ok {
		return nil, fmt.Errorf("no handler for zone %q registered", zone)
	}

	return handler, nil
}

// Generate generates a new code for the given zone.
func Generate(zone string) (*Code, error) {
	handler, err := GetZoneHandler(zone)
	if err != nil {
		return nil, err
	}

	code, err := handler.Generate()
	if err != nil {
		return nil, err
	}

	code.Zone = zone
	return code, nil
}

// Check checks if the given code is valid.
func Check(code *Code) error {
	handler, err := GetZoneHandler(code.Zone)
	if err != nil {
		return err
	}

	return handler.Check(code)
}

// Import imports a code into the given zone.
func Import(code *Code) error {
	handler, err := GetZoneHandler(code.Zone)
	if err != nil {
		return err
	}

	return handler.Import(code)
}

func Get() (code *Code, err error) {
	zonesLock.Lock()
	defer zonesLock.Unlock()

	// TODO: priorities preferred methods
	for _, handler := range zones {
		code, err = handler.Get()
		if err == nil {
			return code, nil
		}
	}
	return nil, err
}
