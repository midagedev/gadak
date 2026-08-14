package jql

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func fixedOpts() Opts {
	return Opts{
		Now:   time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC),
		Email: "dana@example.com",
	}
}

func TestExtractURL(t *testing.T) {
	ex := Extract(`https://example.atlassian.net/issues/?jql=project%20%3D%20NMA%20AND%20statusCategory%20%3D%20%22In%20Progress%22`)
	if !ex.IsURL {
		t.Fatal("expected URL")
	}
	if !strings.Contains(ex.JQL, "project = NMA") || !strings.Contains(ex.JQL, "statusCategory") {
		t.Fatalf("jql %q", ex.JQL)
	}

	ex = Extract(`https://example.atlassian.net/issues/?filter=12345`)
	if ex.FilterID != "12345" || ex.JQL != "" {
		t.Fatalf("filter id %+v", ex)
	}

	ex = Extract(`jql=project%20%3D%20NMA`)
	if ex.JQL != "project = NMA" {
		t.Fatalf("bare jql= %q", ex.JQL)
	}
}

func TestLooksLike(t *testing.T) {
	yes := []string{
		`project = NMA`,
		`statusCategory = "In Progress"`,
		`assignee = currentUser()`,
		`https://x.atlassian.net/issues/?jql=project%20%3D%20NMA`,
		`ORDER BY updated DESC`,
	}
	for _, s := range yes {
		if !LooksLike(s) {
			t.Errorf("LooksLike(%q) = false", s)
		}
	}
	no := []string{"idempotency", "retry AND webhook", "flaky upload", ""}
	for _, s := range no {
		if LooksLike(s) {
			t.Errorf("LooksLike(%q) = true (should stay FTS)", s)
		}
	}
}

func TestParseCommonDashboard(t *testing.T) {
	res := Parse(`project = NMA AND statusCategory = "In Progress" AND assignee = currentUser() ORDER BY updated DESC`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("error %s: %s", res.Error, res.Message)
	}
	if got := res.Filters.JiraProject; len(got) != 1 || got[0] != "NMA" {
		t.Fatalf("project %+v", got)
	}
	if got := res.Filters.StatusCategory; len(got) != 1 || got[0] != "inprogress" {
		t.Fatalf("category %+v", got)
	}
	if got := res.Filters.AssigneeEmail; len(got) != 1 || got[0] != "currentUser()" {
		t.Fatalf("assignee %+v", got)
	}
	if res.Display.Sort != "updated" || res.Display.Dir != "desc" {
		t.Fatalf("order %+v", res.Display)
	}
	if len(res.Unsupported) != 0 {
		t.Fatalf("unsupported %v", res.Unsupported)
	}

	ResolvePeople(&res, nil, "dana@example.com")
	if got := res.Filters.AssigneeEmail; len(got) != 1 || got[0] != "dana@example.com" {
		t.Fatalf("resolved assignee %+v", got)
	}

	jql, omitted := Emit(res.Filters, res.Display, EmitOpts{Email: "dana@example.com"})
	if omitted != nil {
		t.Fatalf("omitted %v", omitted)
	}
	if !strings.Contains(jql, `project = NMA`) ||
		!strings.Contains(jql, `statusCategory = "In Progress"`) ||
		!strings.Contains(jql, `assignee = currentUser()`) ||
		!strings.Contains(jql, `ORDER BY updated DESC`) {
		t.Fatalf("emit %q", jql)
	}
}

func TestParseINAndDatesAndFlags(t *testing.T) {
	res := Parse(`project in (NMA, NMB) AND labels in (backend, api) AND created >= -7d AND assignee is EMPTY`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Filters.JiraProject) != 2 {
		t.Fatalf("projects %+v", res.Filters.JiraProject)
	}
	if len(res.Filters.Labels) != 2 {
		t.Fatalf("labels %+v", res.Filters.Labels)
	}
	if res.Filters.CreatedFrom == nil || *res.Filters.CreatedFrom != "2026-08-07" {
		t.Fatalf("created_from %+v", res.Filters.CreatedFrom)
	}
	if !res.Filters.Unassigned {
		t.Fatal("expected unassigned")
	}
}

func TestParseResolutionAndText(t *testing.T) {
	res := Parse(`resolution is EMPTY AND text ~ "webhook"`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if got := strings.Join(res.Filters.StatusCategory, ","); got != "new,inprogress" && got != "inprogress,new" {
		// mergeUnique preserves order of first insert
		if len(res.Filters.StatusCategory) != 2 {
			t.Fatalf("resolution → %+v", res.Filters.StatusCategory)
		}
	}
	if res.Filters.Q != "webhook" {
		t.Fatalf("q %q", res.Filters.Q)
	}
}

func TestParseSameFieldOR(t *testing.T) {
	res := Parse(`status = "In Progress" OR status = Done`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Filters.Status) != 2 {
		t.Fatalf("status %+v", res.Filters.Status)
	}
}

func TestUnsupportedAreListed(t *testing.T) {
	cases := []struct {
		jql  string
		want string
	}{
		{`project = NMA AND sprint = 12`, "sprint"},
		{`status = A OR assignee = B`, "OR across"},
		{`labels = a AND labels = b`, "has-all"},
		{`status != Done`, "only ="},
		{`status WAS Done`, "parse"},
		{`assignee in membersOf("x")`, "membersOf"},
		{`https://x.atlassian.net/issues/?filter=99`, "filter"},
	}
	for _, tc := range cases {
		res := Parse(tc.jql, fixedOpts())
		blob := res.Error + " " + res.Message + " " + strings.Join(res.Unsupported, " | ")
		if !strings.Contains(strings.ToLower(blob), strings.ToLower(tc.want)) {
			t.Errorf("%q → %q, want substring %q", tc.jql, blob, tc.want)
		}
		// A supported clause next to an unsupported one must still apply.
		if strings.Contains(tc.jql, "project = NMA") && tc.want == "sprint" {
			if len(res.Filters.JiraProject) != 1 || res.Filters.JiraProject[0] != "NMA" {
				t.Errorf("project dropped alongside sprint: %+v", res.Filters.JiraProject)
			}
		}
	}
}

func TestFilterIDURL(t *testing.T) {
	res := Parse(`https://x.atlassian.net/issues/?filter=12345`, fixedOpts())
	if res.Error != ErrFilterID {
		t.Fatalf("error %q", res.Error)
	}
}

func TestMatch(t *testing.T) {
	it := Issue{
		Key: "NMA-1", Project: "NMA", Status: "In Progress", StatusCategory: "inprogress",
		Type: "Bug", Priority: "High", Assignee: "Dana", AssigneeEmail: "dana@example.com",
		Labels: []string{"backend"}, CreatedAt: "2026-08-10T00:00:00Z", UpdatedAt: "2026-08-12T00:00:00Z",
	}
	res := Parse(`project = NMA AND statusCategory = "In Progress" AND labels = backend`, fixedOpts())
	if !Match(it, res.Filters) {
		t.Fatal("expected match")
	}
	other := it
	other.Project = "NMB"
	if Match(other, res.Filters) {
		t.Fatal("NMB should not match project = NMA")
	}
}

func TestRoundTripEmitParse(t *testing.T) {
	f := EmptyFilter()
	f.JiraProject = []string{"NMA"}
	f.StatusCategory = []string{"inprogress"}
	f.Labels = []string{"backend", "api"}
	from := "2026-08-01"
	f.CreatedFrom = &from
	jql, omitted := Emit(f, Display{Sort: "updated", Dir: "desc"}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	res := Parse(jql, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if got := res.Filters.JiraProject; len(got) != 1 || got[0] != "NMA" {
		t.Fatalf("project %+v", got)
	}
	if got := res.Filters.StatusCategory; len(got) != 1 || got[0] != "inprogress" {
		t.Fatalf("cat %+v", got)
	}
	if len(res.Filters.Labels) != 2 {
		t.Fatalf("labels %+v", res.Filters.Labels)
	}
	if res.Filters.CreatedFrom == nil || *res.Filters.CreatedFrom != "2026-08-01" {
		t.Fatalf("created %+v", res.Filters.CreatedFrom)
	}
}

func TestEmitOmitsGadakOnly(t *testing.T) {
	f := EmptyFilter()
	f.JiraProject = []string{"NMA"}
	f.Reopened = true
	f.Stale = true
	jql, omitted := Emit(f, Display{}, EmitOpts{})
	if !strings.Contains(jql, "project = NMA") || strings.Contains(strings.ToLower(jql), "reopen") {
		t.Fatalf("jql %q", jql)
	}
	if strings.Join(omitted, ",") != "reopened,stale" {
		t.Fatalf("omitted %v", omitted)
	}
}

func TestStartOfDayRelative(t *testing.T) {
	res := Parse(`updated >= startOfDay(-7d)`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if res.Filters.UpdatedFrom == nil || *res.Filters.UpdatedFrom != "2026-08-07" {
		t.Fatalf("updated_from %+v", res.Filters.UpdatedFrom)
	}
}

func TestImplicitAND(t *testing.T) {
	res := Parse(`project = NMA status = Open`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Filters.JiraProject) != 1 || len(res.Filters.Status) != 1 {
		t.Fatalf("implicit AND %+v / %+v", res.Filters.JiraProject, res.Filters.Status)
	}
}

func TestOrderByCustomFieldKeepsClauses(t *testing.T) {
	// Board filters ship as `project = X ORDER BY cf[10019] ASC` (Rank).
	res := Parse(`project = NMA ORDER BY cf[10019] ASC`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Filters.JiraProject) != 1 || res.Filters.JiraProject[0] != "NMA" {
		t.Fatalf("project dropped: %+v", res.Filters.JiraProject)
	}
	if len(res.Unsupported) == 0 {
		t.Fatal("expected ORDER BY cf[10019] to be listed")
	}
}

func TestHashEncodesViewParams(t *testing.T) {
	f := EmptyFilter()
	f.JiraProject = []string{"NMA"}
	f.StatusCategory = []string{"inprogress"}
	h := Hash(f, Display{Sort: "updated", Dir: "desc", GroupBy: "status_category"})
	q, err := url.ParseQuery(h)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("pj") != "NMA" || q.Get("sc") != "inprogress" {
		t.Fatalf("hash %q", h)
	}
	if q.Has("s") || q.Has("d") || q.Has("g") {
		t.Fatalf("defaults should be omitted: %q", h)
	}
}

func TestStatusCategoryId(t *testing.T) {
	res := Parse(`statusCategory = 4`, fixedOpts())
	if len(res.Filters.StatusCategory) != 1 || res.Filters.StatusCategory[0] != "inprogress" {
		t.Fatalf("%+v (%s)", res.Filters.StatusCategory, res.Message)
	}
}
