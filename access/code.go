package access

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"

	"github.com/safing/portbase/container"
)

type Code struct {
	Zone string
	Data []byte
}

type CodeHandler interface {
	Generate() (*Code, error)
	Check(code *Code) error
	Import(code *Code) error
	Get() (*Code, error)
}

func (c *Code) Raw() []byte {
	cont := container.New()
	cont.Append([]byte(c.Zone))
	cont.Append([]byte(":"))
	cont.Append(c.Data)
	return cont.CompileData()
}

func (c *Code) String() string {
	return c.Zone + ":" + base58.Encode(c.Data)
}

func ParseRawCode(code []byte) (*Code, error) {
	splitted := bytes.SplitN(code, []byte(":"), 2)
	if len(splitted) < 2 {
		return nil, errors.New("invalid code format: zone/data separator missing")
	}

	return &Code{
		Zone: string(splitted[0]),
		Data: splitted[1],
	}, nil
}

func ParseCode(code string) (*Code, error) {
	splitted := strings.SplitN(code, ":", 2)
	if len(splitted) < 2 {
		return nil, errors.New("invalid code format: zone/data separator missing")
	}

	data, err := base58.Decode(splitted[1])
	if err != nil {
		return nil, fmt.Errorf("invalid code format: %s", err)
	}

	return &Code{
		Zone: splitted[0],
		Data: data,
	}, nil
}
