package jql

import "strings"

// Match reports whether an issue satisfies the Filter. Semantics match the
// web UI: AND across fields, OR within a field.
func Match(it Issue, f Filter) bool {
	if len(f.JiraProject) > 0 && !containsFold(f.JiraProject, it.Project) {
		return false
	}
	if len(f.Keys) > 0 && !containsFold(f.Keys, it.Key) {
		return false
	}
	if len(f.StatusCategory) > 0 && !containsFold(f.StatusCategory, effectiveCategory(it.StatusCategory)) {
		return false
	}
	if len(f.Status) > 0 && !containsFold(f.Status, it.Status) {
		return false
	}
	if f.Unassigned && it.AssigneeEmail != "" {
		return false
	}
	if len(f.AssigneeEmail) > 0 && !matchPerson(f.AssigneeEmail, it.AssigneeEmail, it.Assignee, it.AssigneeID) {
		return false
	}
	if len(f.ReporterEmail) > 0 && !matchPerson(f.ReporterEmail, it.ReporterEmail, it.Reporter, "") {
		return false
	}
	if len(f.IssueType) > 0 && !containsFold(f.IssueType, it.Type) {
		return false
	}
	if len(f.Priority) > 0 && !containsFold(f.Priority, it.Priority) {
		return false
	}
	if len(f.Labels) > 0 && !anyContains(f.Labels, it.Labels) {
		return false
	}
	if len(f.Components) > 0 && !anyContains(f.Components, it.Components) {
		return false
	}
	if len(f.FixVersions) > 0 && !anyContains(f.FixVersions, it.FixVersions) {
		return false
	}
	if (f.CreatedFrom != nil || f.CreatedTo != nil) && !inRange(it.CreatedAt, f.CreatedFrom, f.CreatedTo) {
		return false
	}
	if (f.UpdatedFrom != nil || f.UpdatedTo != nil) && !inRange(it.UpdatedAt, f.UpdatedFrom, f.UpdatedTo) {
		return false
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		if !textMatch(it, q) {
			return false
		}
	}
	return true
}

func effectiveCategory(sc string) string {
	s := strings.ToLower(strings.TrimSpace(sc))
	switch s {
	case "new", "inprogress", "done":
		return s
	case "indeterminate":
		return "inprogress"
	default:
		return s
	}
}

func matchPerson(want []string, email, name, id string) bool {
	for _, w := range want {
		if strings.EqualFold(w, "currentUser()") {
			continue
		}
		if eqFold(w, email) || eqFold(w, name) || eqFold(w, id) {
			return true
		}
	}
	return false
}

func textMatch(it Issue, q string) bool {
	needle := strings.ToLower(q)
	hay := strings.ToLower(strings.Join([]string{
		it.Key, it.Project, it.Status, it.Type, it.Priority,
		it.Assignee, it.AssigneeEmail, it.Reporter,
		strings.Join(it.Labels, " "),
	}, " "))
	if strings.Contains(hay, needle) {
		return true
	}
	// Compact key form (nma123).
	compact := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	nk := compact(needle)
	return nk != "" && strings.Contains(compact(it.Key), nk)
}

func inRange(iso string, from, to *string) bool {
	if iso == "" {
		return from == nil || *from == ""
	}
	d := iso
	if len(d) >= 10 {
		d = d[:10]
	}
	if from != nil && *from != "" && d < *from {
		return false
	}
	if to != nil && *to != "" && d > *to {
		return false
	}
	return true
}

func containsFold(have []string, want string) bool {
	for _, h := range have {
		if eqFold(h, want) {
			return true
		}
	}
	return false
}

func anyContains(selected, values []string) bool {
	for _, v := range values {
		if containsFold(selected, v) {
			return true
		}
	}
	return false
}

func eqFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
