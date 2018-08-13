package api

import (
	"strings"

	"github.com/Safing/safing-core/container"
)

type ApiError struct {
	Temporary bool
	Msg       string
}

func NewApiError(msg string) *ApiError {
	return &ApiError{
		Msg: msg,
	}
}

func ParseError(c *container.Container) *ApiError {
	err := &ApiError{
		Msg: string(c.CompileData()),
	}

	if strings.HasPrefix(err.Msg, "[temp] ") {
		err.Temporary = true
	}

	return err
}

func (err *ApiError) Error() string {
	return err.Msg
}

func (err *ApiError) Bytes() []byte {
	return []byte(err.Msg)
}

func (err *ApiError) MarkAsTemporary() *ApiError {
	if !err.Temporary {
		err.Msg = "[temp] " + err.Msg
		err.Temporary = true
	}
	return err
}
