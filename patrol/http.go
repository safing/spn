package patrol

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/tevino/abool"

	"github.com/safing/portbase/log"
	"github.com/safing/portbase/modules"
)

var httpsConnectivityConfirmed = abool.NewBool(true)

// HTTPSConnectivityConfirmed returns whether the last HTTPS connectivity check succeeded.
// Is "true" before first test.
func HTTPSConnectivityConfirmed() bool {
	return httpsConnectivityConfirmed.IsSet()
}

// CheckHTTPSConnection checks if a HTTPS connection to the given domain can be established.
func CheckHTTPSConnection(ctx context.Context, domain string) (statusCode int, err error) {
	// Build URL.
	// Use HTTPS to ensure that we have really communicated with the desired
	// server and not with an intermediate.
	url := fmt.Sprintf("https://%s/", domain)

	// Prepare all parts of the request.
	// TODO: Evaluate if we want to change the User-Agent.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			DisableKeepAlives:   true,
			DisableCompression:  true,
			TLSHandshakeTimeout: 5 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	// Make request to server.
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send http request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("unexpected status code: %s", resp.Status)
	}

	return resp.StatusCode, nil
}

func httpsConnectivityCheck(ctx context.Context, task *modules.Task) error {
	// Start tracing logs.
	ctx, tracer := log.AddTracer(ctx)
	defer tracer.Submit()

	// Run checks and report status.
	success := runHTTPSConnectivityChecks(ctx)
	if success {
		tracer.Info("spn/patrol: https connectivity check succeeded")
		if httpsConnectivityConfirmed.SetToIf(false, true) {
			module.TriggerEvent(ChangeSignalEventName, nil)
		}
		return nil
	}

	tracer.Errorf("spn/patrol: https connectivity check failed")
	if httpsConnectivityConfirmed.SetToIf(true, false) {
		module.TriggerEvent(ChangeSignalEventName, nil)
	}
	return nil
}

func runHTTPSConnectivityChecks(ctx context.Context) (ok bool) {
	// Step 1: Check 1 domain, require 100%
	if checkHTTPSConnectivity(ctx, 1, 1) {
		return true
	}

	// Step 2: Check 5 domains, require 80%
	if checkHTTPSConnectivity(ctx, 5, 0.8) {
		return true
	}

	// Step 3: Check 20 domains, require 90%
	if checkHTTPSConnectivity(ctx, 20, 0.9) {
		return true
	}

	return false
}

func checkHTTPSConnectivity(ctx context.Context, checks int, requiredSuccessFraction float32) (ok bool) {
	log.Tracer(ctx).Tracef(
		"spn/patrol: testing connectivity via https (%d checks; %.0f%% required)",
		checks,
		requiredSuccessFraction*100,
	)

	// Run tests.
	var succeeded int
	for i := 0; i < checks; i++ {
		if checkHTTPSConnection(ctx) {
			succeeded++
		}
	}

	// Check success.
	successFraction := float32(succeeded) / float32(checks)
	if successFraction < requiredSuccessFraction {
		log.Tracer(ctx).Warningf(
			"spn/patrol: https connectivity check failed: %d/%d (%.0f%%)",
			succeeded,
			checks,
			successFraction*100,
		)
		return false
	}

	log.Tracer(ctx).Debugf(
		"spn/patrol: https connectivity check succeeded: %d/%d (%.0f%%)",
		succeeded,
		checks,
		successFraction*100,
	)
	return true
}

func checkHTTPSConnection(ctx context.Context) (ok bool) {
	testDomain := getRandomTestDomain()
	code, err := CheckHTTPSConnection(ctx, testDomain)
	if err != nil {
		log.Tracer(ctx).Debugf("spn/patrol: https connect check failed: %s: %s", testDomain, err)
		return false
	}

	log.Tracer(ctx).Tracef("spn/patrol: https connect check succeeded: %s [%d]", testDomain, code)
	return true
}
