package tui

import (
	"encoding/json"
	"sort"
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

// listSort is the list display sort from a saved view (display.sort / display.dir).
// Empty key means "leave filter order" (mirror default: updated desc).
type listSort struct {
	key string // updated | created | priority | reopen_count
	dir string // asc | desc (empty → desc when key set)
}

func (s listSort) isDefault() bool {
	if s.key == "" {
		return true
	}
	dir := s.dir
	if dir == "" {
		dir = "desc"
	}
	return s.key == "updated" && dir == "desc"
}

// chip returns a short status-bar label, e.g. "sort:created↑".
func (s listSort) chip() string {
	if s.isDefault() {
		return ""
	}
	dir := s.dir
	if dir == "" {
		dir = "desc"
	}
	arrow := "↓"
	if dir == "asc" {
		arrow = "↑"
	}
	return "sort:" + s.key + arrow
}

// appliedView is the result of parsing a saved view's config for the TUI.
type appliedView struct {
	filter      listFilter
	name        string
	sort        listSort
	groupBy     string // status | status_category | assignee | priority | epic | ""
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

	// Supported filter keys
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
		parseDisplayConfig(wrap.Display, &av)
	}
	return av
}

// parseDisplayConfig reads display.sort / dir / group_by. Supported values are
// applied; unsupported values are reported per-key (e.g. sort=relevance).
func parseDisplayConfig(raw json.RawMessage, av *appliedView) {
	var disp map[string]json.RawMessage
	if err := json.Unmarshal(raw, &disp); err != nil || len(disp) == 0 {
		return
	}
	for k, v := range disp {
		switch k {
		case "sort":
			var s string
			if err := json.Unmarshal(v, &s); err != nil || s == "" {
				continue
			}
			switch s {
			case "updated", "created", "priority", "reopen_count":
				av.sort.key = s
			case "relevance":
				av.unsupported = append(av.unsupported, "sort=relevance")
			default:
				av.unsupported = append(av.unsupported, "sort="+s)
			}
		case "dir":
			var s string
			if err := json.Unmarshal(v, &s); err != nil || s == "" {
				continue
			}
			switch strings.ToLower(s) {
			case "asc", "desc":
				av.sort.dir = strings.ToLower(s)
			}
		case "group_by":
			var s string
			if err := json.Unmarshal(v, &s); err != nil || s == "" {
				continue
			}
			switch s {
			case "status", "status_category", "assignee", "priority", "epic":
				av.groupBy = s
			case "none":
				// explicit no grouping
			default:
				av.unsupported = append(av.unsupported, "group_by="+s)
			}
		default:
			// columns and any other display keys
			if !isEmptyFilterValue(v) {
				av.unsupported = append(av.unsupported, k)
			}
		}
	}
	if av.sort.key == "" {
		// Rejected or missing sort: do not keep a dangling dir.
		av.sort.dir = ""
	} else if av.sort.dir == "" {
		av.sort.dir = "desc"
	}
}

// sortVisible reorders indices in place by listSort. Empty/missing field values
// always sort last, regardless of direction.
func sortVisible(all []row, visible []int, s listSort) {
	if s.key == "" || len(visible) < 2 {
		return
	}
	asc := s.dir == "asc"
	sort.SliceStable(visible, func(i, j int) bool {
		return lessIssue(all[visible[i]].lite, all[visible[j]].lite, s.key, asc)
	})
}

func lessIssue(a, b store.IssueLite, key string, asc bool) bool {
	switch key {
	case "created":
		return lessStr(deref(a.CreatedAt), deref(b.CreatedAt), asc)
	case "priority":
		if a.PriorityRank != b.PriorityRank {
			return lessPriority(a.PriorityRank, b.PriorityRank, asc)
		}
		return lessStr(deref(a.UpdatedAt), deref(b.UpdatedAt), false)
	case "reopen_count":
		if a.ReopenCount != b.ReopenCount {
			return lessRank(a.ReopenCount, b.ReopenCount, true, true, asc)
		}
		return lessStr(deref(a.UpdatedAt), deref(b.UpdatedAt), false)
	case "updated":
		fallthrough
	default:
		return lessStr(deref(a.UpdatedAt), deref(b.UpdatedAt), asc)
	}
}

// lessPriority orders by store.IssueLite.PriorityRank — the issue's position in
// the site's own priority list (1 = first, i.e. most urgent). Rank 0 means the
// issue has no priority, or one the site does not list, and always sorts last.
//
// The rank, not the name, is the sort axis on purpose: Jira localizes priority
// names per account language, so a name table would silently degrade to "all
// unknown" on a Korean or Japanese account and disagree with the web UI, which
// sorts the same saved view by priority_rank.
func lessPriority(a, b int, asc bool) bool {
	return lessRank(a, b, a > 0, b > 0, !asc)
}

// lessStr: empty strings always last; otherwise lexicographic (ISO timestamps).
func lessStr(a, b string, asc bool) bool {
	aOK, bOK := a != "", b != ""
	if !aOK && !bOK {
		return false
	}
	if !aOK {
		return false
	}
	if !bOK {
		return true
	}
	if a == b {
		return false
	}
	if asc {
		return a < b
	}
	return a > b
}

// lessRank compares integer ranks. For priority, lower rank = higher priority
// (Highest=0); desc puts higher priority first (rank ascending inverted via !asc).
func lessRank(a, b int, aOK, bOK, asc bool) bool {
	if !aOK && !bOK {
		return false
	}
	if !aOK {
		return false
	}
	if !bOK {
		return true
	}
	if a == b {
		return false
	}
	if asc {
		return a < b
	}
	return a > b
}

// listLine is one screen row in the issue list (group header or issue).
type listLine struct {
	kind   int // lineKindIssue | lineKindHeader
	label  string
	count  int
	visIdx int // index into m.visible when kind is issue
}

const (
	lineKindIssue = iota
	lineKindHeader
)

// buildListLines expands visible issues into screen lines, inserting group
// headers when groupBy is set. With groupBy empty the result is 1:1 with visible.
func buildListLines(all []row, visible []int, groupBy string) []listLine {
	if groupBy == "" {
		out := make([]listLine, len(visible))
		for i := range visible {
			out[i] = listLine{kind: lineKindIssue, visIdx: i}
		}
		return out
	}

	type bucket struct {
		key   string
		label string
		rank  int   // priority_rank of the group, for groupBy == "priority"
		idxs  []int // indices into visible
	}
	// Epic labels prefer "KEY summary" when the epic row is in the pool.
	epicTitles := map[string]string{}
	if groupBy == "epic" {
		for _, r := range all {
			epicTitles[r.lite.IssueKey] = r.lite.Summary
		}
	}
	order := make([]string, 0)
	byKey := map[string]*bucket{}
	for vi, ai := range visible {
		key, label := groupKeyLabel(all[ai].lite, groupBy, epicTitles)
		b, ok := byKey[key]
		if !ok {
			b = &bucket{key: key, label: label, rank: all[ai].lite.PriorityRank}
			byKey[key] = b
			order = append(order, key)
		}
		b.idxs = append(b.idxs, vi)
	}

	// Order groups: status_category / priority by rank; else label asc; empty last.
	sort.SliceStable(order, func(i, j int) bool {
		ki, kj := order[i], order[j]
		if ki == "" && kj == "" {
			return false
		}
		if ki == "" {
			return false
		}
		if kj == "" {
			return true
		}
		switch groupBy {
		case "status_category":
			return statusCategoryRank(ki) < statusCategoryRank(kj)
		case "priority":
			// Same axis as the priority sort: the site's own rank, never the
			// localized name. Rank 0 (unlisted priority) goes after ranked ones.
			ri, rj := byKey[ki].rank, byKey[kj].rank
			if (ri > 0) != (rj > 0) {
				return ri > 0
			}
			if ri == rj {
				return strings.ToLower(byKey[ki].label) < strings.ToLower(byKey[kj].label)
			}
			return ri < rj
		default:
			return strings.ToLower(byKey[ki].label) < strings.ToLower(byKey[kj].label)
		}
	})

	out := make([]listLine, 0, len(visible)+len(order))
	for _, k := range order {
		b := byKey[k]
		out = append(out, listLine{kind: lineKindHeader, label: b.label, count: len(b.idxs)})
		for _, vi := range b.idxs {
			out = append(out, listLine{kind: lineKindIssue, visIdx: vi})
		}
	}
	return out
}

func groupKeyLabel(lite store.IssueLite, groupBy string, epicTitles map[string]string) (key, label string) {
	switch groupBy {
	case "status":
		if lite.Status == "" {
			return "", "—"
		}
		return lite.Status, lite.Status
	case "status_category":
		cat := lite.StatusCategory
		if cat == "indeterminate" {
			cat = "inprogress"
		}
		if cat == "" {
			return "", "—"
		}
		return cat, cat
	case "assignee":
		name := strings.TrimSpace(deref(lite.Assignee))
		email := strings.TrimSpace(deref(lite.AssigneeEmail))
		if name == "" && email == "" {
			return "", "—"
		}
		if email != "" {
			if name == "" {
				name = email
			}
			return email, name
		}
		return name, name
	case "priority":
		p := strings.TrimSpace(deref(lite.Priority))
		if p == "" {
			return "", "—"
		}
		return p, p
	case "epic":
		ek := strings.TrimSpace(deref(lite.EpicKey))
		if ek == "" {
			return "", "(no epic)"
		}
		if sum := strings.TrimSpace(epicTitles[ek]); sum != "" {
			return ek, ek + " " + sum
		}
		return ek, ek
	default:
		return "", "—"
	}
}

// statusCategoryRank: new → inprogress → done (task "todo" ≡ Jira "new").
func statusCategoryRank(cat string) int {
	switch cat {
	case "new":
		return 0
	case "inprogress":
		return 1
	case "done":
		return 2
	default:
		return 50
	}
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
