package jira

import (
	"github.com/midagedev/gadak/internal/atlhttp"
)

// Usage is a point-in-time snapshot of this client's outbound Jira traffic.
// Counters are process-local until a caller persists them (see store.api_usage).
//
// Requests counts every HTTP attempt, including retries: that is the unit that
// draws from Jira's rate budget. This is our own call volume, not Jira's
// remaining point pool — the site does not expose that.
type Usage = atlhttp.Usage

// Usage returns the current counters without resetting them.
func (c *Client) Usage() Usage {
	if c == nil {
		return Usage{}
	}
	return c.usage.Snapshot()
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
	return c.usage.Take()
}
