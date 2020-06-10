package hub

import (
	"fmt"
	"net"
	"testing"

	"github.com/safing/jess"
	"github.com/safing/portbase/formats/dsd"
)

func init() {
	SetHubIPValidationFn(func(hub *Hub, ip net.IP) error {
		return nil
	})
}

func TestHubUpdate(t *testing.T) {
	// message signing

	testData := []byte{0}

	s1, err := jess.GenerateSignet("Ed25519", 0)
	if err != nil {
		t.Fatal(err)
	}
	err = s1.StoreKey()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("s1: %+v\n", s1)

	s1e, err := s1.AsRecipient()
	if err != nil {
		t.Fatal(err)
	}
	err = s1e.StoreKey()
	if err != nil {
		t.Fatal(err)
	}
	s1e.ID = createHubID(s1e.Scheme, s1e.Key)
	s1.ID = s1e.ID

	e := jess.Envelope{
		SuiteID: jess.SuiteSignV1,
		Senders: []*jess.Signet{s1},
	}
	s, err := e.Correspondence(nil)
	if err != nil {
		t.Fatal(err)
	}
	letter, err := s.Close(testData)
	if err != nil {
		t.Fatal(err)
	}

	// smuggle the key
	letter.Keys = append(letter.Keys, &jess.Seal{
		Value: s1e.Key,
	})
	fmt.Printf("letter: %+v\n", letter)

	// pack
	data, err := letter.ToDSD(dsd.JSON)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = OpenHubMsg(data, ScopePublic, true)
	if err != nil {
		t.Fatal(err)
	}
}
