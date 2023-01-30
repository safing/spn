package patrol

import (
	"context"
	"fmt"
	"sort"
	"testing"
)

var enableDomainTools = "no" // change to "yes" to enable

// TestCleanDomains checks, cleans and prints an improved domain list.
// Run with:
// go test -run ^TestCleanDomains$ github.com/safing/spn/patrol -ldflags "-X github.com/safing/spn/patrol.enableDomainTools=yes" -timeout 3h -v
// This is provided as a test for easier maintenance and ops.
func TestCleanDomains(t *testing.T) { //nolint:paralleltest
	if enableDomainTools != "yes" {
		t.Skip()
		return
	}

	// Go through all domains and check if they are reachable.
	goodDomains := make([]string, 0, len(testDomains))
	for _, domain := range testDomains {
		if domain == "addtoany.com" {
			break
		}

		// Check if domain is reachable.
		code, err := CheckHTTPSConnection(context.Background(), domain)
		if err != nil {
			t.Logf("FAIL: %s: %s", domain, err)
		} else {
			t.Logf("OK: %s [%d]", domain, code)
			goodDomains = append(goodDomains, domain)
			continue
		}

		// If failed, try again with a www. prefix
		wwwDomain := "www." + domain
		code, err = CheckHTTPSConnection(context.Background(), wwwDomain)
		if err != nil {
			t.Logf("FAIL: %s: %s", wwwDomain, err)
		} else {
			t.Logf("OK: %s [%d]", wwwDomain, code)
			goodDomains = append(goodDomains, wwwDomain)
		}

	}

	sort.Strings(goodDomains)
	fmt.Println("printing good domains:")
	for _, domain := range goodDomains {
		fmt.Printf("%q,\n", domain)
	}

	fmt.Println("IMPORTANT: do not forget to go through list and check if everything looks good")
}
