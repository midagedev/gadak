// Package calendar is the single owner of "which calendar day is this?".
//
// Two stored shapes exist:
//
//   - Instant — created_at, updated_at, resolved_at, status_changed_at,
//     reopened_at, assignee_changed_at. UTC timestamps. The calendar day is
//     the day in the given zone (local for the UI / Match).
//   - Date — duedate. A YYYY-MM-DD calendar date stored as written. Never
//     parse it as UTC midnight.
//
// Go and the web (web/src/lib/calendar.ts) implement the same rules.
// Explain reports the decision so a mismatch is observable.
package calendar

import (
	"strings"
	"time"
)

// Kind selects how a stored value is read.
type Kind int

const (
	// Instant is a UTC timestamp (RFC3339 / ISO-8601).
	Instant Kind = iota
	// Date is a calendar day stored as YYYY-MM-DD.
	Date
)

func (k Kind) String() string {
	if k == Date {
		return "date"
	}
	return "instant"
}

// Zone is the calendar used for Instant → day. Date values ignore it.
type Zone struct {
	loc *time.Location
}

// Local is the process local zone (the viewer's zone in the UI / CLI).
func Local() Zone { return Zone{time.Local} }

// UTC is the UTC zone.
func UTC() Zone { return Zone{time.UTC} }

// In wraps a location. nil falls back to Local.
func In(loc *time.Location) Zone {
	if loc == nil {
		return Local()
	}
	return Zone{loc}
}

// Named loads an IANA zone (tests pin Asia/Seoul so CI UTC cannot hide a miss).
func Named(name string) (Zone, error) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return Zone{}, err
	}
	return Zone{loc}, nil
}

func (z Zone) locOrLocal() *time.Location {
	if z.loc == nil {
		return time.Local
	}
	return z.loc
}

// Name is the IANA / Local identifier Explain reports.
func (z Zone) Name() string {
	return z.locOrLocal().String()
}

// Decision is one Day conversion, for tests and debugging.
type Decision struct {
	Raw         string `json:"raw"`
	Kind        string `json:"kind"`
	Zone        string `json:"zone"`
	CalendarDay string `json:"calendar_day"`
	OK          bool   `json:"ok"`
}

// Explain reports how Day classified raw.
func Explain(raw string, kind Kind, z Zone) Decision {
	d := Decision{Raw: raw, Kind: kind.String(), Zone: z.Name()}
	day, ok := Day(raw, kind, z)
	d.CalendarDay = day
	d.OK = ok
	return d
}

// Day returns YYYY-MM-DD for raw, or false when it cannot be read.
func Day(raw string, kind Kind, z Zone) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if kind == Date {
		return dateOnly(raw)
	}
	if ymd, ok := dateOnly(raw); ok && len(raw) == 10 {
		// Date-only string passed as Instant: do not parse as UTC midnight.
		return ymd, true
	}
	t, ok := parseInstant(raw)
	if !ok {
		return dateOnly(raw)
	}
	return t.In(z.locOrLocal()).Format("2006-01-02"), true
}

// InRange is an inclusive YYYY-MM-DD compare after Day.
// An empty raw fails when any bound is set; with no bounds it still passes.
func InRange(raw string, kind Kind, from, to string, z Zone) bool {
	if strings.TrimSpace(raw) == "" {
		return strings.TrimSpace(from) == "" && strings.TrimSpace(to) == ""
	}
	day, ok := Day(raw, kind, z)
	if !ok {
		return false
	}
	if from != "" && day < from {
		return false
	}
	if to != "" && day > to {
		return false
	}
	return true
}

// StartOfWeekMonday is the ISO date of Monday 00:00 in z for now.
func StartOfWeekMonday(now time.Time, z Zone) string {
	t := now.In(z.locOrLocal())
	offset := int(t.Weekday()+6) % 7 // Monday = 0
	mon := time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, z.locOrLocal())
	return mon.Format("2006-01-02")
}

// FormatDay is YYYY-MM-DD of t in z. compileDate uses this so JQL
// startOfDay() / -7d land on the same calendar the matcher uses.
func FormatDay(t time.Time, z Zone) string {
	return t.In(z.locOrLocal()).Format("2006-01-02")
}

func dateOnly(raw string) (string, bool) {
	if len(raw) < 10 {
		return "", false
	}
	ymd := raw[:10]
	if _, err := time.Parse("2006-01-02", ymd); err != nil {
		return "", false
	}
	return ymd, true
}

func parseInstant(raw string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
