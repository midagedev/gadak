package tui

import (
	"strings"
	"time"

	"github.com/midagedev/scry/internal/store"
)

// Tab is the status-category filter. Logic keys on status_category only.
type Tab int

const (
	TabAll  Tab = iota
	TabOpen     // non-done
	TabInProgress
	TabDone
)

func (t Tab) Label() string {
	switch t {
	case TabOpen:
		return "open"
	case TabInProgress:
		return "in progress"
	case TabDone:
		return "done"
	default:
		return "all"
	}
}

// row is one list entry with a pre-lowercased haystack for filter matching.
// Building the haystack once keeps / filtering cheap at 10k rows.
type row struct {
	lite   store.IssueLite
	search string // lower(key + " " + summary + " " + assignee)
}

func buildRows(lites []store.IssueLite) []row {
	out := make([]row, len(lites))
	for i, l := range lites {
		assignee := ""
		if l.Assignee != nil {
			assignee = *l.Assignee
		}
		out[i] = row{
			lite:   l,
			search: strings.ToLower(l.IssueKey + " " + l.Summary + " " + assignee),
		}
	}
	return out
}

// matchTab reports whether a status category belongs on the given tab.
func matchTab(tab Tab, category string) bool {
	switch tab {
	case TabOpen:
		return category != "done"
	case TabInProgress:
		return category == "inprogress" || category == "indeterminate"
	case TabDone:
		return category == "done"
	default:
		return true
	}
}

// applyFilter rebuilds the visible index list from the full row slice.
// filter is already lowercased by the caller (or empty).
func applyFilter(all []row, tab Tab, filter string) []int {
	out := make([]int, 0, len(all))
	for i, r := range all {
		if !matchTab(tab, r.lite.StatusCategory) {
			continue
		}
		if filter != "" && !strings.Contains(r.search, filter) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// relativeTime renders a compact age for ISO-ish timestamps stored by the mirror.
func relativeTime(iso string, now time.Time) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		// Watermarks sometimes arrive without zone; try a few common shapes.
		for _, layout := range []string{
			"2006-01-02T15:04:05.000-0700",
			"2006-01-02T15:04:05.000+0000",
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02 15:04:05",
		} {
			if t, err = time.Parse(layout, iso); err == nil {
				break
			}
		}
		if err != nil {
			return iso
		}
	}
	d := now.Sub(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h ago"
	case d < 30*24*time.Hour:
		return itoa(int(d.Hours()/24)) + "d ago"
	default:
		return itoa(int(d.Hours()/(24*30))) + "mo ago"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
