package jql

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/calendar"
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
		{`project = NMA AND sprint = "Sprint 41"`, "sprint"},
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

func TestStatusCategoryJiraKeyAndGadakToken(t *testing.T) {
	for _, q := range []string{
		`statusCategory = "indeterminate"`,
		`statusCategory = inprogress`,
	} {
		res := Parse(q, fixedOpts())
		if len(res.Filters.StatusCategory) != 1 || res.Filters.StatusCategory[0] != "inprogress" {
			t.Fatalf("%s → %+v (%s)", q, res.Filters.StatusCategory, res.Message)
		}
	}
	it := Issue{Key: "NMA-1", StatusCategory: "indeterminate"}
	f := EmptyFilter()
	f.StatusCategory = []string{"inprogress"}
	if !Match(it, f) {
		t.Fatal("stored Jira key indeterminate must match filter inprogress")
	}
	jql, omitted := Emit(f, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if !strings.Contains(jql, `statusCategory = "In Progress"`) {
		t.Fatalf("emit %q", jql)
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
	if QueryURL(h) != "#/?"+h {
		t.Errorf("QueryURL %q", QueryURL(h))
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

// GDK-521 FAIL-first: parent = / IN must compile into the subset (not
// skip as "not in the subset"). This assertion compiles against HEAD
// because it only inspects Unsupported.
func TestParentEqualsIsSupported(t *testing.T) {
	for _, q := range []string{
		`parent = NMB-196`,
		`parent in (NMB-196, NMB-197)`,
		`parent = nmb-196`,
	} {
		res := Parse(q, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", q, res.Error, res.Message)
		}
		if len(res.Unsupported) != 0 {
			t.Fatalf("%s Unsupported %v", q, res.Unsupported)
		}
	}
}

func TestParentEqualsAndIn(t *testing.T) {
	res := Parse(`parent = NMB-196`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Unsupported) != 0 {
		t.Fatalf("Unsupported %v", res.Unsupported)
	}
	if got := res.Filters.Parent; len(got) != 1 || got[0] != "NMB-196" {
		t.Fatalf("Parent %+v", got)
	}

	res = Parse(`parent in (NMB-196, NMB-197)`, fixedOpts())
	if len(res.Unsupported) != 0 {
		t.Fatalf("Unsupported %v", res.Unsupported)
	}
	if got := res.Filters.Parent; len(got) != 2 || got[0] != "NMB-196" || got[1] != "NMB-197" {
		t.Fatalf("Parent IN %+v", got)
	}

	res = Parse(`parent = nmb-196`, fixedOpts())
	if got := res.Filters.Parent; len(got) != 1 || got[0] != "NMB-196" {
		t.Fatalf("lowercase Parent %+v", got)
	}
}

func TestParentNeqStaysUnsupported(t *testing.T) {
	for _, q := range []string{
		`parent != NMB-196`,
		`parent ~ NMB-196`,
	} {
		res := Parse(q, fixedOpts())
		if len(res.Unsupported) == 0 {
			t.Fatalf("%s must stay unsupported", q)
		}
		blob := strings.Join(res.Unsupported, " ")
		if !strings.Contains(blob, "not in the subset") && !strings.Contains(blob, "only = and IN") {
			t.Fatalf("%s unsupported %q", q, blob)
		}
		if len(res.Filters.Parent) != 0 {
			t.Fatalf("%s must not populate Parent: %+v", q, res.Filters.Parent)
		}
	}
}

func TestParentMatchEmitHash(t *testing.T) {
	res := Parse(`parent in (NMB-196, NMB-197)`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	hit := Issue{Key: "NMB-1", ParentKey: "NMB-196"}
	hitFold := Issue{Key: "NMB-2", ParentKey: "nmb-197"}
	miss := Issue{Key: "NMB-3", ParentKey: "NMB-1"}
	none := Issue{Key: "NMB-4", ParentKey: ""}
	if !Match(hit, res.Filters) {
		t.Error("NMB-196 parent should match")
	}
	if !Match(hitFold, res.Filters) {
		t.Error("nmb-197 parent should match case-insensitively")
	}
	if Match(miss, res.Filters) {
		t.Error("other parent should not match")
	}
	if Match(none, res.Filters) {
		t.Error("empty parent should not match")
	}
	emitted, omitted := Emit(res.Filters, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if !strings.Contains(emitted, "parent in (") ||
		!strings.Contains(emitted, "NMB-196") ||
		!strings.Contains(emitted, "NMB-197") {
		t.Errorf("emit %q", emitted)
	}
	h := Hash(res.Filters, Display{})
	if !strings.Contains(h, "pk=NMB-196,NMB-197") {
		t.Errorf("hash %q", h)
	}
}

func TestEmptyFilterParentMarshal(t *testing.T) {
	b, err := json.Marshal(EmptyFilter())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"parent":[]`) {
		t.Fatalf("empty Parent must marshal as []: %s", b)
	}
}

func TestMatchParentEmptyIsUnconstrained(t *testing.T) {
	it := Issue{Key: "NMB-1", ParentKey: "NMB-196"}
	if !Match(it, EmptyFilter()) {
		t.Fatal("empty Parent must not constrain")
	}
}

// I1 FAIL-first: an email-hidden roster row must resolve by name/account id
// to AccountID, then Match the issue that only carries that id.
func TestI1EmailHiddenNameResolvesToAccountID(t *testing.T) {
	people := []Person{{AccountID: "acc-me", Name: "Me", DisplayName: "Me User", Email: ""}}
	res := Parse(`assignee = Me`, Opts{})
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	ResolvePeople(&res, people, "")
	if got := res.Filters.AssigneeEmail; len(got) != 1 || got[0] != "acc-me" {
		t.Fatalf("resolved assignee %+v unsupported %v", got, res.Unsupported)
	}
	it := Issue{AssigneeID: "acc-me", AssigneeEmail: ""}
	if !Match(it, res.Filters) {
		t.Fatal("email-hidden assignee should match via account id")
	}
}

// GDK-250: a 01:00 KST create is stored as 2026-08-17T16:00:00.000Z.
// FAIL-first on the UTC-prefix inRange (red captured 2026-08-18). Timezone
// is pinned to Asia/Seoul so a UTC CI runner cannot hide the miss.
func TestInRangeKSTCreatedFrom(t *testing.T) {
	seoul, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	z := calendar.In(seoul)
	from := "2026-08-18"
	it := Issue{CreatedAt: "2026-08-17T16:00:00.000Z"}
	f := EmptyFilter()
	f.CreatedFrom = &from
	if !MatchIn(it, f, z) {
		t.Fatal("2026-08-18 01:00 KST stored as 2026-08-17T16:00:00.000Z must match created_from=2026-08-18")
	}
	if MatchIn(it, f, calendar.UTC()) {
		t.Fatal("UTC calendar day is the 17th; must not match from=18")
	}
}

// GDK-518 FAIL-first: sprint = / IN numeric ids and sprint in openSprints()
// must compile into the subset (not skip as "not in the subset"). This
// assertion compiles against HEAD because it only inspects Unsupported /
// Applied.
func TestSprintEqualsIsSupported(t *testing.T) {
	for _, q := range []string{
		`sprint = 12`,
		`sprint in (12, 13)`,
		`sprint in openSprints()`,
	} {
		res := Parse(q, fixedOpts())
		if res.Error != "" {
			t.Fatalf("%s: %s: %s", q, res.Error, res.Message)
		}
		if len(res.Unsupported) != 0 {
			t.Fatalf("%s Unsupported %v", q, res.Unsupported)
		}
		applied := strings.Join(res.Applied, " ")
		if !strings.Contains(applied, "sprint") {
			t.Fatalf("%s Applied %v, want sprint", q, res.Applied)
		}
	}
}

func TestSprintEqualsAndInAndOpenSprints(t *testing.T) {
	res := Parse(`sprint = 12`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if got := res.Filters.SprintIDs; len(got) != 1 || got[0] != "12" {
		t.Fatalf("SprintIDs %+v", got)
	}
	if len(res.Filters.SprintState) != 0 {
		t.Fatalf("SprintState %+v, want empty", res.Filters.SprintState)
	}

	res = Parse(`sprint in (12, 13)`, fixedOpts())
	if len(res.Unsupported) != 0 {
		t.Fatalf("Unsupported %v", res.Unsupported)
	}
	if got := res.Filters.SprintIDs; len(got) != 2 || got[0] != "12" || got[1] != "13" {
		t.Fatalf("SprintIDs IN %+v", got)
	}

	res = Parse(`sprint in openSprints()`, fixedOpts())
	if len(res.Unsupported) != 0 {
		t.Fatalf("openSprints Unsupported %v", res.Unsupported)
	}
	if len(res.Filters.SprintIDs) != 0 {
		t.Fatalf("openSprints must not set SprintIDs: %+v", res.Filters.SprintIDs)
	}
	if got := res.Filters.SprintState; len(got) != 1 || got[0] != "active" {
		t.Fatalf("SprintState %+v, want [active]", got)
	}
}

func TestSprintNameAndClosedStayUnsupported(t *testing.T) {
	for _, q := range []string{
		`sprint = "Sprint 41"`,
		`sprint in closedSprints()`,
		`sprint != 12`,
	} {
		res := Parse(q, fixedOpts())
		if len(res.Unsupported) == 0 {
			t.Fatalf("%s must stay unsupported", q)
		}
		if len(res.Filters.SprintIDs) != 0 || len(res.Filters.SprintState) != 0 {
			t.Fatalf("%s must not populate sprint filters: ids=%+v state=%+v", q, res.Filters.SprintIDs, res.Filters.SprintState)
		}
	}
}

func TestSprintMatchEmitHash(t *testing.T) {
	res := Parse(`sprint = 12`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	hit := Issue{Key: "NMB-1", SprintID: "12"}
	miss := Issue{Key: "NMB-2", SprintID: "13"}
	none := Issue{Key: "NMB-3"}
	if !Match(hit, res.Filters) {
		t.Error("sprint_id 12 should match sprint = 12")
	}
	if Match(miss, res.Filters) {
		t.Error("sprint_id 13 should not match sprint = 12")
	}
	if Match(none, res.Filters) {
		t.Error("empty sprint_id should not match sprint = 12")
	}
	emitted, omitted := Emit(res.Filters, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if !strings.Contains(emitted, "sprint") || !strings.Contains(emitted, "12") {
		t.Errorf("emit %q", emitted)
	}
	h := Hash(res.Filters, Display{})
	if !strings.Contains(h, "sid=12") {
		t.Errorf("hash %q", h)
	}

	open := Parse(`sprint in openSprints()`, fixedOpts())
	active := Issue{Key: "NMB-4", SprintState: "active"}
	closed := Issue{Key: "NMB-5", SprintState: "closed"}
	if !Match(active, open.Filters) {
		t.Error("sprint_state=active should match openSprints()")
	}
	if Match(closed, open.Filters) {
		t.Error("sprint_state=closed should not match openSprints()")
	}
	emitted, _ = Emit(open.Filters, Display{}, EmitOpts{})
	if emitted != "sprint in openSprints()" {
		t.Errorf("openSprints emit %q", emitted)
	}
}

func TestEmptyFilterSprintMarshal(t *testing.T) {
	b, err := json.Marshal(EmptyFilter())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"sprint_ids":[]`) {
		t.Fatalf("empty SprintIDs must marshal as []: %s", b)
	}
}

func TestMatchSprintEmptyIsUnconstrained(t *testing.T) {
	it := Issue{Key: "NMB-1", SprintID: "12", SprintState: "active"}
	if !Match(it, EmptyFilter()) {
		t.Fatal("empty sprint filters must not constrain")
	}
}

func TestParseDueAndResolved(t *testing.T) {
	res := Parse(`duedate >= "2026-08-20" AND resolved >= "2026-08-01"`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if res.Filters.DueFrom == nil || *res.Filters.DueFrom != "2026-08-20" {
		t.Fatalf("due_from %+v", res.Filters.DueFrom)
	}
	if res.Filters.ResolvedFrom == nil || *res.Filters.ResolvedFrom != "2026-08-01" {
		t.Fatalf("resolved_from %+v", res.Filters.ResolvedFrom)
	}
	dueOnly := EmptyFilter()
	dueOnly.DueFrom = res.Filters.DueFrom
	if !Match(Issue{Duedate: "2026-08-20"}, dueOnly) {
		t.Fatal("duedate 2026-08-20 must match due_from=2026-08-20")
	}
	if Match(Issue{Duedate: "2026-08-19"}, dueOnly) {
		t.Fatal("duedate 2026-08-19 must miss due_from=2026-08-20")
	}
}

func TestHashDueResolvedParams(t *testing.T) {
	f := EmptyFilter()
	from := "2026-08-20"
	f.DueFrom = &from
	resFrom := "2026-08-17"
	f.ResolvedFrom = &resFrom
	h := Hash(f, Display{})
	q, err := url.ParseQuery(h)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("df") != "2026-08-20" || q.Get("rf") != "2026-08-17" {
		t.Fatalf("hash %q", h)
	}
}

// I1 FAIL-first: currentUser() + configured account id must match an
// issue that has AssigneeID and no email.
func TestI1CurrentUserAccountIDMatchesHiddenAssignee(t *testing.T) {
	res := Parse(`assignee = currentUser()`, Opts{})
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	ResolveIdentity(&res, nil, Identity{AccountID: "acc-me"})
	if got := res.Filters.AssigneeEmail; len(got) != 1 || got[0] != "acc-me" {
		t.Fatalf("currentUser resolved %+v unsupported %v", got, res.Unsupported)
	}
	it := Issue{AssigneeID: "acc-me", AssigneeEmail: ""}
	if !Match(it, res.Filters) {
		t.Fatal("currentUser() via account id should match AssigneeID")
	}
}

// I1 reporter symmetry: email-hidden reporter matches on ReporterID.
func TestI1CurrentUserAccountIDMatchesHiddenReporter(t *testing.T) {
	res := Parse(`reporter = currentUser()`, Opts{})
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	ResolveIdentity(&res, nil, Identity{AccountID: "acc-me"})
	if got := res.Filters.ReporterEmail; len(got) != 1 || got[0] != "acc-me" {
		t.Fatalf("currentUser reporter resolved %+v unsupported %v", got, res.Unsupported)
	}
	it := Issue{ReporterID: "acc-me", ReporterEmail: ""}
	if !Match(it, res.Filters) {
		t.Fatal("currentUser() via account id should match ReporterID")
	}
}

// I2 FAIL-first: assignee is EMPTY must not treat an id-only assignee as empty.
func TestI2UnassignedIgnoresIDOnlyAssignee(t *testing.T) {
	it := Issue{AssigneeID: "acc-1", AssigneeEmail: ""}
	if Match(it, Filter{Unassigned: true}) {
		t.Fatal("id-only assignee must not match assignee is EMPTY")
	}
	bare := Issue{}
	if !Match(bare, Filter{Unassigned: true}) {
		t.Fatal("no id/email/name should match unassigned")
	}
}

// Legacy fallback: a roster row with email and no account id still resolves.
func TestLegacyEmailOnlyPersonStillMatches(t *testing.T) {
	people := []Person{{Email: "old@example.com", Name: "Old", AccountID: ""}}
	res := Parse(`assignee = Old`, Opts{})
	ResolvePeople(&res, people, "")
	if got := res.Filters.AssigneeEmail; len(got) != 1 || got[0] != "old@example.com" {
		t.Fatalf("legacy email %+v unsupported %v", got, res.Unsupported)
	}
	if !Match(Issue{AssigneeEmail: "old@example.com"}, res.Filters) {
		t.Fatal("email-only person should still match")
	}
}

// Ambiguous same-name people stay unsupported after id-bodied resolve.
func TestAmbiguousNameStaysUnsupported(t *testing.T) {
	people := []Person{
		{AccountID: "acc-1", Name: "Kim", Email: ""},
		{AccountID: "acc-2", Name: "Kim", Email: ""},
	}
	res := Parse(`assignee = Kim`, Opts{})
	ResolvePeople(&res, people, "")
	if len(res.Filters.AssigneeEmail) != 0 {
		t.Fatalf("ambiguous should not apply: %+v", res.Filters.AssigneeEmail)
	}
	blob := strings.Join(res.Unsupported, " ")
	if !strings.Contains(blob, "ambiguous") {
		t.Fatalf("want ambiguous, got %q", blob)
	}
}

func TestPeopleFromIssuesCollectsReporterID(t *testing.T) {
	people := PeopleFromIssues([]Issue{{
		Reporter: "Rep", ReporterEmail: "", ReporterID: "acc-rp",
	}})
	if len(people) != 1 || people[0].AccountID != "acc-rp" {
		t.Fatalf("reporter roster %+v", people)
	}
}

func TestResolveUnsupportedReasons(t *testing.T) {
	res := Parse(`assignee = currentUser()`, Opts{})
	ResolveIdentity(&res, nil, Identity{})
	blob := strings.Join(res.Unsupported, " ")
	if !strings.Contains(blob, "(no account email — set one on a connected workspace, or drop currentUser())") {
		t.Fatalf("empty identity: %q", blob)
	}
	if strings.Contains(blob, "이메일") {
		t.Fatalf("Korean leaked into English CLI: %q", blob)
	}

	res = Parse(`assignee = Nobody`, Opts{})
	ResolvePeople(&res, nil, "")
	blob = strings.Join(res.Unsupported, " ")
	if !strings.Contains(blob, "(not in the mirror)") {
		t.Fatalf("missing person: %q", blob)
	}
	if strings.Contains(blob, "미러") {
		t.Fatalf("Korean leaked into English CLI: %q", blob)
	}
}

// GDK-441 FAIL-first: web ViewFilters.jira_project_not is a JSON key the Go
// Filter struct did not declare, so encoding/json dropped it and Emit wrote
// a wider JQL with no NOT clause and nothing in omitted.
func TestEmitJiraProjectNot(t *testing.T) {
	var f Filter
	if err := json.Unmarshal([]byte(`{"jira_project_not":["XYZ"]}`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.JiraProjectNot) != 1 || f.JiraProjectNot[0] != "XYZ" {
		t.Fatalf("unmarshal JiraProjectNot %+v", f.JiraProjectNot)
	}
	got, omitted := Emit(f, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if got != "project not in (XYZ)" {
		t.Fatalf("silent drop of jira_project_not: jql=%q omitted=%v", got, omitted)
	}
}

func TestEmitJiraProjectIncludeAndNot(t *testing.T) {
	var f Filter
	if err := json.Unmarshal([]byte(`{"jira_project":["NMA"],"jira_project_not":["XYZ"]}`), &f); err != nil {
		t.Fatal(err)
	}
	got, omitted := Emit(f, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if got != "project = NMA AND project not in (XYZ)" {
		t.Fatalf("jql %q", got)
	}
}

func TestEmitSourceProjectNotOmitted(t *testing.T) {
	var f Filter
	if err := json.Unmarshal([]byte(`{"source_project_not":["scratch"]}`), &f); err != nil {
		t.Fatal(err)
	}
	got, omitted := Emit(f, Display{}, EmitOpts{})
	if strings.Contains(strings.ToLower(got), "source") {
		t.Fatalf("source_project_not is not JQL: %q", got)
	}
	found := false
	for _, o := range omitted {
		if o == "source_project_not" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("source_project_not must be listed in omitted, got jql=%q omitted=%v", got, omitted)
	}
}

func TestUnknownJSONKeysStayHarmless(t *testing.T) {
	var f Filter
	if err := json.Unmarshal([]byte(`{"jira_project":["NMA"],"not_a_real_axis":["x"]}`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.JiraProject) != 1 || f.JiraProject[0] != "NMA" {
		t.Fatalf("project %+v", f.JiraProject)
	}
	got, omitted := Emit(f, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if got != "project = NMA" {
		t.Fatalf("jql %q", got)
	}
}

func TestEmptyFilterProjectNotMarshal(t *testing.T) {
	b, err := json.Marshal(EmptyFilter())
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"jira_project_not":[]`) || !strings.Contains(s, `"source_project_not":[]`) {
		t.Fatalf("empty not slices must marshal as []: %s", b)
	}
}

func TestEmitJiraProjectNotQuotesAndMany(t *testing.T) {
	var f Filter
	if err := json.Unmarshal([]byte(`{"jira_project_not":["XYZ","NMB","XY Z"]}`), &f); err != nil {
		t.Fatal(err)
	}
	got, omitted := Emit(f, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	if got != `project not in (XYZ, NMB, "XY Z")` {
		t.Fatalf("jql %q", got)
	}
}

func TestParseProjectNotIn(t *testing.T) {
	res := Parse(`project not in (XYZ, NMB)`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Unsupported) != 0 {
		t.Fatalf("unsupported %v", res.Unsupported)
	}
	if got := res.Filters.JiraProjectNot; len(got) != 2 || got[0] != "XYZ" || got[1] != "NMB" {
		t.Fatalf("JiraProjectNot %+v", got)
	}
	if len(res.Filters.JiraProject) != 0 {
		t.Fatalf("must not apply as include: %+v", res.Filters.JiraProject)
	}
}

func TestParseProjectIncludeAndNot(t *testing.T) {
	res := Parse(`project = NMA AND project not in (XYZ)`, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Unsupported) != 0 {
		t.Fatalf("unsupported %v", res.Unsupported)
	}
	if got := res.Filters.JiraProject; len(got) != 1 || got[0] != "NMA" {
		t.Fatalf("JiraProject %+v", got)
	}
	if got := res.Filters.JiraProjectNot; len(got) != 1 || got[0] != "XYZ" {
		t.Fatalf("JiraProjectNot %+v", got)
	}
}

func TestRoundTripProjectNotIn(t *testing.T) {
	var f Filter
	if err := json.Unmarshal([]byte(`{"jira_project":["NMA"],"jira_project_not":["XYZ"]}`), &f); err != nil {
		t.Fatal(err)
	}
	jql, omitted := Emit(f, Display{}, EmitOpts{})
	if len(omitted) != 0 {
		t.Fatalf("omitted %v", omitted)
	}
	res := Parse(jql, fixedOpts())
	if res.Error != "" {
		t.Fatalf("%s: %s", res.Error, res.Message)
	}
	if len(res.Unsupported) != 0 {
		t.Fatalf("unsupported %v", res.Unsupported)
	}
	if got := res.Filters.JiraProject; len(got) != 1 || got[0] != "NMA" {
		t.Fatalf("JiraProject %+v", got)
	}
	if got := res.Filters.JiraProjectNot; len(got) != 1 || got[0] != "XYZ" {
		t.Fatalf("JiraProjectNot %+v", got)
	}
}

func TestParseProjectNeqStaysUnsupported(t *testing.T) {
	res := Parse(`project != XYZ`, fixedOpts())
	blob := res.Error + " " + res.Message + " " + strings.Join(res.Unsupported, " | ")
	if !strings.Contains(blob, "only =") {
		t.Fatalf("want != listed as unsupported, got %q", blob)
	}
	if len(res.Filters.JiraProject) != 0 || len(res.Filters.JiraProjectNot) != 0 {
		t.Fatalf("!= must not apply: include=%+v not=%+v", res.Filters.JiraProject, res.Filters.JiraProjectNot)
	}
}

func TestParseStatusNotInStaysUnsupported(t *testing.T) {
	res := Parse(`status not in (X)`, fixedOpts())
	blob := res.Error + " " + res.Message + " " + strings.Join(res.Unsupported, " | ")
	if !strings.Contains(strings.ToLower(blob), "only =") && !strings.Contains(strings.ToLower(blob), "not in") {
		t.Fatalf("want status not in listed as unsupported, got %q", blob)
	}
	if len(res.Filters.Status) != 0 {
		t.Fatalf("status not in must not apply as include: %+v", res.Filters.Status)
	}
}

func TestHashEncodesProjectNot(t *testing.T) {
	f := EmptyFilter()
	f.JiraProjectNot = []string{"XYZ"}
	f.SourceProjectNot = []string{"scratch"}
	h := Hash(f, Display{})
	q, err := url.ParseQuery(h)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("pjn") != "XYZ" || q.Get("spjn") != "scratch" {
		t.Fatalf("hash %q", h)
	}
}

func TestMatchJiraProjectNot(t *testing.T) {
	keep := Issue{Key: "NMA-1", Project: "NMA"}
	drop := Issue{Key: "XYZ-1", Project: "XYZ"}
	f := EmptyFilter()
	f.JiraProjectNot = []string{"XYZ"}
	if !Match(keep, f) {
		t.Fatal("NMA should survive jira_project_not=XYZ")
	}
	if Match(drop, f) {
		t.Fatal("XYZ should be excluded by jira_project_not=XYZ")
	}
	both := EmptyFilter()
	both.JiraProject = []string{"NMA", "XYZ"}
	both.JiraProjectNot = []string{"XYZ"}
	if !Match(keep, both) {
		t.Fatal("include then exclude: NMA stays")
	}
	if Match(drop, both) {
		t.Fatal("include then exclude: XYZ in both lists, exclude wins")
	}
}

func TestEmitSourceProjectAndNotBothOmitted(t *testing.T) {
	var f Filter
	if err := json.Unmarshal([]byte(`{"source_project":["src"],"source_project_not":["scratch"]}`), &f); err != nil {
		t.Fatal(err)
	}
	got, omitted := Emit(f, Display{}, EmitOpts{})
	if got != "" {
		t.Fatalf("source axes are not JQL: %q", got)
	}
	if strings.Join(omitted, ",") != "source_project,source_project_not" {
		t.Fatalf("omitted %v", omitted)
	}
}
