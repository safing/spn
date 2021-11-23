package access

import (
	"context"
	"os"
	"testing"
)

var (
	testUsername = os.Getenv("SPN_TEST_USERNAME")
	testPassword = os.Getenv("SPN_TEST_PASSWORD")
)

func TestClient(t *testing.T) {
	if testUsername == "" || testPassword == "" {
		t.Fatal("test username or password not configured")
	}

	loginAndRefresh(t, true, 5)
	clearUserCaches()
	loginAndRefresh(t, false, 1)

	err := logout(false)
	if err != nil {
		t.Fatalf("failed to log out: %s", err)
	}
	t.Logf("logged out")

	loginAndRefresh(t, true, 1)

	err = logout(true)
	if err != nil {
		t.Fatalf("failed to log out: %s", err)
	}
	t.Logf("logged out with purge")

	loginAndRefresh(t, true, 1)
}

func loginAndRefresh(t *testing.T, doLogin bool, refreshTimes int) {
	if doLogin {
		_, _, err := login(context.Background(), testUsername, testPassword)
		if err != nil {
			t.Fatalf("login failed: %s", err)
		}
		user, err := GetUser()
		if err != nil {
			t.Fatalf("failed to get user: %s", err)
		}
		t.Logf("user (from login): %+v", user.User)
		t.Logf("device (from login): %+v", user.User.Device)
		authToken, err := GetAuthToken()
		if err != nil {
			t.Fatalf("failed to get auth token: %s", err)
		}
		t.Logf("auth token: %+v", authToken.Token)
	}

	for i := 0; i < refreshTimes; i++ {
		user, _, err := getUserProfile(context.Background())
		if err != nil {
			t.Fatalf("getting profile failed: %s", err)
		}
		t.Logf("user (from refresh): %+v", user.User)

		authToken, err := GetAuthToken()
		if err != nil {
			t.Fatalf("failed to get auth token: %s", err)
		}
		t.Logf("auth token: %+v", authToken.Token)
	}
}
