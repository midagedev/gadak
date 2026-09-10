package atlhttp

// UsageBox is the instrument pair every Atlassian-family client carries:
// the call meter behind Usage / TakeUsage and the per-kind request
// breakdown behind TakeRequestBreakdown. jira.Client and confluence.Client
// embed it by value (GDK-1780), so the three read methods promote from one
// owner instead of a byte-identical pair per package; transport wiring
// hands out &box.Meter / &box.Breakdown the same way the clients once
// handed their own fields. Counters are process-local, atomic, and never
// block a request.
type UsageBox struct {
	// Meter is the call-volume counter: requests, retries, throttles,
	// waits. See Usage / TakeUsage.
	Meter Meter
	// Breakdown tallies per-kind request count and wall time for the sync
	// pass's "sync: requests …" line. The Confluence client also records
	// its PauseBetween politeness sleep here.
	Breakdown Breakdown
}

// Usage returns the current counters without resetting them.
func (b *UsageBox) Usage() Usage {
	if b == nil {
		return Usage{}
	}
	return b.Meter.Snapshot()
}

// TakeUsage returns the current counters and zeroes the numeric fields so a
// flusher can accumulate into daily totals without double-counting.
//
// LastThrottledAt is a timestamp, not a counter: it is included in the
// snapshot but is NOT cleared.
func (b *UsageBox) TakeUsage() Usage {
	if b == nil {
		return Usage{}
	}
	return b.Meter.Take()
}

// TakeRequestBreakdown returns the per-kind request tally since the last
// take, zeroing it — the accumulate-once shape of TakeUsage, feeding the
// sync pass's "sync: requests …" line.
func (b *UsageBox) TakeRequestBreakdown() BreakdownSnapshot {
	if b == nil {
		return BreakdownSnapshot{}
	}
	return b.Breakdown.Take()
}
