package httppolicy

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateBudget throttles proactively from Jira Data Center's rate-limit
// headers (GDK-1646). With rate limiting enabled, DC states its token-bucket
// budget on every authenticated response —
//
//	X-RateLimit-Limit             bucket size
//	X-RateLimit-Remaining         tokens left
//	X-RateLimit-Interval-Seconds  refill interval
//	X-RateLimit-FillRate          tokens added per interval
//	retry-after                   seconds until a token exists (0 when some
//	                               remain)
//
// — so the wall is visible before it is hit, and a large first sync firing
// requests back to back can be spaced without ever eating a 429. Wait's
// reactive half stays where it was: a 429's Retry-After is honoured by Wait
// (policy.go) regardless of this type.
//
// Cloud publishes none of these headers and the built-in tracker never
// rate-limits, so only jira.NewServer wires one in; a nil *RateBudget is
// disabled everywhere and the Cloud request path is unchanged.
type RateBudget struct {
	// Sleep suspends the caller for d. Nil uses a real timer; tests inject
	// a recorder so a throttled path is asserted without waiting.
	Sleep func(context.Context, time.Duration) error
	// Reserve is the token floor kept in hand: Wait sleeps while the origin
	// reports no more than Reserve tokens left, so the bucket is never run
	// to zero by us. Zero means the default, 2.
	Reserve int
	// Meter, when non-nil, records the time actually spent sleeping
	// (NoteWait), the same accounting Wait gives a retry.
	Meter *Meter

	mu              sync.Mutex
	seen            bool // any X-RateLimit-* header observed so far
	remaining       int
	limit           int
	intervalSeconds int
	fillRate        int // parsed for the contract; Wait never needs it
	retryAfter      int
}

// defaultReserve is the in-hand floor when Reserve is zero. Two buys one
// spare token for a concurrent worker's in-flight request.
const defaultReserve = 2

// Observe records the budget a response stated. Each header is parsed alone
// (Atoi): an absent or garbage value leaves the previous one, so a response
// that states only part of the budget keeps the rest. A response carrying
// none of the X-RateLimit headers is not from the limiter at all (an error
// page, a redirect) and resets the budget to unseen — Wait goes quiet until
// the origin states a budget again.
func (b *RateBudget) Observe(h http.Header) {
	if b == nil || h == nil {
		return
	}
	take := func(name string) (present, parsed bool, v int) {
		s := strings.TrimSpace(h.Get(name))
		if s == "" {
			return false, false, 0
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return true, false, 0
		}
		return true, true, n
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	stated := false
	if p, ok, v := take("X-RateLimit-Limit"); p {
		stated = true
		if ok {
			b.limit = v
		}
	}
	if p, ok, v := take("X-RateLimit-Remaining"); p {
		stated = true
		if ok {
			b.remaining = v
		}
	}
	if p, ok, v := take("X-RateLimit-Interval-Seconds"); p {
		stated = true
		if ok {
			b.intervalSeconds = v
		}
	}
	if p, ok, v := take("X-RateLimit-FillRate"); p {
		stated = true
		if ok {
			b.fillRate = v
		}
	}
	if _, ok, v := take("Retry-After"); ok {
		b.retryAfter = v
	}
	b.seen = stated
}

// Wait blocks until the origin's stated budget has room for one more
// request. It is a no-op until a budget has been observed, and returns nil
// straight away while more than Reserve tokens remain. In the red it sleeps
// retry-after seconds when the origin stated a positive one, else one refill
// interval (capped at MaxWait), then optimistically resets Remaining to
// Limit — the next Observe corrects whatever the sleep actually refilled.
func (b *RateBudget) Wait(ctx context.Context) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if !b.seen {
		b.mu.Unlock()
		return nil
	}
	reserve := b.Reserve
	if reserve == 0 {
		reserve = defaultReserve
	}
	if b.remaining > reserve {
		b.mu.Unlock()
		return nil
	}
	retryAfter, interval, limit := b.retryAfter, b.intervalSeconds, b.limit
	b.mu.Unlock()

	d := time.Duration(interval) * time.Second
	if retryAfter > 0 {
		d = time.Duration(retryAfter) * time.Second
	}
	if d > MaxWait {
		d = MaxWait
	}
	if d <= 0 {
		// No refill clock was ever stated, so there is nothing to wait for
		// and nothing learned that would justify the optimistic reset; the
		// next Observe is the earliest correction.
		return nil
	}
	sleep := b.Sleep
	if sleep == nil {
		sleep = sleepTimer
	}
	start := time.Now()
	if err := sleep(ctx, d); err != nil {
		b.Meter.NoteWait(time.Since(start))
		return err
	}
	b.Meter.NoteWait(time.Since(start))
	b.mu.Lock()
	b.remaining = limit
	b.mu.Unlock()
	return nil
}

// sleepTimer is the production Sleep: one timer, cancellable by context.
func sleepTimer(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
