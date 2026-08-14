package atlhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestMeterNilSnapshotAndTake(t *testing.T) {
	var m *Meter
	if got := m.Snapshot(); got != (Usage{}) {
		t.Errorf("nil Snapshot = %+v", got)
	}
	if got := m.Take(); got != (Usage{}) {
		t.Errorf("nil Take = %+v", got)
	}
	// note* must not panic
	m.noteRequest()
	m.noteThrottled()
	m.noteServerError()
	m.noteRetry()
	m.noteWait(time.Millisecond)
	m.noteStatus(429)
}

func TestMeterTakeZerosCountersKeepsLastThrottledAt(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	t.Cleanup(srv.Close)
	m := &Meter{}
	cfg := Config{
		Base: srv.URL, Auth: testAuth, HTTP: srv.Client(),
		Retries: 2, Backoff: time.Millisecond, Usage: m,
	}
	_, _, err := DoRaw(context.Background(), cfg, http.MethodGet, "/x", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}

	snap := m.Snapshot()
	if snap.Requests != 2 || snap.Throttled != 1 || snap.ServerErrors != 1 || snap.Retries != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if snap.WaitMS <= 0 {
		t.Errorf("WaitMS = %d, want > 0", snap.WaitMS)
	}
	if snap.LastThrottledAt.IsZero() || snap.LastThrottledAt.Location() != time.UTC {
		t.Errorf("LastThrottledAt = %v, want non-zero UTC", snap.LastThrottledAt)
	}

	taken := m.Take()
	if taken.Requests != snap.Requests || taken.Throttled != snap.Throttled ||
		taken.ServerErrors != snap.ServerErrors || taken.Retries != snap.Retries ||
		taken.WaitMS != snap.WaitMS {
		t.Errorf("Take = %+v, want %+v", taken, snap)
	}
	if !taken.LastThrottledAt.Equal(snap.LastThrottledAt) {
		t.Errorf("Take moved LastThrottledAt")
	}
	after := m.Snapshot()
	if after.Requests != 0 || after.Throttled != 0 || after.ServerErrors != 0 || after.Retries != 0 || after.WaitMS != 0 {
		t.Errorf("counters not zeroed: %+v", after)
	}
	if !after.LastThrottledAt.Equal(snap.LastThrottledAt) {
		t.Errorf("LastThrottledAt must survive Take: %v vs %v", after.LastThrottledAt, snap.LastThrottledAt)
	}
}

func TestMeterNoteWaitNonPositive(t *testing.T) {
	var m Meter
	m.noteWait(0)
	m.noteWait(-time.Second)
	if got := m.Snapshot().WaitMS; got != 0 {
		t.Errorf("WaitMS = %d, want 0", got)
	}
}

func TestMeterNoteStatusClassification(t *testing.T) {
	var m Meter
	m.noteStatus(429)
	m.noteStatus(500)
	m.noteStatus(503)
	m.noteStatus(200)
	m.noteStatus(404)
	u := m.Snapshot()
	if u.Throttled != 1 {
		t.Errorf("Throttled = %d, want 1", u.Throttled)
	}
	if u.ServerErrors != 2 {
		t.Errorf("ServerErrors = %d, want 2 (5xx excluding 429)", u.ServerErrors)
	}
}

func TestMeterConcurrentIncrements(t *testing.T) {
	var m Meter
	var wg sync.WaitGroup
	const n, per = 8, 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < per; j++ {
				m.noteRequest()
			}
		}()
	}
	wg.Wait()
	if got := m.Snapshot().Requests; got != n*per {
		t.Errorf("Requests = %d, want %d", got, n*per)
	}
}
