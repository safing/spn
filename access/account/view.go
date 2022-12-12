package account

import (
	"fmt"
	"strings"
	"time"
)

// View holds metadata that assists in displaying account information.
type View struct {
	Message           string
	ShowAccountData   bool
	ShowAccountButton bool
	ShowLoginButton   bool
	ShowRefreshButton bool
	ShowLogoutButton  bool
}

// UpdateView updates the view and handles plan/package fallbacks.
func (u *User) UpdateView(requestStatusCode int) {
	v := &View{}

	// Clean up naming and fallbacks when finished.
	defer func() {
		// Display "Free" package if no plan is set or if it expired.
		if u.CurrentPlan == nil ||
			u.Subscription == nil ||
			u.Subscription.EndsAt == nil ||
			time.Now().After(*u.Subscription.EndsAt) {
			u.CurrentPlan = &Plan{
				Name: "Free",
			}
		}

		// Prepend "Portmaster " to plan name.
		// TODO: Remove when Plan/Package naming has been updated.
		if !strings.HasPrefix(u.CurrentPlan.Name, "Portmaster ") {
			u.CurrentPlan.Name = "Portmaster " + u.CurrentPlan.Name
		}

		// Apply new view to user.
		u.View = v
	}()

	// Set view data based on return code.
	switch requestStatusCode {
	case StatusInvalidAuth, StatusInvalidDevice, StatusDeviceInactive:
		// Account deleted or Device inactive or deleted.
		// When using token based auth, there is no difference between these cases.
		v.Message = "This device was deactivated or removed from your account. Please log in again."
		v.ShowAccountData = true
		v.ShowAccountButton = true
		v.ShowLoginButton = true
		v.ShowLogoutButton = true
		return

	case StatusUnknownError:
		v.Message = "There is an unknown error in the communication with the account server. The shown information may not be up to date. "

	case StatusConnectionError:
		v.Message = "Portmaster could not connect to the account server. The shown information may not be up to date. "
	}

	// Set view data based on profile data.
	switch {
	case u.State == UserStateLoggedOut:
		// User logged out.
		v.ShowAccountButton = true
		v.ShowLoginButton = true
		return

	case u.State == UserStateSuspended:
		// Account is suspended.
		v.Message += fmt.Sprintf("Your account (%s) was suspended. Please contact support for details.", u.Username)
		v.ShowAccountButton = true
		v.ShowRefreshButton = true
		v.ShowLogoutButton = true
		return

	case u.Subscription == nil || u.Subscription.EndsAt == nil:
		// Account has never had a subscription.
		v.Message += "Upgrade on the Account Page to protect your privacy even more."

	case time.Now().After(*u.Subscription.EndsAt):
		// Subscription expired.
		if u.CurrentPlan != nil {
			v.Message += fmt.Sprintf("Your package %s has ended. Extend it on the Account Page.", u.CurrentPlan.Name)
		} else {
			v.Message += "Your package has ended. Extend it on the Account Page."
		}

	case time.Until(*u.Subscription.EndsAt) < 7*24*time.Hour:
		// Add generic ending soon message if the package ends in less than 7 days.
		v.Message += "Your package ends soon. Extend it on the Account Page."
	}

	// Defaults for generally good accounts.
	v.ShowAccountData = true
	v.ShowAccountButton = true
	v.ShowRefreshButton = true
	v.ShowLogoutButton = true
}
