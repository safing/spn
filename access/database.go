package access

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/record"
	"github.com/safing/spn/access/account"
)

const (
	userRecordKey      = "core:spn/account/user"
	authTokenRecordKey = "core:spn/account/authtoken"
)

var db = database.NewInterface(&database.Options{
	Local:    true,
	Internal: true,
})

type UserRecord struct {
	record.Base
	sync.Mutex

	*account.User

	LoggedInAt *time.Time
}

type AuthTokenRecord struct {
	record.Base
	sync.Mutex

	Token *account.AuthToken
}

func (authToken *AuthTokenRecord) GetToken() *account.AuthToken {
	authToken.Lock()
	defer authToken.Unlock()

	return authToken.Token
}

func SaveNewAuthToken(deviceID string, resp *http.Response) error {
	token := resp.Header.Get(account.AuthHeaderNextToken)
	if token == "" {
		// TODO: Remove when fixed on server.
		token = resp.Header.Get(account.AuthHeaderNextTokenDeprecated)
	}
	if token == "" {
		fmt.Printf("%+v", resp)
		fmt.Printf("%+v", resp.Header)
		return errors.New("token is missing")
	}

	newAuthToken := &AuthTokenRecord{
		Token: &account.AuthToken{
			Device: deviceID,
			Token:  token,
		},
	}
	return newAuthToken.Save()
}

func (authToken *AuthTokenRecord) Update(resp *http.Response) error {
	token := resp.Header.Get(account.AuthHeaderNextToken)
	if token == "" {
		// TODO: Remove when fixed on server.
		token = resp.Header.Get(account.AuthHeaderNextTokenDeprecated)
	}
	if token == "" {
		return errors.New("token is missing")
	}

	// Update token with new account.AuthToken.
	func() {
		authToken.Lock()
		defer authToken.Unlock()

		authToken.Token = &account.AuthToken{
			Device: authToken.Token.Device,
			Token:  token,
		}
	}()

	return authToken.Save()
}

var (
	cachedUser       *UserRecord
	cachedAuthToken  *AuthTokenRecord
	accountCacheLock sync.Mutex
)

func clearUserCaches() {
	accountCacheLock.Lock()
	defer accountCacheLock.Unlock()

	cachedUser = nil
	cachedAuthToken = nil
}

func GetUser() (*UserRecord, error) {
	// Check cache.
	accountCacheLock.Lock()
	defer accountCacheLock.Unlock()
	if cachedUser != nil {
		return cachedUser, nil
	}

	// Load from disk.
	r, err := db.Get(userRecordKey)
	if err != nil {
		return nil, err
	}

	// Unwrap record.
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &UserRecord{}
		err = record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		cachedUser = new
		return cachedUser, nil
	}

	// Or adjust type.
	new, ok := r.(*UserRecord)
	if !ok {
		return nil, fmt.Errorf("record not of type *UserRecord, but %T", r)
	}
	cachedUser = new
	return cachedUser, nil
}

func (user *UserRecord) Save() error {
	// Update cache.
	accountCacheLock.Lock()
	defer accountCacheLock.Unlock()
	cachedUser = user

	// Set, check and update metadata.
	if !user.KeyIsSet() {
		user.SetKey(userRecordKey)
	}
	user.UpdateMeta()

	return db.Put(user)
}

func GetAuthToken() (*AuthTokenRecord, error) {
	// Check cache.
	accountCacheLock.Lock()
	defer accountCacheLock.Unlock()
	if cachedAuthToken != nil {
		return cachedAuthToken, nil
	}

	// Load from disk.
	r, err := db.Get(authTokenRecordKey)
	if err != nil {
		return nil, err
	}

	// Unwrap record.
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &AuthTokenRecord{}
		err = record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		cachedAuthToken = new
		return new, nil
	}

	// Or adjust type.
	new, ok := r.(*AuthTokenRecord)
	if !ok {
		return nil, fmt.Errorf("record not of type *AuthTokenRecord, but %T", r)
	}
	cachedAuthToken = new
	return new, nil
}

func (authToken *AuthTokenRecord) Save() error {
	// Update cache.
	accountCacheLock.Lock()
	defer accountCacheLock.Unlock()
	cachedAuthToken = authToken

	// Set, check and update metadata.
	if !authToken.KeyIsSet() {
		authToken.SetKey(authTokenRecordKey)
	}
	authToken.UpdateMeta()
	authToken.Meta().MakeSecret()
	authToken.Meta().MakeCrownJewel()

	return db.Put(authToken)
}
