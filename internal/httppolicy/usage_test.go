package httppolicy

import (
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
	m.NoteRequest()
	m.NoteRetry()
	m.NoteWait(time.Millisecond)
	m.NoteStatus(429)
}

func TestMeterTakeZerosCountersKeepsLastThrottledAt(t *testing.T) {
	var m Meter
	m.NoteRequest()
	m.NoteStatus(429)
	m.NoteStatus(500)
	m.NoteRetry()
	m.NoteWait(3 * time.Millisecond)

	snap := m.Snapshot()
	if snap.Requests != 1 || snap.Throttled != 1 || snap.ServerErrors != 1 || snap.Retries != 1 {
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
	m.NoteWait(0)
	m.NoteWait(-time.Second)
	if got := m.Snapshot().WaitMS; got != 0 {
		t.Errorf("WaitMS = %d, want 0", got)
	}
}

func TestMeterNoteStatusClassification(t *testing.T) {
	var m Meter
	m.NoteStatus(429)
	m.NoteStatus(500)
	m.NoteStatus(503)
	m.NoteStatus(200)
	m.NoteStatus(404)
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
				m.NoteRequest()
			}
		}()
	}
	wg.Wait()
	if got := m.Snapshot().Requests; got != n*per {
		t.Errorf("Requests = %d, want %d", got, n*per)
	}
}
