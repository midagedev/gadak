// Package httppolicy is the host-neutral HTTP retry and usage policy shared
// by origin clients (Atlassian via atlhttp, Linear). It owns which statuses
// are retryable, backoff plus Retry-After, the error-body snippet, the
// response byte cap, and the Usage/Meter counters.
//
// It deliberately does not own Base/Auth/path joining (atlhttp's host pin:
// the Authorization header never leaves the configured site), Linear's
// bare-key Authorization header, Linear's x-ratelimit-* headers, or
// rejected-credential sentinels. ErrAuth stays per host family so Linear
// need not share atlhttp.ErrAuth (GDK-274).
package httppolicy

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// MaxBody is the response-body cap applied with io.LimitReader. Both
// atlhttp.DoRaw and linear.Client.gql used 64<<20; one constant keeps them
// from drifting.
const MaxBody int64 = 64 << 20

// MaxWait is the ceiling on exponential backoff. Retry-After seconds replace
// the backoff outright and are not themselves capped here — matching the
// previous atlhttp/linear wait() bodies.
const MaxWait = 30 * time.Second

// IsRetryable is the read-path retry set: throttle and transient server
// errors. 501 and 505 are answers, not transients. Writes use
// IsRetryableWrite — a 500 may mean the mutation applied.
func IsRetryable(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	}
	return false
}

// IsRetryableWrite is the mutating retry set: 429 and 503 only.
func IsRetryableWrite(code int) bool {
	return code == 429 || code == 503
}

// Wait sleeps for the retry delay: backoff<<attempt, capped at MaxWait,
// replaced by a positive Retry-After (integer seconds; Atoi). A non-nil
// meter records the time actually spent. Cancel stops the wait and is
// still recorded.
func Wait(ctx context.Context, backoff time.Duration, attempt int, retryAfter string, meter *Meter) error {
	d := backoff << attempt
	if d > MaxWait {
		d = MaxWait
	}
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s > 0 {
		d = time.Duration(s) * time.Second
	}
	if d <= 0 {
		return ctx.Err()
	}
	start := time.Now()
	select {
	case <-ctx.Done():
		meter.NoteWait(time.Since(start))
		return ctx.Err()
	case <-time.After(d):
		meter.NoteWait(time.Since(start))
		return nil
	}
}

// Snippet trims and truncates a response body for error messages.
func Snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
