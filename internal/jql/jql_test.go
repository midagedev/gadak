package jql

import (
	"encoding/json"
	"net/url"
	"strconv"
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

// G1 FAIL-first: key IN must be a Keys axis, not a stuffed text needle.
// On HEAD this fails: Match is empty (needle is "NMA-1 NMA-2") and Emit
// writes text ~ "NMA-1 NMA-2". The fix populates Filter.Keys.
func TestKeyInMatchAndEmit(t *testing.T) {
	res := Parse(`key in (NMA-1, NMA-2)`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if res.Filters.Q != "" {
		t.Fatalf("key IN stuffed q: %q", res.Filters.Q)
	}
	if got := res.Filters.Keys; len(got) != 2 || got[0] != "NMA-1" || got[1] != "NMA-2" {
		t.Fatalf("Keys %+v", got)
	}
	a := Issue{Key: "NMA-1", Project: "NMA"}
	b := Issue{Key: "nma-2", Project: "NMA"}
	miss := Issue{Key: "NMA-3", Project: "NMA"}
	if !Match(a, res.Filters) {
		t.Error("NMA-1 should match key in (NMA-1, NMA-2)")
	}
	if !Match(b, res.Filters) {
		t.Error("nma-2 should match key in (NMA-1, NMA-2) case-insensitively")
	}
	if Match(miss, res.Filters) {
		t.Error("NMA-3 should not match")
	}
	emitted, omitted := Emit(res.Filters, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if strings.Contains(emitted, "text ~") {
		t.Errorf("emit lost the key axis: %q", emitted)
	}
	if !strings.Contains(emitted, `key in (`) ||
		!strings.Contains(emitted, "NMA-1") ||
		!strings.Contains(emitted, "NMA-2") {
		t.Errorf("emit %q", emitted)
	}
	h := Hash(res.Filters, Display{})
	if !strings.Contains(h, "ks=NMA-1,NMA-2") {
		t.Errorf("hash %q", h)
	}
	if QueryURL(h) != "#/?"+h || HashURL(res.Filters, Display{}) != "#/?"+h {
		t.Errorf("HashURL %q", HashURL(res.Filters, Display{}))
	}
}

func TestKeyEqualsAndAliases(t *testing.T) {
	for _, q := range []string{
		`key = nma-1`,
		`issuekey = NMA-1`,
		`issue = NMA-1`,
		`key = "nma-1"`,
	} {
		res := Parse(q, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", q, res.Error, res.Message)
		}
		if got := res.Filters.Keys; len(got) != 1 || got[0] != "NMA-1" {
			t.Fatalf("%s → Keys %+v", q, got)
		}
		if res.Filters.Q != "" {
			t.Fatalf("%s stuffed q: %q", q, res.Filters.Q)
		}
		emitted, _ := Emit(res.Filters, Display{}, EmitOpts{})
		if emitted != `key = "NMA-1"` {
			t.Fatalf("%s emit %q", q, emitted)
		}
	}
}

func TestKeyOrderPreservedAndDeduped(t *testing.T) {
	res := Parse(`key in (NMA-2, nma-1, NMA-2, NMA-1)`, fixedOpts())
	if got := res.Filters.Keys; len(got) != 2 || got[0] != "NMA-2" || got[1] != "NMA-1" {
		t.Fatalf("Keys %+v", got)
	}
	h := Hash(res.Filters, Display{})
	if !strings.Contains(h, "ks=NMA-2,NMA-1") {
		t.Fatalf("hash %q", h)
	}
}

func TestKeyLimit(t *testing.T) {
	vals := make([]string, MaxKeys+1)
	for i := range vals {
		vals[i] = "NMA-" + strconv.Itoa(i+1)
	}
	res := Parse("key in ("+strings.Join(vals, ", ")+")", fixedOpts())
	if res.Error != ErrTooManyKeys {
		t.Fatalf("error %q want %q (%s)", res.Error, ErrTooManyKeys, res.Message)
	}
	if !strings.Contains(res.Message, "501") || !strings.Contains(res.Message, "500") {
		t.Fatalf("message %q", res.Message)
	}
	if err := CheckKeyLimit(MaxKeys + 1); err == nil || !strings.Contains(err.Error(), "501") {
		t.Fatalf("CheckKeyLimit %v", err)
	}
	if err := CheckKeyLimit(MaxKeys); err != nil {
		t.Fatalf("CheckKeyLimit(%d) %v", MaxKeys, err)
	}
}

func TestSplitKeysCommaAndSpace(t *testing.T) {
	got := SplitKeys("NMA-1, nma-2  NMA-3\nNMA-1")
	if len(got) != 3 || got[0] != "NMA-1" || got[1] != "NMA-2" || got[2] != "NMA-3" {
		t.Fatalf("%v", got)
	}
}

func TestEmptyFilterKeysMarshal(t *testing.T) {
	b, err := json.Marshal(EmptyFilter())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"keys":[]`) {
		t.Fatalf("empty Keys must marshal as []: %s", b)
	}
}

func TestMatchKeysEmptyIsUnconstrained(t *testing.T) {
	it := Issue{Key: "NMA-1"}
	if !Match(it, EmptyFilter()) {
		t.Fatal("empty Keys must not constrain")
	}
}
