package account

import (
	"errors"
	"net/http"
)

const (
	AuthHeaderDevice              = "Device-17"
	AuthHeaderToken               = "Token-17"
	AuthHeaderNextToken           = "Next-Token-17"
	AuthHeaderNextTokenDeprecated = "Next_token_17"
)

type AuthToken struct {
	Device string
	Token  string
}

func GetAuthTokenFromRequest(request *http.Request) (*AuthToken, error) {
	device := request.Header.Get(AuthHeaderDevice)
	if device == "" {
		return nil, errors.New("device ID is missing")
	}
	token := request.Header.Get(AuthHeaderToken)
	if token == "" {
		return nil, errors.New("token is missing")
	}

	return &AuthToken{
		Device: device,
		Token:  token,
	}, nil
}

func (at *AuthToken) ApplyTo(request *http.Request) {
	request.Header.Set(AuthHeaderDevice, at.Device)
	request.Header.Set(AuthHeaderToken, at.Token)
}
