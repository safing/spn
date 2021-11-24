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

var (
	ErrMissingDeviceID = errors.New("missing device ID")
	ErrMissingToken    = errors.New("missing token")
)

type AuthToken struct {
	Device string
	Token  string
}

func GetAuthTokenFromRequest(request *http.Request) (*AuthToken, error) {
	device := request.Header.Get(AuthHeaderDevice)
	if device == "" {
		return nil, ErrMissingDeviceID
	}
	token := request.Header.Get(AuthHeaderToken)
	if token == "" {
		return nil, ErrMissingToken
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

func GetNextTokenFromResponse(resp *http.Response) (token string, ok bool) {
	token = resp.Header.Get(AuthHeaderNextToken)
	if token == "" {
		// TODO: Remove when fixed on server.
		token = resp.Header.Get(AuthHeaderNextTokenDeprecated)
	}

	return token, token != ""
}

func ApplyNextTokenToResponse(resp *http.Response, token string) {
	resp.Header.Set(AuthHeaderNextToken, token)
}
