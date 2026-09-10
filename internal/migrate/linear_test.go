package migrate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/store"
)

func TestPickLinearState(t *testing.T) {
	states := []linear.WorkflowState{
		{ID: "s-todo", Name: "Todo", Type: "unstarted", Position: 1},
		{ID: "s-back", Name: "Backlog", Type: "backlog", Position: 0},
		{ID: "s-prog", Name: "In Progress", Type: "started", Position: 2},
		{ID: "s-rev", Name: "In Review", Type: "started", Position: 3},
		{ID: "s-canc", Name: "Canceled", Type: "canceled", Position: 5},
		{ID: "s-done", Name: "Done", Type: "completed", Position: 4},
	}
	for _, tc := range []struct{ cat, want string }{
		{"new", "s-back"},
		{"inprogress", "s-prog"}, // lowest position among started
		{"done", "s-done"},       // completed, never canceled
		{"", "s-back"},           // unknown category → new
	} {
		if got := pickLinearState(states, tc.cat); got.ID != tc.want {
			t.Errorf("pickLinearState(%q) = %q, want %q", tc.cat, got.ID, tc.want)
		}
	}
	// No backlog state: new falls through to unstarted.
	if got := pickLinearState(states[:1], "new"); got.ID != "s-todo" {
		t.Errorf("new without backlog = %q, want s-todo", got.ID)
	}
	if got := pickLinearState(states[:2], "done"); got.ID != "" {
		t.Errorf("done without any done-category state must be empty, got %q", got.ID)
	}
	// GDK-1314: a team whose done states are all canceled-type is still a
	// team with a done category — sync files canceled under done, so migrate
	// must land there too instead of failing. FAIL-first: the type-list
	// version returned the zero state here.
	cancelOnly := []linear.WorkflowState{
		{ID: "s-back", Name: "Backlog", Type: "backlog", Position: 0},
		{ID: "s-dup", Name: "Duplicate", Type: "duplicate", Position: 9},
		{ID: "s-canc", Name: "Canceled", Type: "canceled", Position: 5},
	}
	if got := pickLinearState(cancelOnly, "done"); got.ID != "s-canc" {
		t.Errorf("done on a canceled-only team = %q, want s-canc (lowest of the cancel-ish kinds)", got.ID)
	}
}

func TestLinearPriority(t *testing.T) {
	for _, tc := range []struct {
		rank, want int
		collapsed  bool
	}{
		{0, 0, false}, {1, 1, false}, {4, 4, false}, {5, 4, true}, {9, 4, true},
	} {
		got, collapsed := linearPriority(tc.rank)
		if got != tc.want || collapsed != tc.collapsed {
			t.Errorf("linearPriority(%d) = %d,%v want %d,%v", tc.rank, got, collapsed, tc.want, tc.collapsed)
		}
	}
}

func TestLinearRelationType(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Blocks", "blocks"}, {"blocks", "blocks"},
		{"Duplicate", "duplicate"}, {"Cloners", "related"},
		{"Relates", "related"}, {"Problem/Incident", "related"}, {"", "related"},
	} {
		if got := linearRelationType(tc.in); got != tc.want {
			t.Errorf("linearRelationType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMigrateFooterRoundTrip(t *testing.T) {
	desc := "line one\n\nline two" + migrateFooter("NMS-12")
	if got := parseMigrateFooter(desc); got != "NMS-12" {
		t.Fatalf("parse = %q", got)
	}
	if got := parseMigrateFooter("no footer here\ngadak-migrate is a word"); got != "" {
		t.Fatalf("false positive %q", got)
	}
	if got := parseMigrateFooter(migrateFooter("A-1") + "\n"); got != "A-1" {
		t.Fatalf("trailing newline: %q", got)
	}
}

// Both ends of a pair often carry the row (inward on one, outward on the
// other); asymmetric pairs also exist. Either way one relation per pair.
func TestLinearRelationsDedupe(t *testing.T) {
	issues := []Issue{
		{Key: "A", Links: []Link{{Type: "Blocks", Outward: "B"}, {Type: "Relates", Outward: "C"}}},
		{Key: "B", Links: []Link{{Type: "Blocks", Inward: "A"}}},
		{Key: "C", Links: []Link{{Type: "Relates", Inward: "A"}, {Type: "Duplicate", Inward: "B"}}},
	}
	got := linearRelations(issues)
	want := map[string]bool{"A blocks B": true, "A related C": true, "B duplicate C": true}
	if len(got) != len(want) {
		t.Fatalf("relations %+v, want %d", got, len(want))
	}
	for _, r := range got {
		if !want[r.from+" "+r.typ+" "+r.to] {
			t.Errorf("unexpected relation %+v", r)
		}
	}
}

func TestLinearTime(t *testing.T) {
	if got := linearTime("2026-08-04T10:46:17.315Z"); got != "2026-08-04T10:46:17.315Z" {
		t.Errorf("utc ms: %q", got)
	}
	if got := linearTime("2026-08-04T19:46:17.315+0900"); got != "2026-08-04T10:46:17.315Z" {
		t.Errorf("jira offset: %q", got)
	}
	if got := linearTime("garbage"); got != "" {
		t.Errorf("garbage: %q", got)
	}
}

// --dry-run must not open a single connection: the client points at a
// server that fails the test on any request.
func TestToLinearDryRunIsOffline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("dry-run made a request: %s", r.URL.Path)
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := linear.New("not-a-key")
	c.Endpoint = srv.URL

	sqlDB, err := store.OpenReadOnly(seedMirror(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	doc, st, err := Build(context.Background(), sqlDB, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ToLinear(context.Background(), c, doc, st, LinearOptions{TeamKey: "FIX", DryRun: true, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || rep.Team != "FIX" {
		t.Fatalf("report %+v", rep)
	}
	byMetric := map[string]VerifyRow{}
	for _, r := range rep.Counts {
		byMetric[r.Metric] = r
	}
	if byMetric["issues"].Source != 2 || byMetric["comments"].Source != 1 || byMetric["relations"].Source != 1 {
		t.Fatalf("counts %+v", rep.Counts)
	}
	joined := strings.Join(rep.NotMigrated, "\n")
	if !strings.Contains(joined, "history") {
		t.Fatalf("history must be reported as not migrated: %q", joined)
	}
	// T-3 is cut by --limit 2; nothing references it, so no drops.
	if byMetric["parents"].Source != 0 {
		t.Fatalf("parents %+v", byMetric["parents"])
	}
}

// fakeLinearServe answers ToLinear's read path from canned pages and lets
// the caller fail the Users lookup selectively (by email substring), so a
// lookup error can be told apart from a miss without a second server.
func fakeLinearServe(t *testing.T, failEmails ...string) (*linear.Client, *int) {
	t.Helper()
	creates := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		page := func(coll, nodes string) string {
			return `{"data":{"` + coll + `":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[` + nodes + `]}}}`
		}
		switch {
		case strings.Contains(string(body), "query Teams"):
			w.Write([]byte(page("teams", `{"id":"team-1","key":"FIX","name":"Fix"}`)))
		case strings.Contains(string(body), "query WorkflowStates"):
			w.Write([]byte(page("workflowStates", `{"id":"s-back","name":"Backlog","type":"backlog","position":0}`)))
		case strings.Contains(string(body), "query Issues"):
			w.Write([]byte(page("issues", "")))
		case strings.Contains(string(body), "query Labels"):
			w.Write([]byte(page("issueLabels", `{"id":"lab-1","name":"gadak-migrate"}`)))
		case strings.Contains(string(body), "query Users"):
			for _, email := range failEmails {
				if strings.Contains(string(body), email) {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
			w.Write([]byte(`{"data":{"users":{"nodes":[]}}}`))
		case strings.Contains(string(body), "mutation IssueCreate"):
			creates++
			w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":{"id":"i-` +
				strconv.Itoa(creates) + `","identifier":"FIX-` + strconv.Itoa(creates) + `","createdAt":"2026-09-10T00:00:00.000Z"}}}}`))
		default:
			t.Errorf("unexpected GraphQL document: %.120s", body)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	c := linear.New("not-a-key")
	c.Endpoint = srv.URL
	c.Retries, c.Backoff = 1, 0
	return c, &creates
}

// TestToLinearNamesFailedAssigneeLookups is the GDK-1318 gate for the
// Linear path: a Users lookup that errors must not fail the run (one
// person must not sink the migration), must not pass as success either —
// the report names the account — and must stay distinguishable from a
// genuine miss (the account Linear simply does not have). FAIL-first: on
// the swallowing source, the run reported plain success and this test
// failed on the missing line.
func TestToLinearNamesFailedAssigneeLookups(t *testing.T) {
	c, creates := fakeLinearServe(t, "broken@example.com")
	doc := &Doc{
		Issues: []Issue{
			{Key: "NMB-1", Summary: "lookup breaks", StatusCategory: "new",
				Assignee: "acc-broken", AssigneeEmail: "broken@example.com"},
			{Key: "NMB-2", Summary: "genuine miss", StatusCategory: "new",
				Assignee: "acc-miss", AssigneeEmail: "miss@example.com"},
		},
	}
	rep, err := ToLinear(context.Background(), c, doc, nil, LinearOptions{TeamKey: "FIX"})
	if err != nil {
		t.Fatalf("a failed assignee lookup must not fail the run (one person, whole migration): %v", err)
	}
	joined := strings.Join(rep.NotMigrated, "\n")
	if !strings.Contains(joined, "assignee lookups failed for 1 accounts (acc-broken)") {
		t.Fatalf("a failed lookup must be named in the report, not swallowed: %q", joined)
	}
	if strings.Contains(joined, "acc-miss") {
		t.Fatalf("a genuine miss is not a lookup failure — the assignees row already counts it: %q", joined)
	}
	// The migration continued: both issues were created, and the assignees
	// row tells the truth (source 2, migrated 0) instead of fake success.
	if *creates != 2 {
		t.Fatalf("creates = %d, want 2 — the run must keep going past a failed lookup", *creates)
	}
	byMetric := map[string]VerifyRow{}
	for _, r := range rep.Counts {
		byMetric[r.Metric] = r
	}
	if a := byMetric["assignees"]; a.Source != 2 || a.Migrated != 0 {
		t.Fatalf("assignees row %+v, want Source 2 / Migrated 0", a)
	}
}
