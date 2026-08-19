package httppolicy

import (
	"sync/atomic"
	"time"
)

// Usage is a point-in-time snapshot of outbound HTTP traffic for one
// origin client. Counters are process-local until a caller persists them
// (see store.api_usage).
//
// Requests counts every HTTP attempt, including retries: that is the unit
// that draws from the site's rate budget. This is our own call volume, not
// the remaining point pool — the site does not expose that.
//
// Jira, Confluence, and Linear all report this type so an operator can
// compare "linear throttled us" against "Jira throttled us" field by field
// (Snapshot / Take / Client.Usage / Client.TakeUsage). LastThrottledAt is
// the shared name; WaitMS is milliseconds actually spent in Wait.
type Usage struct {
	Requests        int64
	Throttled       int64     // 429 responses
	ServerErrors    int64     // 5xx responses, excluding 429
	Retries         int64     // attempts re-sent after a wait
	WaitMS          int64     // milliseconds actually spent in Wait
	LastThrottledAt time.Time // UTC; zero if never throttled
}

// Meter holds atomic counters shared by concurrent request goroutines.
// A Client is used from up to 4 concurrent sync workers (contracts/sync.md).
type Meter struct {
	requests            atomic.Int64
	throttled           atomic.Int64
	serverErrors        atomic.Int64
	retries             atomic.Int64
	waitMS              atomic.Int64
	lastThrottledUnixNs atomic.Int64 // UTC unix nanoseconds; 0 if never set
}

// Snapshot returns the current counters without resetting them.
// Same shape as store.DB.PoolStats / WriteBusyRetries: a cheap accessor, no logs.
func (m *Meter) Snapshot() Usage {
	if m == nil {
		return Usage{}
	}
	return Usage{
		Requests:        m.requests.Load(),
		Throttled:       m.throttled.Load(),
		ServerErrors:    m.serverErrors.Load(),
		Retries:         m.retries.Load(),
		WaitMS:          m.waitMS.Load(),
		LastThrottledAt: unixNsToTime(m.lastThrottledUnixNs.Load()),
	}
}

// Take returns the current counters and zeroes the numeric fields so a
// flusher can accumulate into daily totals without double-counting.
//
// LastThrottledAt is a timestamp, not a counter: it is included in the
// snapshot but is NOT cleared. The in-process "last 429" stays visible until
// the process exits or a later 429 overwrites it.
func (m *Meter) Take() Usage {
	if m == nil {
		return Usage{}
	}
	return Usage{
		Requests:        m.requests.Swap(0),
		Throttled:       m.throttled.Swap(0),
		ServerErrors:    m.serverErrors.Swap(0),
		Retries:         m.retries.Swap(0),
		WaitMS:          m.waitMS.Swap(0),
		LastThrottledAt: unixNsToTime(m.lastThrottledUnixNs.Load()), // not reset
	}
}

// NoteRequest records one HTTP attempt that left the process.
func (m *Meter) NoteRequest() {
	if m == nil {
		return
	}
	m.requests.Add(1)
}

// NoteRetry records an attempt re-sent after a wait.
func (m *Meter) NoteRetry() {
	if m == nil {
		return
	}
	m.retries.Add(1)
}

// NoteWait records time actually spent in Wait. Non-positive durations are ignored.
func (m *Meter) NoteWait(d time.Duration) {
	if m == nil || d <= 0 {
		return
	}
	m.waitMS.Add(d.Milliseconds())
}

// NoteStatus classifies a completed HTTP status into Throttled (429) or
// ServerErrors (other 5xx).
func (m *Meter) NoteStatus(code int) {
	if m == nil {
		return
	}
	switch {
	case code == 429:
		m.throttled.Add(1)
		m.lastThrottledUnixNs.Store(time.Now().UTC().UnixNano())
	case code >= 500 && code <= 599:
		m.serverErrors.Add(1)
	}
}

func unixNsToTime(ns int64) time.Time {
	if ns <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}
