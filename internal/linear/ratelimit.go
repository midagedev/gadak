package linear

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimit is the budget Linear states in response headers after every call
// (captured 2026-08-18 on a personal API key):
//
//	x-complexity: 1
//	x-ratelimit-complexity-limit: 3000000
//	x-ratelimit-complexity-remaining: 2999999
//	x-ratelimit-complexity-reset: 1787064041508
//	x-ratelimit-requests-limit: 2500
//	x-ratelimit-requests-remaining: 2499
//	x-ratelimit-requests-reset: 1787064041508
//
// Two axes: a request count (2500 per window) and a query-complexity sum
// (3,000,000 per window), both resetting at the same epoch-millisecond stamp.
// This is server-claimed state, unlike Usage, which counts what this process
// sent — a connector that only watches one of them can be surprised by the
// other.
type RateLimit struct {
	// Complexity of the most recent query (x-complexity).
	Complexity int64 `json:"complexity"`
	// ComplexityLimit / ComplexityRemaining are the window's budget and what
	// is left of it; ComplexityResetMS is the epoch-ms reset instant.
	ComplexityLimit     int64 `json:"complexity_limit"`
	ComplexityRemaining int64 `json:"complexity_remaining"`
	ComplexityResetMS   int64 `json:"complexity_reset_ms"`
	// RequestsLimit / RequestsRemaining are the request-count window;
	// RequestsResetMS is the epoch-ms reset instant.
	RequestsLimit     int64 `json:"requests_limit"`
	RequestsRemaining int64 `json:"requests_remaining"`
	RequestsResetMS   int64 `json:"requests_reset_ms"`
	// ObservedAt is when these values were read (UTC).
	ObservedAt time.Time `json:"observed_at"`
}

// LastRateLimit returns the headers of the most recent response, or the zero
// RateLimit before the first call. It is a snapshot for diagnostics — sync
// status output, `gadak` health — never a gate: a missing header must not
// fail a request that succeeded.
func (c *Client) LastRateLimit() RateLimit {
	if rl := c.rate.Load(); rl != nil {
		return *rl
	}
	return RateLimit{}
}

// noteRateLimit parses the x-ratelimit-* headers of one response. Unparsable
// or absent headers leave fields at zero; parsing never fails the request.
func (c *Client) noteRateLimit(h http.Header) {
	rl := RateLimit{
		Complexity:          parseHeaderInt(h, "X-Complexity"),
		ComplexityLimit:     parseHeaderInt(h, "X-RateLimit-Complexity-Limit"),
		ComplexityRemaining: parseHeaderInt(h, "X-RateLimit-Complexity-Remaining"),
		ComplexityResetMS:   parseHeaderInt(h, "X-RateLimit-Complexity-Reset"),
		RequestsLimit:       parseHeaderInt(h, "X-RateLimit-Requests-Limit"),
		RequestsRemaining:   parseHeaderInt(h, "X-RateLimit-Requests-Remaining"),
		RequestsResetMS:     parseHeaderInt(h, "X-RateLimit-Requests-Reset"),
		ObservedAt:          time.Now().UTC(),
	}
	c.rate.Store(&rl)
}

func parseHeaderInt(h http.Header, name string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(h.Get(name)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
