package access

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/safing/portbase/api"
	"github.com/safing/portbase/database"
	"github.com/safing/portbase/formats/dsd"
	"github.com/safing/portbase/log"
	"github.com/safing/spn/access/account"
)

// https://account.safing.io/v1/authenticate

const (
	AccountServer   = "https://account.safing.io"
	LoginPath       = "/v1/authenticate"
	UserProfilePath = "/v1/user_profile"
)

var (
	accountClient     = &http.Client{}
	clientRequestLock sync.Mutex
)

func login(ctx context.Context, username, password string) (user *UserRecord, code int, err error) {
	clientRequestLock.Lock()
	defer clientRequestLock.Unlock()

	// Get previous user.
	previousUser, err := GetUser()
	if err != nil {
		previousUser = nil
	}

newRequest:

	// Create new request.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, AccountServer+LoginPath, nil)

	// Add username and password.
	request.SetBasicAuth(username, password)

	// Try to reuse the device ID, if the username matches the previous user.
	if previousUser != nil && username == previousUser.Username {
		request.Header.Set(account.AuthHeaderDevice, previousUser.Device.ID)
	}

	// Set requested HTTP response format.
	_, err = dsd.RequestHTTPResponseFormat(request, dsd.JSON)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to set requested response format: %w", err)
	}

	// Make request.
	resp, err := accountClient.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		// All good!

	case account.StatusInvalidAuth:
		// Wrond username / password.
		return nil, resp.StatusCode, ErrInvalidCredentials

	case account.StatusReachedDeviceLimit:
		// Device limit is reached.
		return nil, resp.StatusCode, ErrDeviceLimitReached

	case account.StatusDeviceInactive:
		// Device is locked.
		return nil, resp.StatusCode, ErrDeviceIsLocked

	case account.StatusInvalidDevice:
		// Given device is invalid or inactive.
		if previousUser != nil {
			// Try again without the previous user.
			previousUser = nil
			goto newRequest
		}

		fallthrough
	default:
		return nil, resp.StatusCode, fmt.Errorf("unexpected reply: [%d] %s", resp.StatusCode, resp.Status)
	}

	// Load response data.
	userAccount := &account.User{}
	_, err = dsd.LoadFromHTTPResponse(resp, userAccount)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to parse response: %w", err)
	}

	// Save new user.
	now := time.Now()
	user = &UserRecord{
		User:       userAccount,
		LoggedInAt: &now,
	}
	err = user.Save()
	if err != nil {
		return user, resp.StatusCode, fmt.Errorf("failed to save new user profile: %w", err)
	}

	// Save initial auth token.
	err = SaveNewAuthToken(user.Device.ID, resp)
	if err != nil {
		return user, resp.StatusCode, fmt.Errorf("failed to save initial auth token: %w", err)
	}

	return user, resp.StatusCode, nil
}

func logout(purge bool) error {
	clientRequestLock.Lock()
	defer clientRequestLock.Unlock()

	// Clear caches.
	clearUserCaches()

	// Delete auth token.
	err := db.Delete(authTokenRecordKey)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("failed to delete auth token: %w", err)
	}

	// Delete all user data if purging.
	if purge {
		err := db.Delete(userRecordKey)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return fmt.Errorf("failed to delete user: %w", err)
		}

		return nil
	}

	// Else, keep just the username and device ID in order to log into the same device again.
	user, err := GetUser()
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("failed to load user for logout: %w", err)
	}

	// Reset all user data, except for username and device ID.
	func() {
		user.Lock()
		defer user.Unlock()

		user.User = &account.User{
			Username: user.Username,
			Device: &account.Device{
				ID: user.Device.ID,
			},
		}
		user.LoggedInAt = &time.Time{}
	}()
	err = user.Save()
	if err != nil {
		return fmt.Errorf("failed to save user for logout: %w", err)
	}

	return nil
}

func getUserProfile(ctx context.Context) (user *UserRecord, statusCode int, err error) {
	clientRequestLock.Lock()
	defer clientRequestLock.Unlock()

	// Get auth token to apply to request.
	authToken, err := GetAuthToken()
	if err != nil {
		return nil, 0, ErrNotLoggedIn
	}

	// Create new request.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, AccountServer+UserProfilePath, nil)

	// Set auth token.
	authToken.Token.ApplyTo(request)

	// Set requested HTTP response format.
	_, err = dsd.RequestHTTPResponseFormat(request, dsd.JSON)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to set requested response format: %w", err)
	}

	// Make request.
	resp, err := accountClient.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	// All good!
	case account.StatusInvalidAuth,
		account.StatusInvalidDevice,
		account.StatusReachedDeviceLimit,
		account.StatusDeviceInactive:
		// We were logged out!
		err = logout(false)
		if err != nil {
			log.Warningf("access: failed to log out user because of failed request [%d]: %s", resp.StatusCode, err)
		}

		fallthrough
	default:
		return nil, resp.StatusCode, api.ErrorWithStatus(
			fmt.Errorf("unexpected reply: [%d] %s", resp.StatusCode, resp.Status),
			resp.StatusCode,
		)
	}

	// Save next auth token.
	err = authToken.Update(resp)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to save next auth token: %w", err)
	}

	// Load response data.
	userData := &account.User{}
	_, err = dsd.LoadFromHTTPResponse(resp, userData)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to parse response: %w", err)
	}

	// Save to previous user, if exists.
	previousUser, err := GetUser()
	if err == nil {
		func() {
			previousUser.Lock()
			defer previousUser.Unlock()
			previousUser.User = userData
		}()
		err := previousUser.Save()
		if err != nil {
			log.Warningf("access: failed to save updated user profile: %s", err)
		}
		return previousUser, resp.StatusCode, nil
	}

	// Else, save as new user.
	now := time.Now()
	newUser := &UserRecord{
		User:       userData,
		LoggedInAt: &now,
	}
	err = newUser.Save()
	if err != nil {
		log.Warningf("access: failed to save new user profile: %s", err)
	}
	return newUser, resp.StatusCode, nil
}
