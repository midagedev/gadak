package main

import (
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/httppolicy"
)

// TestGateNamedRetryableStatusesMatchPolicy fails if httppolicy.IsRetryable
// grows a code this scanner cannot resolve from an http.Status* selector.
func TestGateNamedRetryableStatusesMatchPolicy(t *testing.T) {
	known := map[int]string{}
	for name, code := range retryableHTTPStatusName {
		if !httppolicy.IsRetryable(code) {
			t.Errorf("retryableHTTPStatusName[%q]=%d is not httppolicy.IsRetryable", name, code)
		}
		known[code] = name
	}
	for code := 100; code < 600; code++ {
		if httppolicy.IsRetryable(code) && known[code] == "" {
			t.Errorf("httppolicy.IsRetryable(%d) has no http.Status* name in retryableHTTPStatusName", code)
		}
	}
}

// retryableHTTPStatusName is the net/http identifiers for the codes
// httppolicy.IsRetryable currently accepts. Values come from net/http, not a
// hand-built table; TestGateNamedRetryableStatusesMatchPolicy locks the set
// to the policy.
//
// The repo-wide walk that consumes this map (stubs must not answer a
// retryable status through the LinearEndpoint seam) is a source lint and
// lives behind the sourcelint tag in linear_endpoint_stub_walk_test.go —
// run it with `bash tools/sourcelint.sh` (GDK-1144).
var retryableHTTPStatusName = map[string]int{
	"StatusTooManyRequests":     http.StatusTooManyRequests,
	"StatusInternalServerError": http.StatusInternalServerError,
	"StatusBadGateway":          http.StatusBadGateway,
	"StatusServiceUnavailable":  http.StatusServiceUnavailable,
	"StatusGatewayTimeout":      http.StatusGatewayTimeout,
}
