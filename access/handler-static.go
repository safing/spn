package access

import (
	"errors"
	"fmt"
	"sync"

	"github.com/safing/jess/lhash"
	"github.com/safing/portbase/rng"
)

const (
	staticCodeSecretSize = 32
)

type SaticCodeHandler struct {
	lock sync.Mutex

	verifier *lhash.LabeledHash

	scrambleAlg   lhash.Algorithm
	scrambledCode *Code
}

func NewSaticCodeHandler(verificationHash string, scrambleAlg lhash.Algorithm) (*SaticCodeHandler, error) {
	verifier, err := lhash.FromBase58(verificationHash)
	if err != nil {
		return nil, err
	}

	return &SaticCodeHandler{
		verifier:    verifier,
		scrambleAlg: scrambleAlg,
	}, nil
}

func BootstrapStaticCodeHandler(zone string, scrambleAlg lhash.Algorithm) (
	h *SaticCodeHandler,
	accessCode *Code,
	scrambledCode *Code,
	verificationHash *lhash.LabeledHash,
	err error,
) {
	h = &SaticCodeHandler{
		scrambleAlg: scrambleAlg,
	}

	// Generate access code.
	accessCode, err = h.Generate()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to generate access code: %w", err)
	}
	accessCode.Zone = zone

	// Scramble the access code manually as importing it requires a check with
	// the yet non-existent verifier.
	scrambledCode = &Code{
		Zone: zone,
		Data: lhash.Digest(h.scrambleAlg, accessCode.Data).Bytes(),
	}

	// Create verification hash.
	verificationHash = lhash.Digest(h.scrambleAlg, scrambledCode.Data)
	h.verifier = verificationHash

	// Sanity check the bootstrapped handler.
	// check if scrambled code is valid
	err = h.Import(accessCode)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to check access code: %w", err)
	}
	err = h.Check(scrambledCode)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to check scrambled code: %w", err)
	}

	return
}

func (h *SaticCodeHandler) Generate() (*Code, error) {
	// get random data
	randomData, err := rng.Bytes(staticCodeSecretSize)
	if err != nil {
		return nil, err
	}

	return &Code{Data: randomData}, nil
}

func (h *SaticCodeHandler) Check(code *Code) error {
	if !h.verifier.MatchesData(code.Data) {
		return errors.New("code is invalid")
	}

	return nil
}

func (h *SaticCodeHandler) Import(code *Code) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	// scramble data on import
	scrambled := lhash.Digest(h.scrambleAlg, code.Data)

	// check
	if !h.verifier.MatchesData(scrambled.Bytes()) {
		return errors.New("code is invalid")
	}

	// save
	h.scrambledCode = &Code{
		Zone: code.Zone,
		Data: scrambled.Bytes(),
	}

	return nil
}

func (h *SaticCodeHandler) Get() (*Code, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	if h.scrambledCode == nil {
		return nil, errors.New("no code imported")
	}

	return h.scrambledCode, nil
}
