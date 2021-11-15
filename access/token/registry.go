package token

import "sync"

// Handler represents a token handling system.
type Handler interface {
	// Zone returns the zone name.
	Zone() string

	// ShouldRequest returns whether the new tokens should be requested.
	ShouldRequest() bool

	// GetToken returns a token.
	GetToken() (token *Token, err error)

	// Verify verifies the given token.
	Verify(token *Token) error

	// Save serializes and returns the current tokens.
	Save() ([]byte, error)

	// Load loads the given tokens into the handler.
	Load(data []byte) error
}

var (
	registry         = make(map[string]Handler)
	pblindRegistry   = make([]*PBlindHandler, 0, 1)
	scrambleRegistry = make([]*ScrambleHandler, 0, 1)

	registryLock sync.RWMutex
)

func RegisterPBlindHandler(h *PBlindHandler) error {
	registryLock.Lock()
	defer registryLock.Unlock()

	if err := registerHandler(h, h.opts.Zone); err != nil {
		return err
	}

	pblindRegistry = append(pblindRegistry, h)
	return nil
}

func RegisterScrambleHandler(h *ScrambleHandler) error {
	registryLock.Lock()
	defer registryLock.Unlock()

	if err := registerHandler(h, h.opts.Zone); err != nil {
		return err
	}

	scrambleRegistry = append(scrambleRegistry, h)
	return nil
}

func registerHandler(h Handler, zone string) error {
	if zone == "" {
		return ErrNoZone
	}

	_, ok := registry[zone]
	if ok {
		return ErrZoneTaken
	}

	registry[zone] = h
	return nil
}

func GetHandler(zone string) (handler Handler, ok bool) {
	registryLock.RLock()
	defer registryLock.RUnlock()

	handler, ok = registry[zone]
	return
}
