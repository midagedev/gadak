package calendar

// GDK-1130: four parsers kept private timestamp layout tables, and the
// tables drifted — this package's was RFC3339-only, so a Jira-stamped
// instant (fixed milliseconds, numeric offset with no colon — the spelling
// every Jira origin emits) read as "no date at all". The ISO spellings now
// come from the single parse owner (config.ParseTimestamp); the calendar's
// own shapes (space-separated, date-only) stay here and are added beside
// it, never inside it.

import "testing"

func TestInstantAcceptsJiraStamp(t *testing.T) {
	// The offset has to differ from the zone for this to be a real test:
	// 2026-08-18 10:04 in Los Angeles is 2026-08-19 01:04 in Seoul, so the
	// day this stamp names in Seoul is the 19th. Pre-fix, parseInstant
	// rejected the no-colon offset and Day's date-only fallback silently
	// read the stamp's first ten characters instead — the 18th, the offset
	// thrown away without a word.
	day, ok := Day("2026-08-18T10:04:05.000-0700", Instant, seoul(t))
	if !ok || day != "2026-08-19" {
		t.Fatalf("a Jira-stamped instant must be a day in the zone it lands in: day=%q ok=%v", day, ok)
	}
	// The no-fraction sibling some fixtures and Server surfaces write.
	day, ok = Day("2026-08-18T10:04:05-0700", Instant, seoul(t))
	if !ok || day != "2026-08-19" {
		t.Fatalf("the no-fraction Jira stamp must parse too: day=%q ok=%v", day, ok)
	}
}
