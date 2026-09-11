package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

/*
 * GDK-1429: flowFields' store answer is memoized on the sync version, so a
 * mirror that has not moved does not pay the cycle-percentile walk again.
 *
 *   TestFlowMemoHoldsAcrossBootstrapAndDelta — the handler wiring
 *        ① one bootstrap + two deltas on an unchanged mirror → exactly one
 *           store call; all three responses carry the same flow block
 *   TestFlowMemoInvalidatesOnSyncVersion — the key
 *        ② same version twice → one call
 *        ③ a moved sync version → a second call (the answer may have moved)
 *   TestFlowMemoSurvivesThresholdSet — the live config gate
 *        ④ staleThresholdHours set after a memoized answer → absent, and
 *           the memo is not handed back when the threshold is cleared
 *           unless the version moved (the remembered answer is still the
 *           mirror's truth, not the config's)
 *   TestFlowMemoErrorNotCached — the error path
 *        ⑤ a failing store call is retried on the next request, not
 *           memoized into a permanent nil
 *
 * The counter rides s.cycleP85 — the same injection seat syncKick's tests
 * use — because the store call itself is what the issue is about: no
 * recompute on an unchanged mirror, not "the handler answers quickly".
 */

func pinCycleP85(t *testing.T, h *Handler, calls *int32, p85 float64, samples int, err error) {
	t.Helper()
	h.s.cycleP85 = func(context.Context, time.Time) (float64, int, error) {
		atomic.AddInt32(calls, 1)
		return p85, samples, err
	}
}

func TestFlowMemoHoldsAcrossBootstrapAndDelta(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	var calls int32
	pinCycleP85(t, h, &calls, 42.5, 60, nil)

	want := &flowOut{CycleP85Hours: 42.5, Samples: 60}
	for i, tt := range []struct {
		path string
	}{
		{apiBase + "bootstrap/"},
		{apiBase + "delta/?since=2026-07-15T00:00:00.000Z"},
		{apiBase + "delta/?since=2026-08-01T00:00:00.000Z"},
	} {
		rec := get(t, h, tt.path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, rec.Code)
		}
		var body struct {
			Flow *flowOut `json:"flow"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("request %d decode: %v", i, err)
		}
		if body.Flow == nil || *body.Flow != *want {
			t.Fatalf("request %d flow = %+v, want %+v", i, body.Flow, want)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("cycle p85 computed %d times for a bootstrap + two deltas on an unchanged mirror, want 1", n)
	}
}

func TestFlowMemoInvalidatesOnSyncVersion(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	var calls int32
	pinCycleP85(t, h, &calls, 10, 60, nil)

	st, err := db.SyncState(context.Background(), sourceID)
	if err != nil {
		t.Fatalf("sync state: %v", err)
	}
	if a := h.s.flowFields(context.Background(), st.Version); a == nil || a.CycleP85Hours != 10 {
		t.Fatalf("first call: %+v", a)
	}
	if b := h.s.flowFields(context.Background(), st.Version); b == nil || b.CycleP85Hours != 10 {
		t.Fatalf("second call: %+v", b)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("same version twice computed %d times, want 1", n)
	}
	if c := h.s.flowFields(context.Background(), st.Version+1); c == nil || c.CycleP85Hours != 10 {
		t.Fatalf("moved version: %+v", c)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("a moved sync version must recompute: %d calls, want 2", n)
	}
}

func TestFlowMemoSurvivesThresholdSet(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	var calls int32
	pinCycleP85(t, h, &calls, 10, 60, nil)

	st, err := db.SyncState(context.Background(), sourceID)
	if err != nil {
		t.Fatalf("sync state: %v", err)
	}
	if a := h.s.flowFields(context.Background(), st.Version); a == nil {
		t.Fatalf("first call: flow absent before any threshold was set")
	}

	// A settings PUT replaces the config live; the threshold gate must not
	// be masked by the remembered answer.
	set := *cfg
	set.StaleThresholdHours = 48
	h.s.cfg.Store(&set)
	if b := h.s.flowFields(context.Background(), st.Version); b != nil {
		t.Fatalf("explicit threshold masked by memo: %+v", b)
	}
	h.s.cfg.Store(cfg)
	if c := h.s.flowFields(context.Background(), st.Version); c == nil || c.CycleP85Hours != 10 {
		t.Fatalf("threshold cleared: %+v", c)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("threshold round trip recomputed %d times, want 1 (the mirror never moved)", n)
	}
}

func TestFlowMemoErrorNotCached(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	var calls int32
	pinCycleP85(t, h, &calls, 0, 0, context.DeadlineExceeded)

	st, err := db.SyncState(context.Background(), sourceID)
	if err != nil {
		t.Fatalf("sync state: %v", err)
	}
	if a := h.s.flowFields(context.Background(), st.Version); a != nil {
		t.Fatalf("failing store call produced a flow block: %+v", a)
	}
	// The next request must ask the store again, not replay the failure.
	h.s.cycleP85 = func(context.Context, time.Time) (float64, int, error) {
		atomic.AddInt32(&calls, 1)
		return 10, 60, nil
	}
	if b := h.s.flowFields(context.Background(), st.Version); b == nil || b.CycleP85Hours != 10 {
		t.Fatalf("error was memoized: %+v", b)
	}
}
