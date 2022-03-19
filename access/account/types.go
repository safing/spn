package account

import "time"

// User, Subscription and Charge states.
const (
	// UserStateNone is only used within Portmaster for saving information for
	// logging into the same device.
	UserStateNone      = ""
	UserStateFresh     = "fresh"
	UserStateQueued    = "queued"
	UserStateApproved  = "approved"
	UserStateSuspended = "suspended"
	UserStateLoggedOut = "loggedout" // Portmaster only.

	SubscriptionStatePending   = "pending"
	SubscriptionStateActive    = "active"
	SubscriptionStateCancelled = "cancelled"
	SubscriptionStateExpired   = "expired"

	ChargeStatePending   = "pending"
	ChargeStateCompleted = "completed"
	ChargeStateDead      = "dead"
)

// Agent and Hub return statuses.
const (
	// StatusInvalidAuth [401 Unauthorized] is returned when the credentials are
	// invalid or the user was logged out.
	StatusInvalidAuth = 401
	// StatusInvalidDevice [404 Not Found] is returned when the device trying to
	// log into does not exist.
	StatusInvalidDevice = 404
	// StatusReachedDeviceLimit [409 Conflict] is returned when the device limit is reached.
	StatusReachedDeviceLimit = 409
	// StatusDeviceInactive [423 Locked] is returned when the device is locked.
	StatusDeviceInactive = 423
	// StatusNotLoggedIn [412 Precondition] is returned by the Portmaster, if an action required to be logged in, but the user is not logged in.
	StatusNotLoggedIn = 412
)

// User describes an SPN user account.
type User struct {
	Username     string        `json:"username"`
	State        string        `json:"state"`
	Balance      int           `json:"balance"`
	Device       *Device       `json:"device"`
	Subscription *Subscription `json:"subscription"`
	CurrentPlan  *Plan         `json:"current_plan"`
	NextPlan     *Plan         `json:"next_plan"`
}

// MayUseSPN return whether the user may currently use the SPN.
func (u *User) MayUseSPN() bool {
	return u.State == UserStateApproved &&
		u.Subscription != nil &&
		time.Now().Before(u.Subscription.EndsAt)
}

// Device describes a device of an SPN user.
type Device struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Subscription describes an SPN subscription.
type Subscription struct {
	EndsAt time.Time `json:"ends_at"`
	State  string    `json:"state"`
}

// Plan describes an SPN subscription plan.
type Plan struct {
	Name      string `json:"name"`
	Amount    int    `json:"amount"`
	Months    int    `json:"months"`
	Renewable bool   `json:"renewable"`
}
