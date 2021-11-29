package token

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"

	"github.com/safing/portbase/container"
)

type Token struct {
	Zone string
	Data []byte
}

func GetToken(zone string) (*Token, error) {
	handler, ok := GetHandler(zone)
	if !ok {
		return nil, ErrZoneUnknown
	}

	return handler.GetToken()
}

func VerifyToken(token *Token) error {
	handler, ok := GetHandler(token.Zone)
	if !ok {
		return ErrZoneUnknown
	}

	return handler.Verify(token)
}

func (c *Token) Raw() []byte {
	cont := container.New()
	cont.Append([]byte(c.Zone))
	cont.Append([]byte(":"))
	cont.Append(c.Data)
	return cont.CompileData()
}

func (c *Token) String() string {
	return c.Zone + ":" + base58.Encode(c.Data)
}

func ParseRawToken(code []byte) (*Token, error) {
	splitted := bytes.SplitN(code, []byte(":"), 2)
	if len(splitted) < 2 {
		return nil, errors.New("invalid code format: zone/data separator missing")
	}

	return &Token{
		Zone: string(splitted[0]),
		Data: splitted[1],
	}, nil
}

func ParseToken(code string) (*Token, error) {
	splitted := strings.SplitN(code, ":", 2)
	if len(splitted) < 2 {
		return nil, errors.New("invalid code format: zone/data separator missing")
	}

	data, err := base58.Decode(splitted[1])
	if err != nil {
		return nil, fmt.Errorf("invalid code format: %s", err)
	}

	return &Token{
		Zone: splitted[0],
		Data: data,
	}, nil
}
