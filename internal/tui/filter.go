package tui

import (
	"encoding/json"
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

// listFilter is the full local filter applied to the issue list.
// statusCategories, when non-empty, overrides tab for category matching.
type listFilter struct {
	tab              Tab
	statusCategories []string // exact match any; empty → use tab
	text             string   // lowercased haystack substring
	unassigned       bool
	assigneeEmail    string // lowercased; match IssueLite.AssigneeEmail
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

func matchStatusCategories(cats []string, category string) bool {
	if len(cats) == 0 {
		return true
	}
	// Normalize indeterminate → inprogress for matching.
	cat := category
	if cat == "indeterminate" {
		cat = "inprogress"
	}
	for _, c := range cats {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "indeterminate" {
			c = "inprogress"
		}
		if c == cat {
			return true
		}
	}
	return false
}

// applyFilter rebuilds the visible index list from the full row slice.
// filter is already lowercased by the caller (or empty).
func applyFilter(all []row, tab Tab, filter string) []int {
	return applyListFilter(all, listFilter{tab: tab, text: filter})
}

func applyListFilter(all []row, f listFilter) []int {
	out := make([]int, 0, len(all))
	for i, r := range all {
		if len(f.statusCategories) > 0 {
			if !matchStatusCategories(f.statusCategories, r.lite.StatusCategory) {
				continue
			}
		} else if !matchTab(f.tab, r.lite.StatusCategory) {
			continue
		}
		if f.unassigned {
			if r.lite.Assignee != nil && strings.TrimSpace(*r.lite.Assignee) != "" {
				continue
			}
			if r.lite.AssigneeID != nil && strings.TrimSpace(*r.lite.AssigneeID) != "" {
				continue
			}
			if r.lite.AssigneeEmail != nil && strings.TrimSpace(*r.lite.AssigneeEmail) != "" {
				continue
			}
		}
		if f.assigneeEmail != "" {
			email := strings.ToLower(deref(r.lite.AssigneeEmail))
			if email == "" || email != f.assigneeEmail {
				continue
			}
		}
		if f.text != "" && !strings.Contains(r.search, f.text) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// appliedView is the result of parsing a saved view's config for the TUI.
type appliedView struct {
	filter      listFilter
	name        string
	unsupported []string
}

// parseSavedViewConfig maps a stored ViewConfig (or a simplified flat object)
// onto filters the TUI can apply. Unsupported keys are listed, not silently
// dropped without notice.
func parseSavedViewConfig(name string, raw json.RawMessage) appliedView {
	av := appliedView{name: name, filter: listFilter{tab: TabAll}}
	if len(raw) == 0 || string(raw) == "null" {
		return av
	}

	// Full shape: { "filters": { ... }, "display": { ... } }
	var wrap struct {
		Filters map[string]json.RawMessage `json:"filters"`
		Display json.RawMessage            `json:"display"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		av.unsupported = append(av.unsupported, "invalid config")
		return av
	}

	// Also accept a flat filter object at the top level (no "filters" wrapper).
	fields := wrap.Filters
	if fields == nil {
		var flat map[string]json.RawMessage
		if err := json.Unmarshal(raw, &flat); err == nil {
			// Drop display-only keys if present at top level.
			delete(flat, "display")
			delete(flat, "filters")
			fields = flat
		}
	}
	if len(fields) == 0 {
		if len(wrap.Display) > 0 && string(wrap.Display) != "null" && string(wrap.Display) != "{}" {
			av.unsupported = append(av.unsupported, "display")
		}
		return av
	}

	// Supported keys
	supported := map[string]bool{
		"status_category": true,
		"assignee_email":  true,
		"assignee":        true, // simplified / test shape
		"q":               true,
		"text":            true, // alias for q
		"unassigned":      true,
	}

	for k, v := range fields {
		if !supported[k] {
			// Empty / false values don't count as applied filters.
			if isEmptyFilterValue(v) {
				continue
			}
			av.unsupported = append(av.unsupported, k)
			continue
		}
		switch k {
		case "status_category":
			var cats []string
			if err := json.Unmarshal(v, &cats); err != nil {
				// single string
				var s string
				if err2 := json.Unmarshal(v, &s); err2 == nil && s != "" {
					cats = []string{s}
				}
			}
			if len(cats) > 0 {
				av.filter.statusCategories = cats
				av.filter.tab = tabFromCategories(cats)
			}
		case "assignee_email":
			var emails []string
			if err := json.Unmarshal(v, &emails); err != nil {
				var s string
				if err2 := json.Unmarshal(v, &s); err2 == nil && s != "" {
					emails = []string{s}
				}
			}
			if len(emails) > 0 {
				// TUI applies the first email only (AND of multiple is rare).
				av.filter.assigneeEmail = strings.ToLower(strings.TrimSpace(emails[0]))
				if len(emails) > 1 {
					av.unsupported = append(av.unsupported, "assignee_email[1+]")
				}
			}
		case "assignee":
			var s string
			if err := json.Unmarshal(v, &s); err == nil && s != "" {
				// "me" is a web convention we cannot resolve without identity
				// in the filter haystack — put it in text so name match still works.
				if strings.EqualFold(s, "me") {
					av.unsupported = append(av.unsupported, "assignee=me")
				} else if strings.Contains(s, "@") {
					av.filter.assigneeEmail = strings.ToLower(strings.TrimSpace(s))
				} else {
					av.filter.text = strings.ToLower(strings.TrimSpace(s))
				}
			}
		case "q", "text":
			var s string
			if err := json.Unmarshal(v, &s); err == nil && s != "" {
				// Prefer dedicated text; if assignee already set text, append.
				q := strings.ToLower(strings.TrimSpace(s))
				if av.filter.text == "" {
					av.filter.text = q
				} else {
					av.filter.text = av.filter.text + " " + q
				}
			}
		case "unassigned":
			var b bool
			if err := json.Unmarshal(v, &b); err == nil {
				av.filter.unassigned = b
			}
		}
	}
	if len(wrap.Display) > 0 && string(wrap.Display) != "null" && string(wrap.Display) != "{}" {
		// Display (sort/group/columns) is intentionally ignored in the TUI list.
		// Only note it if something non-default-looking is present — always
		// mention so the user knows group/sort did not apply.
		var disp map[string]any
		if json.Unmarshal(wrap.Display, &disp) == nil && len(disp) > 0 {
			av.unsupported = append(av.unsupported, "display")
		}
	}
	return av
}

func isEmptyFilterValue(v json.RawMessage) bool {
	s := strings.TrimSpace(string(v))
	return s == "" || s == "null" || s == "[]" || s == `""` || s == "false"
}

// tabFromCategories picks a status tab that best matches the category list,
// for the tab bar highlight when a precise statusCategories filter is active.
func tabFromCategories(cats []string) Tab {
	set := map[string]bool{}
	for _, c := range cats {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "indeterminate" {
			c = "inprogress"
		}
		set[c] = true
	}
	if len(set) == 1 {
		if set["done"] {
			return TabDone
		}
		if set["inprogress"] {
			return TabInProgress
		}
		if set["new"] {
			// No dedicated "new" tab; highlight open (non-done).
			return TabOpen
		}
	}
	// open-ish: new + inprogress without done
	if (set["new"] || set["inprogress"]) && !set["done"] {
		return TabOpen
	}
	return TabAll
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
