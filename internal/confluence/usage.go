package confluence

import (
	"github.com/midagedev/scry/internal/atlhttp"
)

// Usage is a point-in-time snapshot of this client's outbound Confluence traffic.
// Counters are process-local until a caller persists them (see store.api_usage).
//
// Requests counts every HTTP attempt, including retries: that is the unit that
// draws from Confluence's rate budget.
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
// snapshot but is NOT cleared.
func (c *Client) TakeUsage() Usage {
	if c == nil {
		return Usage{}
	}
	return c.usage.Take()
}
