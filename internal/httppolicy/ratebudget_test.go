package httppolicy

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// GDK-1646. These tests fix the header contract (parse, reset, sleep
// choice, optimistic refill); the live FAIL-first — a real 429 from a Data
// Center with a low limit proving the sleeps land on the wire — is the
// lead's, measured against the lab instance. On the unmodified tree these
// tests do not compile (RateBudget did not exist), which is their FAIL:
// there was no proactive budget at all.

// recorder Sleep: no real waiting, every requested duration is kept.
func recorder(slept *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		*slept = append(*slept, d)
		return nil
	}
}

func header(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

func dcHeaders(remaining, limit, interval, fill, retryAfter string) http.Header {
	return header(
		"X-RateLimit-Limit", limit,
		"X-RateLimit-Remaining", remaining,
		"X-RateLimit-Interval-Seconds", interval,
		"X-RateLimit-FillRate", fill,
		"Retry-After", retryAfter,
	)
}

func TestObserveParsesTheFiveHeaders(t *testing.T) {
	b := &RateBudget{}
	b.Observe(dcHeaders("4", "100", "7", "10", "0"))
	if !b.seen {
		t.Fatal("seen = false after a full DC budget")
	}
	for _, f := range []struct {
		name string
		got  int
		want int
	}{
		{"remaining", b.remaining, 4},
		{"limit", b.limit, 100},
		{"intervalSeconds", b.intervalSeconds, 7},
		{"fillRate", b.fillRate, 10},
		{"retryAfter", b.retryAfter, 0},
	} {
		if f.got != f.want {
			t.Errorf("%s = %d, want %d", f.name, f.got, f.want)
		}
	}
}

func TestObserveKeepsPreviousValueOnAbsentOrGarbage(t *testing.T) {
	b := &RateBudget{}
	b.Observe(dcHeaders("4", "100", "7", "10", "3"))
	// Garbage leaves the previous value; absent leaves it too.
	b.Observe(header(
		"X-RateLimit-Limit", "not-a-number",
		"X-RateLimit-Remaining", "soon",
		"Retry-After", "later",
	))
	if b.limit != 100 || b.remaining != 4 || b.retryAfter != 3 {
		t.Errorf("garbage overwrote the budget: limit=%d remaining=%d retryAfter=%d", b.limit, b.remaining, b.retryAfter)
	}
	// A partial statement updates what it states and keeps the rest.
	b.Observe(header("X-RateLimit-Remaining", "2"))
	if b.remaining != 2 || b.limit != 100 || b.intervalSeconds != 7 {
		t.Errorf("partial statement lost the rest: %+v", b)
	}
}

func TestObserveWithoutRateLimitHeadersResetsSeen(t *testing.T) {
	b := &RateBudget{}
	b.Observe(dcHeaders("0", "100", "7", "10", "3"))
	var slept []time.Duration
	b.Sleep = recorder(&slept)
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 {
		t.Fatalf("the red budget must sleep, slept %v", slept)
	}
	// A response from somewhere other than the limiter (no X-RateLimit-*
	// headers at all — retry-after alone does not count) hides the budget
	// until the origin states one again.
	b.Observe(header("Retry-After", "9"))
	if b.seen {
		t.Error("retry-after alone must not count as a stated budget")
	}
	slept = nil
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 0 {
		t.Errorf("Wait slept %v after the budget went unseen", slept)
	}
}

func TestWaitIsANoOpUntilSeen(t *testing.T) {
	b := &RateBudget{}
	var slept []time.Duration
	b.Sleep = recorder(&slept)
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 0 {
		t.Errorf("an unobserved budget slept %v", slept)
	}
}

func TestWaitSleepsRetryAfterElseInterval(t *testing.T) {
	for _, tc := range []struct {
		name       string
		h          http.Header
		want       time.Duration
		wantSleeps int
	}{
		{"retry-after wins", dcHeaders("1", "100", "7", "10", "3"), 3 * time.Second, 1},
		{"interval when retry-after is zero", dcHeaders("1", "100", "7", "10", "0"), 7 * time.Second, 1},
		{"interval capped at MaxWait", dcHeaders("1", "100", "600", "10", "0"), MaxWait, 1},
	} {
		b := &RateBudget{}
		b.Observe(tc.h)
		var slept []time.Duration
		b.Sleep = recorder(&slept)
		if err := b.Wait(context.Background()); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(slept) != tc.wantSleeps || slept[0] != tc.want {
			t.Errorf("%s: slept %v, want one %s", tc.name, slept, tc.want)
		}
	}
}

func TestWaitHonoursReserveAndOptimisticRefill(t *testing.T) {
	// Reserve defaults to 2 when zero: 2 remaining is already red, 3 is not.
	b := &RateBudget{}
	b.Observe(dcHeaders("2", "100", "7", "10", "0"))
	var slept []time.Duration
	b.Sleep = recorder(&slept)
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 {
		t.Fatalf("remaining 2 with default Reserve 2 must sleep, slept %v", slept)
	}
	// The optimistic refill: after one sleep Remaining is Limit, so the
	// very next Wait passes without another sleep (the next Observe would
	// correct the guess).
	slept = nil
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 0 {
		t.Errorf("after the optimistic refill Wait slept %v", slept)
	}
	// An explicit Reserve moves the floor: 4 remaining with Reserve 4 is
	// red (5 would leave one in hand), and retry-after picks the sleep.
	c := &RateBudget{Reserve: 4}
	c.Observe(dcHeaders("4", "100", "7", "10", "2"))
	slept = nil
	c.Sleep = recorder(&slept)
	if err := c.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 || slept[0] != 2*time.Second {
		t.Errorf("Reserve 4: slept %v, want one 2s", slept)
	}
}

func TestWaitRecordsMeterNoteWait(t *testing.T) {
	b := &RateBudget{Meter: &Meter{}}
	b.Observe(dcHeaders("0", "100", "7", "10", "0"))
	// A Sleep that really spends 5ms, so NoteWait has actual time to record
	// — the same accounting policy.Wait gives a retry.
	b.Sleep = func(ctx context.Context, d time.Duration) error {
		time.Sleep(5 * time.Millisecond)
		return nil
	}
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := b.Meter.Snapshot().WaitMS; got <= 0 {
		t.Errorf("Meter.WaitMS = %d, want the time actually slept", got)
	}
}

func TestWaitPropagatesContextCancellation(t *testing.T) {
	b := &RateBudget{}
	b.Observe(dcHeaders("0", "100", "7", "10", "0"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.Sleep = sleepTimer // the real one, already cancelled ctx returns at once
	if err := b.Wait(ctx); err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	// A cancelled wait must not hand out the optimistic refill.
	var slept []time.Duration
	b.Sleep = recorder(&slept)
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 {
		t.Errorf("after a cancelled wait the budget is still red, slept %v", slept)
	}
}
