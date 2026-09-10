package sync

import (
	"context"
	"sync"
)

// The Confluence fetch pool (GDK-1673). One pass used to walk its search hits
// strictly serially: body, comments, and version stamps for page N+1 waited
// behind every request for page N. On a hosted origin each round trip is a
// network hop, so a 500-page backfill is ~1500 sequential hops — minutes of
// wall clock the origin spends idle. The pool fans the per-item GETs out over
// a bounded worker set while the store writes stay serial and in listing
// order: fetches are independent (read-only GETs), commits are not (batched
// upserts, watermarks that advance only after a committed batch). That split
// is the whole design — the audit shape is "fetch pool → results channel →
// the existing single writer".

const (
	// DefaultFetchConcurrency is the fetch pool's default width: enough to
	// cover a round trip's latency, few enough that a small origin is not
	// stamped on. It is also the ceiling's midpoint, not a tuned number —
	// the honest tuning pass needs the lead's after-merge benchmark.
	DefaultFetchConcurrency = 4
	// MaxFetchConcurrency is the ceiling `--concurrency` is clamped to.
	// Above this the pass stops looking like one client to an operator
	// watching the origin's rate dashboard.
	MaxFetchConcurrency = 8
	// throttleGrowAfter is how many consecutive clean fetch windows AIMD
	// needs before growing the effective width by one (additive-increase).
	// Clean means "no 429 anywhere in the window", so growth is slow on
	// purpose: a pass that halves twice needs 40 clean windows to climb back.
	throttleGrowAfter = 20
)

// FetchConcurrency is the configured width of the fetch pools — the
// Confluence pass (GDK-1673) and the Jira issue pass (GDK-1674) — set by
// `gadak sync --concurrency` and read once per pass. A package-level knob
// rather than an Options field because the CLI and the server share the pass
// entry points. The pass clamps it to [1, MaxFetchConcurrency]; 1 is exactly
// the serial pass of pre-1673, pause included.
var FetchConcurrency = DefaultFetchConcurrency

// clampFetchWidth bounds a configured width to the pool's legal range.
func clampFetchWidth(n int) int {
	if n < 1 {
		return 1
	}
	if n > MaxFetchConcurrency {
		return MaxFetchConcurrency
	}
	return n
}

// Throttle is the pool's AIMD width controller: how many fetches may be in
// flight right now, and how that number moves in response to the origin's
// 429s. One instance lives for a whole pass, so a halving earned in chunk 1
// still holds in chunk 2. It carries no Confluence types — the Jira pool
// (GDK-1674) reuses it as is.
//
// Multiplicative-decrease on a 429: effective = max(1, effective/2). A 429
// is observed by the transport layer per response, but this controller hears
// about it per fetch window (meter delta around one worker's item), so two
// overlapping windows can each report the same throttle and halve twice.
// That over-reaction is accepted: the floor is 1, growth is slow by design,
// and the alternative — serializing window reports — costs more than an
// occasional extra halving on an origin that is already refusing work.
//
// Additive-increase after throttleGrowAfter consecutive clean windows:
// effective+1, back toward the configured width, never past it.
//
// Admission is a counting semaphore (a buffered channel of tokens) plus a
// debt count: a shrink that arrives while every token is in flight takes the
// tokens back as their workers release them. Every operation except Acquire
// is non-blocking, so a cancelled pass can never leave the controller
// wedged — Acquire is the only blocking point and it selects on ctx.
type Throttle struct {
	configured int

	mu sync.Mutex
	// effective is how many tokens may circulate right now.
	effective int
	// minSeen is the lowest effective this pass sank to — the summary line's
	// "how bad did it get" number.
	minSeen int
	// pending is shrink debt: tokens the controller is owed from in-flight
	// workers, taken as they Release.
	pending int
	// clean is the consecutive-clean-window streak since the last throttle.
	clean int

	// sem holds circulating tokens. Occupancy never exceeds configured:
	// tokens exist only here, held by a worker's Acquire, or owed as pending
	// debt, and their total is always exactly `effective` plus debt already
	// repaid — so the give-back send in Release can never block.
	sem chan struct{}
}

// newThrottle builds a controller for one pass. width is clamped here; the
// pass passes the already-clamped knob, tests pass raw values.
func newThrottle(width int) *Throttle {
	if width < 1 {
		width = 1
	}
	t := &Throttle{
		configured: width,
		effective:  width,
		minSeen:    width,
		sem:        make(chan struct{}, width),
	}
	for i := 0; i < width; i++ {
		t.sem <- struct{}{}
	}
	return t
}

// Acquire takes a fetch slot, blocking until one is free. It is the only
// blocking operation and honors ctx, so a cancelled pass drains instead of
// hanging on a shrunk pool.
func (t *Throttle) Acquire(ctx context.Context) error {
	select {
	case <-t.sem:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns a fetch slot. A slot the controller is still owed (shrink
// debt from NoteThrottle) stays with the controller — that is how a halving
// that arrived mid-flight takes effect as busy workers finish.
func (t *Throttle) Release() {
	t.mu.Lock()
	if t.pending > 0 {
		t.pending--
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	t.sem <- struct{}{}
}

// NoteThrottle records a 429 seen inside one worker's fetch window: halve
// the effective width, floor 1. Tokens beyond the new width are removed
// from the idle pool immediately and from in-flight workers as they release.
func (t *Throttle) NoteThrottle() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clean = 0
	halved := t.effective / 2
	if halved < 1 {
		halved = 1
	}
	if halved == t.effective {
		return
	}
	t.pending += t.effective - halved
	t.effective = halved
	if halved < t.minSeen {
		t.minSeen = halved
	}
	// Take back what is idle right now; the rest is Release debt above.
	for t.pending > 0 {
		select {
		case <-t.sem:
			t.pending--
		default:
			return
		}
	}
}

// NoteClean records a fetch window with no 429. After throttleGrowAfter
// consecutive clean windows the width grows by one toward configured.
func (t *Throttle) NoteClean() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clean++
	if t.clean < throttleGrowAfter || t.effective >= t.configured {
		return
	}
	t.clean = 0
	t.effective++
	// Cancel outstanding shrink debt first; only a debt-free pool mints the
	// token back.
	if t.pending > 0 {
		t.pending--
		return
	}
	select {
	case t.sem <- struct{}{}:
	default:
		// Unreachable: growing means circulating < configured = capacity.
	}
}

// Effective is the current admission width. Reading it from a worker while
// others fetch is racy by design — it is the width this instant, not a
// promise about the next.
func (t *Throttle) Effective() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.effective
}

// Configured is the pass's configured width (the --concurrency knob).
func (t *Throttle) Configured() int {
	return t.configured
}

// MinEffective is the lowest width AIMD sank to this pass. Equal to
// configured when the origin never throttled.
func (t *Throttle) MinEffective() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.minSeen
}

// fetchOrdered runs fetch over items on a bounded worker pool and hands each
// result to emit in item order — the pass's "fan out the reads, serialize
// the writes" in one place. Workers and width both come from thr; the CQL
// listing that produced items stays serial and is not this function's
// concern.
//
// Failure is errgroup semantics: the first fetch or emit error cancels the
// pool context and is returned once every worker has drained; later errors
// are dropped. emit runs only on the caller's goroutine, in item order, so
// an emit that appends to a batch needs no lock. In-flight requests never
// exceed the pool width, and the reorder buffer (results received out of
// order) never holds more than one record per worker — the in-flight record
// bound is width, comfortably inside the audit's width×2-batches ceiling.
func fetchOrdered[T any, R any](ctx context.Context, thr *Throttle, items []T,
	fetch func(context.Context, T) (R, error), emit func(int, R) error) error {
	if len(items) == 0 {
		return nil
	}
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		i   int
		r   R
		err error
	}
	// Unbuffered both ways: a task is handed to a worker only when one is
	// ready, and a result moves on the moment the collector can take it.
	tasks := make(chan int)
	results := make(chan result)

	var wg sync.WaitGroup
	// Dispatcher: streams indices, and stops early on cancellation so idle
	// workers see a closed channel instead of waiting out a dead pass.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range items {
			select {
			case tasks <- i:
			case <-pctx.Done():
				close(tasks)
				return
			}
		}
		close(tasks)
	}()
	// Workers: fixed goroutine count (configured width); the Throttle's
	// effective width is what actually admits fetches.
	for w := 0; w < thr.Configured(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				if err := thr.Acquire(pctx); err != nil {
					continue // pass is being torn down; the error arrives via results
				}
				r, err := fetch(pctx, items[i])
				thr.Release()
				results <- result{i: i, r: r, err: err}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collector (caller's goroutine): buffer out-of-order results, emit in
	// listing order. After a first error it keeps draining so no worker is
	// left blocked on an unbuffered send.
	var firstErr error
	pending := make(map[int]R, thr.Configured())
	next := 0
	for res := range results {
		if firstErr != nil {
			continue
		}
		if res.err != nil {
			firstErr = res.err
			cancel()
			continue
		}
		pending[res.i] = res.r
		for {
			r, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			if err := emit(next, r); err != nil {
				firstErr = err
				cancel()
				break
			}
			next++
		}
	}
	return firstErr
}
