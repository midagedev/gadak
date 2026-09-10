package config

import (
	"strings"
	"time"
)

// ISOMilli is the millisecond-precision UTC ISO-8601 layout every timestamp
// gadak writes — store columns, token expiry, usage flush — and that the
// `delta` cursor contract depends on. Milliseconds are not decoration: a
// whole-second cursor would drop a row written in the same second the
// cursor was taken.
const ISOMilli = "2006-01-02T15:04:05.000Z"

// TimeMilli is how Jira stamps timestamps: ISO-8601 with a fixed
// millisecond fraction and a numeric offset with no colon in it, which is
// why time.RFC3339 does not parse them. jira.Layout is this constant under
// its package-local name; the string lives here so the parse owner below
// and the packages that stamp it cannot drift (GDK-1130).
const TimeMilli = "2006-01-02T15:04:05.000-0700"

// TimeNoFrac is TimeMilli without the fraction — the whole-second spelling
// some Jira surfaces and fixtures carry.
const TimeNoFrac = "2006-01-02T15:04:05-0700"

// ParseTimestamp parses every timestamp spelling a mirror column or an
// origin payload carries, and is the single owner of that layout table
// (GDK-1130). Four parsers used to keep private copies, and the copies
// drifted: calendar's table rejected jira.Layout outright, so a
// Jira-stamped instant silently read as "no date"; other tables each
// accepted a different subset of the same spellings.
//
// The set is three layouts, not a growing union: the two Jira no-colon
// stamps above, plus RFC3339Nano — whose optional fraction covers RFC3339,
// ISOMilli, and every colon-offset spelling in one entry. Callers whose
// domain adds genuinely different shapes (the calendar's space-separated
// forms, token expiry's date-only) parse those explicitly beside this
// call, never by extending this table: a shared table that grows per
// caller is the divergence this function exists to end. Unparseable or
// blank input answers false; a parse answers UTC-normalized time.
func ParseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{TimeMilli, TimeNoFrac, time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
