package api

import (
	"testing"

	"github.com/safing/portbase/container"
)

func TestError(t *testing.T) {
	errMsg := NewApiError("somethings wrong").MarkAsTemporary().Error()

	err := ParseError(container.NewContainer([]byte(errMsg)))
	if !err.Temporary {
		t.Fatal("ApiError should be temporary")
	}

	if err.Error() != "[temp] somethings wrong" {
		t.Fatalf("unexpected error string: %s", err.Error())
	}
}
