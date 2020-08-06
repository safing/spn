package access

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/safing/portbase/container"
	"github.com/safing/portbase/rng"
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
	return c.Zone + ":" + base64.RawURLEncoding.EncodeToString(c.Data)
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

	data, err := base64.RawURLEncoding.DecodeString(splitted[1])
	if err != nil {
		return nil, fmt.Errorf("invalid code format: %s", err)
	}

	return &Code{
		Zone: splitted[0],
		Data: data,
	}, nil
}

func getBeautifulRandom(n int) (randomData []byte, err error) {
	for i := 0; i < 10000; i++ {
		// get random data
		randomData, err = rng.Bytes(n)
		if err != nil {
			return
		}

		if strings.ContainsAny(base64.RawURLEncoding.EncodeToString(randomData), "_-") {
			continue
		}
	}
	return
}
