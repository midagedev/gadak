package jql

import (
	"regexp"
	"strings"
)

// EmitOpts controls how identity is written back (currentUser vs email/id).
type EmitOpts struct {
	Email     string
	AccountID string
}

var bareIdent = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// Emit writes the Jira-facing JQL for a Filter. gadak-only flags are listed
// in the second return (reopened, stale, …) so a round-trip cannot pretend
// they survived.
func Emit(f Filter, d Display, opts EmitOpts) (string, []string) {
	var parts []string
	var omitted []string

	if len(f.JiraProject) > 0 {
		parts = append(parts, inClause("project", f.JiraProject))
	}
	if len(f.JiraProjectNot) > 0 {
		parts = append(parts, notInClause("project", f.JiraProjectNot))
	}
	if len(f.Keys) > 0 {
		parts = append(parts, inClause("key", f.Keys))
	}
	if len(f.StatusCategory) > 0 {
		names := make([]string, len(f.StatusCategory))
		for i, c := range f.StatusCategory {
			names[i] = statusCategoryJira(c)
		}
		parts = append(parts, inClause("statusCategory", names))
	}
	if len(f.Status) > 0 {
		parts = append(parts, inClause("status", f.Status))
	}
	if len(f.IssueType) > 0 {
		parts = append(parts, inClause("type", f.IssueType))
	}
	if f.Unassigned {
		parts = append(parts, "assignee is EMPTY")
	}
	if len(f.AssigneeEmail) > 0 {
		vals := make([]string, 0, len(f.AssigneeEmail))
		for _, e := range f.AssigneeEmail {
			if isConfiguredMe(e, opts) {
				vals = append(vals, "currentUser()")
			} else {
				vals = append(vals, quote(e))
			}
		}
		if len(vals) == 1 && vals[0] == "currentUser()" {
			parts = append(parts, "assignee = currentUser()")
		} else if len(vals) == 1 {
			parts = append(parts, "assignee = "+vals[0])
		} else {
			parts = append(parts, "assignee in ("+strings.Join(vals, ", ")+")")
		}
	}
	if len(f.ReporterEmail) > 0 {
		parts = append(parts, inClause("reporter", f.ReporterEmail))
	}
	if len(f.Labels) > 0 {
		parts = append(parts, inClause("labels", f.Labels))
	}
	if len(f.Priority) > 0 {
		parts = append(parts, inClause("priority", f.Priority))
	}
	if len(f.Components) > 0 {
		parts = append(parts, inClause("component", f.Components))
	}
	if len(f.FixVersions) > 0 {
		parts = append(parts, inClause("fixVersion", f.FixVersions))
	}
	parts = append(parts, dateClause("created", f.CreatedFrom, f.CreatedTo)...)
	parts = append(parts, dateClause("updated", f.UpdatedFrom, f.UpdatedTo)...)
	parts = append(parts, dateClause("duedate", f.DueFrom, f.DueTo)...)
	parts = append(parts, dateClause("resolved", f.ResolvedFrom, f.ResolvedTo)...)
	if q := strings.TrimSpace(f.Q); q != "" {
		parts = append(parts, "text ~ "+quote(q))
	}

	if f.Reopened {
		omitted = append(omitted, "reopened")
	}
	if f.Stale {
		omitted = append(omitted, "stale")
	}
	if len(f.TeamGroup) > 0 {
		omitted = append(omitted, "team_group")
	}
	if len(f.Severity) > 0 {
		omitted = append(omitted, "severity")
	}
	if len(f.QARun) > 0 || len(f.QASuite) > 0 || len(f.QAImpact) > 0 {
		omitted = append(omitted, "qa")
	}
	if len(f.DeployState) > 0 {
		omitted = append(omitted, "deploy")
	}
	if len(f.SourceProject) > 0 {
		omitted = append(omitted, "source_project")
	}
	if len(f.SourceProjectNot) > 0 {
		omitted = append(omitted, "source_project_not")
	}
	if len(f.Fields) > 0 {
		omitted = append(omitted, "custom fields")
	}

	jql := strings.Join(parts, " AND ")
	if d.Sort != "" {
		field := d.Sort
		switch d.Sort {
		case "updated", "created", "priority", "due":
			if d.Sort == "due" {
				field = "duedate"
			}
		default:
			field = "updated"
		}
		dir := d.Dir
		if dir != "asc" && dir != "desc" {
			dir = "desc"
		}
		if jql != "" {
			jql += " "
		}
		jql += "ORDER BY " + field + " " + strings.ToUpper(dir)
	}
	return jql, omitted
}

func isConfiguredMe(v string, opts EmitOpts) bool {
	if opts.AccountID != "" && strings.EqualFold(v, opts.AccountID) {
		return true
	}
	if opts.Email != "" && strings.EqualFold(v, opts.Email) {
		return true
	}
	return false
}

func statusCategoryJira(cat string) string {
	switch strings.ToLower(cat) {
	case "new":
		return "To Do"
	case "inprogress":
		return "In Progress"
	case "done":
		return "Done"
	default:
		return cat
	}
}

func inClause(field string, values []string) string {
	if len(values) == 1 {
		return field + " = " + quote(values[0])
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quote(v)
	}
	return field + " in (" + strings.Join(quoted, ", ") + ")"
}

func notInClause(field string, values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quote(v)
	}
	return field + " not in (" + strings.Join(quoted, ", ") + ")"
}

func dateClause(field string, from, to *string) []string {
	var out []string
	if from != nil && *from != "" {
		out = append(out, field+" >= "+quote(*from))
	}
	if to != nil && *to != "" {
		out = append(out, field+" <= "+quote(*to))
	}
	return out
}

func quote(s string) string {
	if s == "currentUser()" {
		return s
	}
	if bareIdent.MatchString(s) && !isReserved(s) {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func isReserved(s string) bool {
	switch strings.ToLower(s) {
	case "and", "or", "not", "in", "is", "empty", "order", "by", "asc", "desc":
		return true
	}
	return false
}
