package access

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/record"
	"github.com/safing/spn/access/account"
)

const (
	userRecordKey           = "core:spn/account/user"
	authTokenRecordKey      = "core:spn/account/authtoken" //nolint:gosec // Not a credential.
	tokenStorageKeyTemplate = "core:spn/account/tokens/%s" //nolint:gosec // Not a credential.
)

var db = database.NewInterface(&database.Options{
	Local:    true,
	Internal: true,
})

// UserRecord holds a SPN user account.
type UserRecord struct {
	record.Base
	sync.Mutex

	*account.User

	LastNotifiedOfEnd *time.Time
	LoggedInAt        *time.Time
}

// AuthTokenRecord holds an authentication token.
type AuthTokenRecord struct {
	record.Base
	sync.Mutex

	Token *account.AuthToken
}

// GetToken returns the token from the record.
func (authToken *AuthTokenRecord) GetToken() *account.AuthToken {
	authToken.Lock()
	defer authToken.Unlock()

	return authToken.Token
}

// SaveNewAuthToken saves a new auth token to the database.
func SaveNewAuthToken(deviceID string, resp *http.Response) error {
	token, ok := account.GetNextTokenFromResponse(resp)
	if !ok {
		return account.ErrMissingToken
	}

	newAuthToken := &AuthTokenRecord{
		Token: &account.AuthToken{
			Device: deviceID,
			Token:  token,
		},
	}
	return newAuthToken.Save()
}

// Update updates an existing auth token with the next token from a response.
func (authToken *AuthTokenRecord) Update(resp *http.Response) error {
	token, ok := account.GetNextTokenFromResponse(resp)
	if !ok {
		return account.ErrMissingToken
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

// GetUser returns the current user account.
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
		newUser := &UserRecord{}
		err = record.Unwrap(r, newUser)
		if err != nil {
			return nil, err
		}
		cachedUser = newUser
		return cachedUser, nil
	}

	// Or adjust type.
	newUser, ok := r.(*UserRecord)
	if !ok {
		return nil, fmt.Errorf("record not of type *UserRecord, but %T", r)
	}
	cachedUser = newUser
	return cachedUser, nil
}

// Save saves the User.
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

// GetAuthToken returns the current auth token.
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
		newAuthRecord := &AuthTokenRecord{}
		err = record.Unwrap(r, newAuthRecord)
		if err != nil {
			return nil, err
		}
		cachedAuthToken = newAuthRecord
		return newAuthRecord, nil
	}

	// Or adjust type.
	newAuthRecord, ok := r.(*AuthTokenRecord)
	if !ok {
		return nil, fmt.Errorf("record not of type *AuthTokenRecord, but %T", r)
	}
	cachedAuthToken = newAuthRecord
	return newAuthRecord, nil
}

// Save saves the auth token to the database.
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
