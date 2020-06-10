package hub

import "github.com/safing/jess"

// SingleTrustStore is a simple truststore that always returns the same Signet.
type SingleTrustStore struct {
	Signet *jess.Signet
}

// GetSignet implements the truststore interface.
func (ts *SingleTrustStore) GetSignet(_ string, _ bool) (*jess.Signet, error) {
	return ts.Signet, nil
}
