package jira

import (
	"sync/atomic"
	"time"
)

// Usage is a point-in-time snapshot of this client's outbound Jira traffic.
// Counters are process-local until a caller persists them (see store.api_usage).
//
// Requests counts every HTTP attempt, including retries: that is the unit that
// draws from Jira's rate budget. This is our own call volume, not Jira's
// remaining point pool — the site does not expose that.
type Usage struct {
	Requests        int64
	Throttled       int64     // 429 responses
	ServerErrors    int64     // 5xx responses, excluding 429
	Retries         int64     // attempts re-sent after a wait
	WaitMS          int64     // milliseconds actually spent in wait()
	LastThrottledAt time.Time // UTC; zero if never throttled
}

// usage holds atomic counters shared by concurrent call() goroutines.
// A Client is used from up to 4 concurrent sync workers (contracts/sync.md).
type usage struct {
	requests            atomic.Int64
	throttled           atomic.Int64
	serverErrors        atomic.Int64
	retries             atomic.Int64
	waitMS              atomic.Int64
	lastThrottledUnixNs atomic.Int64 // UTC unix nanoseconds; 0 if never set
}

// Usage returns the current counters without resetting them.
func (c *Client) Usage() Usage {
	if c == nil {
		return Usage{}
	}
	return c.usage.snapshot()
}

// TakeUsage returns the current counters and zeroes the numeric fields so a
// flusher can accumulate into daily totals without double-counting.
//
// LastThrottledAt is a timestamp, not a counter: it is included in the
// snapshot but is NOT cleared. The in-process "last 429" stays visible until
// the process exits or a later 429 overwrites it.
func (c *Client) TakeUsage() Usage {
	if c == nil {
		return Usage{}
	}
	return c.usage.take()
}

func (u *usage) snapshot() Usage {
	return Usage{
		Requests:        u.requests.Load(),
		Throttled:       u.throttled.Load(),
		ServerErrors:    u.serverErrors.Load(),
		Retries:         u.retries.Load(),
		WaitMS:          u.waitMS.Load(),
		LastThrottledAt: unixNsToTime(u.lastThrottledUnixNs.Load()),
	}
}

func (u *usage) take() Usage {
	s := Usage{
		Requests:        u.requests.Swap(0),
		Throttled:       u.throttled.Swap(0),
		ServerErrors:    u.serverErrors.Swap(0),
		Retries:         u.retries.Swap(0),
		WaitMS:          u.waitMS.Swap(0),
		LastThrottledAt: unixNsToTime(u.lastThrottledUnixNs.Load()), // not reset
	}
	return s
}

func (u *usage) noteRequest() {
	u.requests.Add(1)
}

func (u *usage) noteThrottled() {
	u.throttled.Add(1)
	u.lastThrottledUnixNs.Store(time.Now().UTC().UnixNano())
}

func (u *usage) noteServerError() {
	u.serverErrors.Add(1)
}

func (u *usage) noteRetry() {
	u.retries.Add(1)
}

func (u *usage) noteWait(d time.Duration) {
	if d <= 0 {
		return
	}
	u.waitMS.Add(d.Milliseconds())
}

func (u *usage) noteStatus(code int) {
	switch {
	case code == 429:
		u.noteThrottled()
	case code >= 500 && code <= 599:
		u.noteServerError()
	}
}

func unixNsToTime(ns int64) time.Time {
	if ns <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}
