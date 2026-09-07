package store

// The jql binding: the three conversions every JQL surface — `gadak search
// --jql`, JQL dashboard datasources, the server's parse endpoint, and views
// — needs between mirror rows and jql's neutral shapes. One owner since
// GDK-1574: before the promotion, four copies of the actor conversion and
// two each of the lite mapping and the ORDER BY sort lived across
// cmd/gadak, internal/server, and internal/dashboards, and the CLI's lite
// mapping dropped ReporterID while its dashboard sibling carried it — the
// drift shipped as GDK-1564 (`reporter = …` answered 0 rows) with the jql
// suite green the whole time, because jql.Match itself was correct.
//
// Import direction is store→jql on purpose. internal/jql stays a leaf —
// its package doc promises it does not import the store, write SQL, or
// talk to Jira — so this file may depend on jql, and jql never depends on
// the store. tools/check-store-deps.sh keeps the other firewall (no
// internal/jira, no internal/atlhttp, no net/http) intact on both graphs.

import (
	"fmt"
	"sort"

	"github.com/midagedev/gadak/internal/jql"
)

// ActorPeople turns the narrow actor projection (QueryActorPeople) into
// the roster jql.ResolveIdentity matches assignee/reporter tokens against.
// Single owner since GDK-1574; a surface that needs the roster calls this,
// it does not re-copy the six-field loop.
func ActorPeople(people []ActorPerson) []jql.Person {
	issues := make([]jql.Issue, len(people))
	for i, p := range people {
		issues[i] = jql.Issue{
			Assignee:      p.AssigneeName,
			AssigneeEmail: p.AssigneeEmail,
			AssigneeID:    p.AssigneeID,
			Reporter:      p.ReporterName,
			ReporterEmail: p.ReporterEmail,
			ReporterID:    p.ReporterID,
		}
	}
	return jql.PeopleFromIssues(issues)
}

// LiteToIssue maps a mirror row onto jql's neutral match shape. Every
// IssueLite column jql.Match can key on must cross here — the field table
// in jqlbind_test.go fails by name if one goes missing again (GDK-1564
// was exactly such a hole, in the CLI copy of this mapping).
func LiteToIssue(l IssueLite) jql.Issue {
	return jql.Issue{
		Key:            l.IssueKey,
		ParentKey:      derefStr(l.ParentKey),
		Project:        l.ProjectKey,
		Status:         l.Status,
		StatusCategory: l.StatusCategory,
		Type:           l.IssueType,
		Priority:       derefStr(l.Priority),
		Assignee:       derefStr(l.Assignee),
		AssigneeEmail:  derefStr(l.AssigneeEmail),
		AssigneeID:     derefStr(l.AssigneeID),
		Reporter:       derefStr(l.Reporter),
		ReporterEmail:  derefStr(l.ReporterEmail),
		ReporterID:     derefStr(l.ReporterID),
		Labels:         l.Labels,
		Components:     l.Components,
		FixVersions:    l.FixVersions,
		CreatedAt:      derefStr(l.CreatedAt),
		UpdatedAt:      derefStr(l.UpdatedAt),
		Duedate:        derefStr(l.Duedate),
		ResolvedAt:     derefStr(l.ResolvedAt),
		SprintID:       sprintIDString(l.SprintID),
		SprintState:    derefStr(l.SprintState),
	}
}

// SortDisplay applies the parsed ORDER BY to matched rows: dir defaults to
// desc, priority breaks ties on updated_at, everything else sorts on the
// asked axis. The ordering contract "a JQL dashboard and `gadak search
// --jql` list the same order" is gated by TestSearchJQLOrdersLikeDashboardJQL
// in cmd/gadak — this function is the one sort both surfaces ride.
func SortDisplay(list []IssueLite, d jql.Display) {
	dir := 1
	if d.Dir != "asc" {
		dir = -1
	}
	lessTime := func(a, b *string) bool {
		av, bv := derefStr(a), derefStr(b)
		if dir < 0 {
			return av > bv
		}
		return av < bv
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch d.Sort {
		case "created":
			return lessTime(a.CreatedAt, b.CreatedAt)
		case "priority":
			if a.PriorityRank != b.PriorityRank {
				if dir < 0 {
					return a.PriorityRank < b.PriorityRank
				}
				return a.PriorityRank > b.PriorityRank
			}
			return derefStr(a.UpdatedAt) > derefStr(b.UpdatedAt)
		default:
			return lessTime(a.UpdatedAt, b.UpdatedAt)
		}
	})
}

func sprintIDString(id *int64) string {
	if id == nil {
		return ""
	}
	return fmt.Sprintf("%d", *id)
}
