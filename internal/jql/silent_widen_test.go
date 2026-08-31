package jql

import (
	"strings"
	"testing"
)

// GDK-1234 FAIL-first: five silent-widening shapes must become loud
// refusals (decision 0007: "silence is the failure mode this package
// exists to prevent"). These assertions ran red on the pre-fix compiler —
// the red output showed the widened axis applied with Unsupported empty.

// D1: a second AND-ed include (= or IN) on a single-valued axis is an
// intersection in Jira (usually zero rows); merging it into the has-any
// list is a silent union. The axis must be refused out loud, not widened.
func TestD1SingleAxisSecondIncludeIsRefused(t *testing.T) {
	cases := []struct {
		jql  string
		axis string
	}{
		{`status = Open AND status = Done`, "status"},
		{`status = Open AND status in (Done)`, "status"},
		{`project = STD AND project in (NMB, NMC)`, "project"},
		{`priority = High AND priority = Low`, "priority"},
		{`type = Bug AND type in (Task)`, "type"},
	}
	for _, tc := range cases {
		res := Parse(tc.jql, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", tc.jql, res.Error, res.Message)
		}
		blob := strings.Join(res.Unsupported, " ")
		if !strings.Contains(blob, "intersection, not a union") {
			t.Errorf("%q must be refused out loud, got unsupported=%q applied=%v", tc.jql, blob, res.Applied)
		}
		if got := appliedAxis(res.Filters.Status, res.Filters.JiraProject, res.Filters.Priority, res.Filters.IssueType); got != "" {
			t.Errorf("%q left a widened include applied: %s", tc.jql, got)
		}
	}
}

func appliedAxis(groups ...[]string) string {
	for _, g := range groups {
		if len(g) > 0 {
			return strings.Join(g, ",")
		}
	}
	return ""
}

// D1: the multi-valued guard must catch = AND IN too, not just = AND =.
// `labels = a AND labels in (b, c)` means has-all in Jira; the has-any
// merge matches an a-only issue.
func TestD1MultiAxisEqAndInIsRefused(t *testing.T) {
	for _, q := range []string{
		`labels = a AND labels in (b, c)`,
		`labels in (a) AND labels = b`,
		`labels in (a) AND labels in (b)`,
		`component = infra AND component in (api)`,
	} {
		res := Parse(q, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", q, res.Error, res.Message)
		}
		blob := strings.Join(res.Unsupported, " ")
		if !strings.Contains(blob, "has-all") {
			t.Errorf("%q must be refused out loud, got unsupported=%q", q, blob)
		}
		if len(res.Filters.Labels) > 0 || len(res.Filters.Components) > 0 {
			t.Errorf("%q left a widened include applied: labels=%+v components=%+v", q, res.Filters.Labels, res.Filters.Components)
		}
	}
}

// D2: `resolution is EMPTY` intersects the StatusCategory axis, it never
// unions. `statusCategory = Done AND resolution is EMPTY` is zero rows in
// Jira — the pre-fix union matched every issue.
func TestD2ResolutionEmptyIntersectsStatusCategory(t *testing.T) {
	res := Parse(`statusCategory = Done AND resolution is EMPTY`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	blob := strings.Join(res.Unsupported, " ")
	if !strings.Contains(blob, "zero rows") {
		t.Errorf("empty intersection must be refused out loud, got unsupported=%q statusCategory=%+v", blob, res.Filters.StatusCategory)
	}
	if got := strings.Join(res.Filters.StatusCategory, ","); got != "done" {
		t.Errorf("prior constraint must stay [done], got [%s]", got)
	}

	// Overlapping include sets intersect exactly.
	res = Parse(`resolution is EMPTY AND statusCategory in (Done, "In Progress")`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if got := strings.Join(res.Filters.StatusCategory, ","); got != "inprogress" {
		t.Errorf("intersection of {new,inprogress} and {done,inprogress} is inprogress, got [%s]", got)
	}
	if len(res.Unsupported) != 0 {
		t.Errorf("non-empty intersection is exact, nothing to refuse: %v", res.Unsupported)
	}

	// The same contradiction from two resolution clauses.
	res = Parse(`resolution is EMPTY AND resolution is not EMPTY`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if !strings.Contains(strings.Join(res.Unsupported, " "), "zero rows") {
		t.Errorf("contradictory resolution clauses must be refused: %v", res.Unsupported)
	}
}

// D3: quote must escape the backslash, or lexString eats it on re-parse —
// `C:\path` emitted as "C:\path" round-trips as `C:path`.
func TestD3BackslashRoundTrip(t *testing.T) {
	f := EmptyFilter()
	f.Labels = []string{`C:\path`}
	jql, omitted := Emit(f, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if jql != `labels = "C:\\path"` {
		t.Fatalf("emit must escape the backslash, got %q", jql)
	}

	f.Labels = []string{`C:\path`, `say "hi"`, `mix \" both`}
	jql, _ = Emit(f, Display{}, EmitOpts{})
	res := Parse(jql, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	want := []string{`C:\path`, `say "hi"`, `mix \" both`}
	if got := res.Filters.Labels; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("round trip mangled the values: emit=%q labels=%+v", jql, got)
	}

	// The Parse→Emit→Parse path (res.JQL) is stable for escaped input too.
	res = Parse(`labels = "C:\\path"`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	again := Parse(res.JQL, fixedOpts())
	if got := again.Filters.Labels; len(got) != 1 || got[0] != `C:\path` {
		t.Fatalf("res.JQL round trip: %q → %+v", res.JQL, got)
	}
}

// D4: a clause whose values are all empty must be refused, not vanished —
// pre-fix `project != ""` appeared in neither Applied nor Unsupported.
func TestD4EmptyValueClauseIsRefused(t *testing.T) {
	for _, q := range []string{
		`project != ""`,
		`status not in ("")`,
		`project in ()`,
		`labels = ""`,
		`text ~ ""`,
		`statusCategory in ()`,
	} {
		res := Parse(q, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", q, res.Error, res.Message)
		}
		blob := strings.Join(res.Unsupported, " ")
		if !strings.Contains(blob, "empty value") {
			t.Errorf("%q must be refused out loud, got unsupported=%q applied=%v", q, blob, res.Applied)
		}
	}
}

// D5: multiple text clauses cannot fold into one space-joined needle —
// `text ~ a OR text ~ b` became Q="a b", a substring that matches almost
// nothing. Refuse instead; a single text clause is unchanged.
func TestD5MultiTextIsRefused(t *testing.T) {
	for _, q := range []string{
		`text ~ webhook OR text ~ retry`,
		`summary ~ a OR description ~ b`,
		`text ~ webhook OR text = retry`,
	} {
		res := Parse(q, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", q, res.Error, res.Message)
		}
		if res.Filters.Q != "" {
			t.Errorf("%q joined needles into Q=%q", q, res.Filters.Q)
		}
		if !strings.Contains(strings.Join(res.Unsupported, " "), "OR of text clauses") {
			t.Errorf("%q must be refused out loud, got unsupported=%v", q, res.Unsupported)
		}
	}

	res := Parse(`text ~ webhook AND text ~ retry`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if res.Filters.Q != "webhook" {
		t.Errorf("first text clause must stay applied, got Q=%q", res.Filters.Q)
	}
	if !strings.Contains(strings.Join(res.Unsupported, " "), "one text clause") {
		t.Errorf("second AND-ed text clause must be refused, got %v", res.Unsupported)
	}

	res = Parse(`text ~ webhook`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if res.Filters.Q != "webhook" || len(res.Unsupported) != 0 {
		t.Errorf("single text clause is unchanged: Q=%q unsupported=%v", res.Filters.Q, res.Unsupported)
	}
}

// Audit sweep (same silent-union eye over the axes D1 did not name): every
// other has-any axis widens the same way on a second AND-ed include.
func TestAuditOtherAxesSecondIncludeIsRefused(t *testing.T) {
	cases := []struct {
		jql string
		msg string
	}{
		{`assignee = "a@x.com" AND assignee = "b@x.com"`, "intersection, not a union"},
		{`reporter = "a@x.com" AND reporter in ("b@x.com")`, "intersection, not a union"},
		{`key = STD-1 AND key = STD-2`, "intersection, not a union"},
		{`parent = STD-1 AND parent in (STD-2)`, "intersection, not a union"},
		{`sprint = 12 AND sprint = 13`, "intersection, not a union"},
		{`statusCategory = Done AND statusCategory = "To Do"`, "zero rows"},
	}
	for _, tc := range cases {
		res := Parse(tc.jql, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", tc.jql, res.Error, res.Message)
		}
		blob := strings.Join(res.Unsupported, " ")
		if !strings.Contains(blob, tc.msg) {
			t.Errorf("%q must be refused out loud, got unsupported=%q applied=%v", tc.jql, blob, res.Applied)
		}
		if len(res.Filters.AssigneeEmail) > 0 || len(res.Filters.ReporterEmail) > 0 ||
			len(res.Filters.Keys) > 0 || len(res.Filters.Parent) > 0 ||
			len(res.Filters.SprintIDs) > 0 || len(res.Filters.StatusCategory) > 1 {
			t.Errorf("%q left a widened include applied: %+v", tc.jql, res.Filters)
		}
	}

	// statusCategory two includes intersect exactly when they overlap.
	res := Parse(`statusCategory in (todo, done) AND statusCategory in (inprogress, done)`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if got := strings.Join(res.Filters.StatusCategory, ","); got != "done" {
		t.Errorf("intersection of {new,done} and {inprogress,done} is done, got [%s]", got)
	}
}
