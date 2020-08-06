package access

import (
	"errors"
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
	verifier, err := lhash.LoadFromString(verificationHash)
	if err != nil {
		return nil, err
	}

	return &SaticCodeHandler{
		verifier:    verifier,
		scrambleAlg: scrambleAlg,
	}, nil
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
	if !h.verifier.Matches(code.Data) {
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
	if !h.verifier.Matches(scrambled.Bytes()) {
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
