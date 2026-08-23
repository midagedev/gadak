package jql

import (
	"strings"

	"github.com/midagedev/gadak/internal/calendar"
	"github.com/midagedev/gadak/internal/jira"
)

// Match reports whether an issue satisfies the Filter. Semantics match the
// web UI: AND across fields, OR within a field. Instants use the process
// local calendar (same default as the web owner).
func Match(it Issue, f Filter) bool {
	return MatchIn(it, f, calendar.Local())
}

// MatchIn is Match with an explicit calendar zone. Tests pin Asia/Seoul.
func MatchIn(it Issue, f Filter, z calendar.Zone) bool {
	if len(f.JiraProject) > 0 && !containsFold(f.JiraProject, it.Project) {
		return false
	}
	if len(f.JiraProjectNot) > 0 && containsFold(f.JiraProjectNot, it.Project) {
		return false
	}
	if len(f.Keys) > 0 && !containsFold(f.Keys, it.Key) {
		return false
	}
	if len(f.Parent) > 0 && !containsFold(f.Parent, it.ParentKey) {
		return false
	}
	// Negation twins (GDK-771): include narrows first, exclude subtracts —
	// exclude wins on overlap. Only the JQL-reachable axes appear here; the
	// mirror-only twins (team_group/severity/qa/deploy) cannot come out of
	// Compile, and Issue does not carry those fields.
	if len(f.StatusCategory) > 0 && !containsFold(f.StatusCategory, effectiveCategory(it.StatusCategory)) {
		return false
	}
	if len(f.StatusCategoryNot) > 0 && containsFold(f.StatusCategoryNot, effectiveCategory(it.StatusCategory)) {
		return false
	}
	if len(f.Status) > 0 && !containsFold(f.Status, it.Status) {
		return false
	}
	if len(f.StatusNot) > 0 && containsFold(f.StatusNot, it.Status) {
		return false
	}
	if f.Unassigned && hasAssignee(it) {
		return false
	}
	if len(f.AssigneeEmail) > 0 && !matchPerson(f.AssigneeEmail, it.AssigneeEmail, it.Assignee, it.AssigneeID) {
		return false
	}
	if len(f.AssigneeEmailNot) > 0 && matchPerson(f.AssigneeEmailNot, it.AssigneeEmail, it.Assignee, it.AssigneeID) {
		return false
	}
	if len(f.ReporterEmail) > 0 && !matchPerson(f.ReporterEmail, it.ReporterEmail, it.Reporter, it.ReporterID) {
		return false
	}
	if len(f.ReporterEmailNot) > 0 && matchPerson(f.ReporterEmailNot, it.ReporterEmail, it.Reporter, it.ReporterID) {
		return false
	}
	if len(f.IssueType) > 0 && !containsFold(f.IssueType, it.Type) {
		return false
	}
	if len(f.IssueTypeNot) > 0 && containsFold(f.IssueTypeNot, it.Type) {
		return false
	}
	if len(f.Priority) > 0 && !containsFold(f.Priority, it.Priority) {
		return false
	}
	if len(f.PriorityNot) > 0 && containsFold(f.PriorityNot, it.Priority) {
		return false
	}
	if len(f.Labels) > 0 && !anyContains(f.Labels, it.Labels) {
		return false
	}
	if len(f.LabelsNot) > 0 && anyContains(f.LabelsNot, it.Labels) {
		return false
	}
	if len(f.Components) > 0 && !anyContains(f.Components, it.Components) {
		return false
	}
	if len(f.ComponentsNot) > 0 && anyContains(f.ComponentsNot, it.Components) {
		return false
	}
	if len(f.FixVersions) > 0 && !anyContains(f.FixVersions, it.FixVersions) {
		return false
	}
	if len(f.FixVersionsNot) > 0 && anyContains(f.FixVersionsNot, it.FixVersions) {
		return false
	}
	if len(f.SprintIDs) > 0 && !containsFold(f.SprintIDs, it.SprintID) {
		return false
	}
	if len(f.SprintState) > 0 && !containsFold(f.SprintState, it.SprintState) {
		return false
	}
	if (f.CreatedFrom != nil || f.CreatedTo != nil) && !inRange(it.CreatedAt, calendar.Instant, f.CreatedFrom, f.CreatedTo, z) {
		return false
	}
	if (f.UpdatedFrom != nil || f.UpdatedTo != nil) && !inRange(it.UpdatedAt, calendar.Instant, f.UpdatedFrom, f.UpdatedTo, z) {
		return false
	}
	if (f.DueFrom != nil || f.DueTo != nil) && !inRange(it.Duedate, calendar.Date, f.DueFrom, f.DueTo, z) {
		return false
	}
	if (f.ResolvedFrom != nil || f.ResolvedTo != nil) && !inRange(it.ResolvedAt, calendar.Instant, f.ResolvedFrom, f.ResolvedTo, z) {
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
	if cat, ok := jira.KnownCategory(s); ok {
		return cat
	}
	return s
}

func hasAssignee(it Issue) bool {
	return strings.TrimSpace(it.AssigneeID) != "" ||
		strings.TrimSpace(it.AssigneeEmail) != "" ||
		strings.TrimSpace(it.Assignee) != ""
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

func inRange(iso string, kind calendar.Kind, from, to *string, z calendar.Zone) bool {
	return calendar.InRange(iso, kind, derefRange(from), derefRange(to), z)
}

func derefRange(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
