package store

// The jql binding's own gates (GDK-1574): the field table that makes a
// dropped IssueLite column fail by name (the exact hole that shipped
// GDK-1564 in the CLI copy of this mapping), and the actor/sort contracts
// the four former copy sites now ride.

import (
	"reflect"
	"testing"

	"github.com/midagedev/gadak/internal/jql"
)

// liteFieldPairs is the IssueLite→jql.Issue contract: every column the
// mirror reads that jql.Match can key on, and its jql.Issue counterpart.
// A pair dropped by LiteToIssue fails by name below; a jql.Issue field
// with no pair fails the completeness loop, so a new match axis cannot
// ship unmapped either.
var liteFieldPairs = []struct{ lite, issue string }{
	{"IssueKey", "Key"},
	{"ParentKey", "ParentKey"},
	{"ProjectKey", "Project"},
	{"Status", "Status"},
	{"StatusCategory", "StatusCategory"},
	{"IssueType", "Type"},
	{"Priority", "Priority"},
	{"Assignee", "Assignee"},
	{"AssigneeEmail", "AssigneeEmail"},
	{"AssigneeID", "AssigneeID"},
	{"Reporter", "Reporter"},
	{"ReporterEmail", "ReporterEmail"},
	{"ReporterID", "ReporterID"},
	{"Labels", "Labels"},
	{"Components", "Components"},
	{"FixVersions", "FixVersions"},
	{"CreatedAt", "CreatedAt"},
	{"UpdatedAt", "UpdatedAt"},
	{"Duedate", "Duedate"},
	{"ResolvedAt", "ResolvedAt"},
	{"SprintID", "SprintID"},
	{"SprintState", "SprintState"},
}

// TestLiteToIssueCarriesEveryMappedField enumerates the mapping table:
// each pair is set to a distinct marker on the IssueLite side and must
// arrive on the jql.Issue side. GDK-1564 was a missing ReporterID line
// here that no test could see.
func TestLiteToIssueCarriesEveryMappedField(t *testing.T) {
	lite := IssueLite{}
	lv := reflect.ValueOf(&lite).Elem()
	for _, p := range liteFieldPairs {
		marker := "m:" + p.issue
		f := lv.FieldByName(p.lite)
		switch f.Kind() {
		case reflect.String:
			f.SetString(marker)
		case reflect.Pointer:
			if f.Type().Elem().Kind() == reflect.Int64 {
				n := int64(42)
				f.Set(reflect.ValueOf(&n))
			} else {
				s := marker
				f.Set(reflect.ValueOf(&s))
			}
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{marker}))
		default:
			t.Fatalf("unhandled kind for IssueLite.%s: %s", p.lite, f.Kind())
		}
	}

	got := LiteToIssue(lite)
	gv := reflect.ValueOf(got)
	for _, p := range liteFieldPairs {
		f := gv.FieldByName(p.issue)
		if !f.IsValid() {
			t.Fatalf("jql.Issue has no field %s — table is stale", p.issue)
		}
		marker := "m:" + p.issue
		switch {
		case p.lite == "SprintID": // *int64 42 → string "42"
			if f.String() != "42" {
				t.Errorf("IssueLite.%s never reaches jql.Issue.%s (got %q, want %q)", p.lite, p.issue, f.String(), "42")
			}
		case f.Kind() == reflect.String:
			if f.String() != marker {
				t.Errorf("IssueLite.%s never reaches jql.Issue.%s (got %q, want %q)", p.lite, p.issue, f.String(), marker)
			}
		case f.Kind() == reflect.Slice:
			if !reflect.DeepEqual(f.Interface(), []string{marker}) {
				t.Errorf("IssueLite.%s never reaches jql.Issue.%s (got %v)", p.lite, p.issue, f.Interface())
			}
		default:
			t.Fatalf("unhandled jql.Issue.%s kind %s", p.issue, f.Kind())
		}
	}

	// Completeness: every jql.Issue field must be claimed by a pair, so a
	// new match axis without a mapping row fails here rather than silently
	// matching zero.
	seen := map[string]bool{}
	for _, p := range liteFieldPairs {
		seen[p.issue] = true
	}
	jt := reflect.TypeOf(jql.Issue{})
	for i := 0; i < jt.NumField(); i++ {
		if !seen[jt.Field(i).Name] {
			t.Errorf("jql.Issue.%s has no IssueLite pair in liteFieldPairs — extend the table or LiteToIssue", jt.Field(i).Name)
		}
	}
}

// TestActorPeopleBuildsRoster: the projection's six columns become one
// roster the resolver can match on either axis — the id-only reporter row
// is the shape GDK-1564 was about.
func TestActorPeopleBuildsRoster(t *testing.T) {
	people := ActorPeople([]ActorPerson{
		{AssigneeName: "Dana Whitfield", AssigneeEmail: "dana@example.com", AssigneeID: "acc-hc"},
		{ReporterID: "acc-rp"},
	})
	var idOnly, dana bool
	for _, p := range people {
		if p.AccountID == "acc-rp" && p.Email == "" {
			idOnly = true
		}
		if p.AccountID == "acc-hc" && p.Email == "dana@example.com" && p.Name == "Dana Whitfield" {
			dana = true
		}
	}
	if !idOnly {
		t.Errorf("id-only reporter missing from roster: %+v", people)
	}
	if !dana {
		t.Errorf("full assignee triple missing from roster: %+v", people)
	}
}

// TestSortDisplayAxes pins the three ORDER BY behaviors both surfaces
// inherit: default updated desc, created asc, and the priority tiebreak on
// updated_at. Cross-surface parity itself is gated in cmd/gadak
// (TestSearchJQLOrdersLikeDashboardJQL).
func TestSortDisplayAxes(t *testing.T) {
	s := func(v string) *string { return &v }
	base := []IssueLite{
		{IssueKey: "OLD", CreatedAt: s("2026-01-01"), UpdatedAt: s("2026-01-02")},
		{IssueKey: "NEW", CreatedAt: s("2026-03-01"), UpdatedAt: s("2026-03-02")},
	}

	updatedDesc := append([]IssueLite(nil), base...)
	SortDisplay(updatedDesc, jql.Display{})
	if updatedDesc[0].IssueKey != "NEW" {
		t.Errorf("default sort = %v, want NEW first (updated desc)", keys(updatedDesc))
	}

	createdAsc := append([]IssueLite(nil), base...)
	SortDisplay(createdAsc, jql.Display{Sort: "created", Dir: "asc"})
	if createdAsc[0].IssueKey != "OLD" {
		t.Errorf("created asc = %v, want OLD first", keys(createdAsc))
	}

	prio := []IssueLite{
		{IssueKey: "P1-OLD", PriorityRank: 1, UpdatedAt: s("2026-01-01")},
		{IssueKey: "P1-NEW", PriorityRank: 1, UpdatedAt: s("2026-02-01")},
		{IssueKey: "P0", PriorityRank: 0, UpdatedAt: s("2026-01-15")},
	}
	SortDisplay(prio, jql.Display{Sort: "priority", Dir: "desc"})
	want := []string{"P0", "P1-NEW", "P1-OLD"}
	for i, k := range want {
		if prio[i].IssueKey != k {
			t.Fatalf("priority desc = %v, want %v (rank asc, updated desc tiebreak)", keys(prio), want)
		}
	}
}

func keys(list []IssueLite) []string {
	out := make([]string, len(list))
	for i, l := range list {
		out[i] = l.IssueKey
	}
	return out
}
