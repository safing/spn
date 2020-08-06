package access

import (
	"testing"

	"github.com/safing/portbase/rng"
)

func TestCode(t *testing.T) {
	randomData, err := rng.Bytes(32)
	if err != nil {
		t.Fatal(err)
	}

	c := &Code{
		Zone: "test",
		Data: randomData,
	}

	s := c.String()
	_, err = ParseCode(s)
	if err != nil {
		t.Fatal(err)
	}

	r := c.Raw()
	_, err = ParseRawCode(r)
	if err != nil {
		t.Fatal(err)
	}
}
