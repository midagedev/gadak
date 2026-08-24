package dashboards

// Contract ↔ assertion map (GDK-781). Every rule the spec fixed has at
// least a normal case and a violation/boundary case here:
//
//	contract clause                          | assertions
//	-----------------------------------------+-------------------------------------------
//	html required non-empty                  | TestParseConfig (valid minimal), TestParseConfigRejects (missing/empty/whitespace)
//	datasource = exactly one of sql xor jql  | TestParseConfig (sql one + jql one), TestParseConfigRejects (both / neither)
//	datasource name [a-z0-9][a-z0-9_-]{0,63} | TestParseConfigNameRule (valid set + boundary length 64, invalid set)
//	strict config (unknown key = error)      | TestParseConfigRejects (unknown top-level + unknown datasource key + truncated JSON)
//	name→row resolution                      | TestFindDashboard (exact/substring/id), TestFindDashboardMiss (0-hit, ambiguous)
//	datasource SQL runs read-only            | TestExecuteSQLWriteRefused (UPDATE/INSERT refused by mode=ro)
//	row ceilings 10000 rows / 2 MiB          | TestExecuteSQLTruncation (row cap, byte cap, first giant row still surfaces)
//	0-row display-name comparison → warning  | TestExecuteSQLWarning (Korean-status mirror, English display name)
//	JQL: parse → identity → match → fixed    | TestExecuteJQL (currentUser, statusCategory, resolution is EMPTY,
//	  columns, CLI-parity sort                 ORDER BY updated ASC)
//	JQL refused when nothing applies         | TestExecuteJQLRefused (watchers > 1, parse error)
//	JQL partial apply warns, doesn't lie     | TestExecuteJQLPartialWarning (currentUser skipped →
//	  (no identity → "Mine" = all open)      |   rows still filter, warning names the clause)
//	store version counter is monotonic       | TestStoreRoundTrip (save/update/delete each bump)

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
)

// openDash opens a throwaway store — never the real ~/.gadak.
func openDash(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seeded mirrors the server fixture's three-issue shape (Korean display
// statuses on purpose: NMB-1 진행 중/inprogress, NMB-2 완료/done, NMA-9 할
// 일/new) so the display-name warning axis is exercised against real locale
// data, not a placeholder.
func seeded(t *testing.T) *store.DB {
	t.Helper()
	db := openDash(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Records: []store.IssueRecord{
			{
				Item: store.Item{ID: "jira:1001", SourceID: "jira", ExternalID: "1001", Key: "NMB-1",
					Title:     "batch worker drops the last page",
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug",
					Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
					Priority: "High", Assignee: "김현철", AssigneeID: "acc-hc", AssigneeEmail: "hc@example.com"},
			},
			{
				Item: store.Item{ID: "jira:1002", SourceID: "jira", ExternalID: "1002", Key: "NMB-2",
					Title:     "cloud upload retries forever",
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-20T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug",
					Status: "완료", StatusID: "10001", StatusCategory: "done", Priority: "Medium"},
			},
			{
				Item: store.Item{ID: "jira:2001", SourceID: "jira", ExternalID: "2001", Key: "NMA-9",
					Title:     "modeler crash on import",
					CreatedAt: "2026-07-05T00:00:00.000Z", UpdatedAt: "2026-07-06T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "NMA", IssueType: "Task",
					Status: "할 일", StatusID: "1", StatusCategory: "new"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return db
}

func TestParseConfig(t *testing.T) {
	minimal, err := ParseConfig([]byte(`{"html":"<p>hi</p>"}`))
	if err != nil {
		t.Fatalf("minimal: %v", err)
	}
	if minimal.HTML != "<p>hi</p>" || len(minimal.Datasources) != 0 {
		t.Fatalf("minimal = %+v", minimal)
	}
	full, err := ParseConfig([]byte(`{
		"html": "<b>ok</b>",
		"datasources": {"open": {"sql": "select 1"}, "mine": {"jql": "assignee = currentUser()"}}
	}`))
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if full.Datasources["open"].SQL != "select 1" || full.Datasources["mine"].JQL != "assignee = currentUser()" {
		t.Fatalf("datasources = %+v", full.Datasources)
	}
	// Round-trip: a parsed config marshals back to a config that parses.
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := ParseConfig(raw); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
}

func TestParseConfigRejects(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"truncated json", `{"html":`, "not valid JSON"},
		{"html missing", `{"datasources":{}}`, "html is required"},
		{"html empty", `{"html":""}`, "html is required"},
		{"html whitespace", `{"html":"   "}`, "html is required"},
		{"html non-string", `{"html":3}`, "html must be a string"},
		{"unknown top-level", `{"html":"x","queries":{}}`, `unknown field "queries"`},
		{"datasources non-object", `{"html":"x","datasources":[]}`, "datasources must be an object"},
		{"sql and jql", `{"html":"x","datasources":{"a":{"sql":"select 1","jql":"project = NMB"}}}`, "got both"},
		{"neither", `{"html":"x","datasources":{"a":{}}}`, "got neither"},
		{"unknown datasource key", `{"html":"x","datasources":{"a":{"query":"select 1"}}}`, `unknown field "query"`},
		{"datasource sql non-string", `{"html":"x","datasources":{"a":{"sql":7}}}`, "sql must be a string"},
	}
	for _, tc := range cases {
		_, err := ParseConfig([]byte(tc.raw))
		if err == nil {
			t.Fatalf("%s: accepted %s", tc.name, tc.raw)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: error %q missing %q", tc.name, err.Error(), tc.want)
		}
	}
}

// The name rule is a boundary the URL path depends on: it becomes the
// /data/{name}/ segment. 64 chars is the last valid length (boundary); 65
// and every invalid start/charset member are refused with the pattern in
// the message so the fix needs no docs lookup.
func TestParseConfigNameRule(t *testing.T) {
	valid := []string{"a", "0", "open", "open-by-status", "v2_counts", strings.Repeat("n", 64)}
	for _, name := range valid {
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false, want true", name)
		}
	}
	invalid := []string{"", "-lead", "_x", "Upper", "has space", "한글", strings.Repeat("n", 65)}
	for _, name := range invalid {
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true, want false", name)
		}
	}
	for _, name := range []string{"Upper", "-lead", strings.Repeat("n", 65)} {
		raw := `{"html":"x","datasources":{` + quote(name) + `:{"sql":"select 1"}}}`
		_, err := ParseConfig([]byte(raw))
		if err == nil || !strings.Contains(err.Error(), NamePattern) {
			t.Errorf("name %q: error = %v, want pattern %s in message", name, err, NamePattern)
		}
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestFindDashboard(t *testing.T) {
	db := openDash(t)
	ctx := context.Background()
	first, err := db.SaveDashboard(ctx, "Triage Wall", `{"html":"<p>a</p>"}`)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := db.SaveDashboard(ctx, "Release Board", `{"html":"<p>b</p>"}`); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	for _, name := range []string{"triage wall", "  Triage Wall  ", "TRIAGE WALL", first.ID} {
		d, err := FindDashboard(db, name)
		if err != nil {
			t.Fatalf("FindDashboard(%q): %v", name, err)
		}
		if d.ID != first.ID {
			t.Fatalf("FindDashboard(%q) = %s, want %s", name, d.ID, first.ID)
		}
	}
	// One substring hit resolves — prefixes stay usable.
	if _, err := FindDashboard(db, "triage"); err != nil {
		t.Fatalf("substring: %v", err)
	}
}

func TestFindDashboardMiss(t *testing.T) {
	db := openDash(t)
	_, err := FindDashboard(db, "zzz")
	if err == nil || !strings.Contains(err.Error(), "none saved") {
		t.Fatalf("empty-store message = %v", err)
	}
	ctx := context.Background()
	if _, err := db.SaveDashboard(ctx, "Alpha", `{"html":"x"}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := db.SaveDashboard(ctx, "Beta", `{"html":"x"}`); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	_, err = FindDashboard(db, "zzz")
	if err == nil || !strings.Contains(err.Error(), "available: Alpha; Beta") {
		t.Fatalf("available-list message = %v", err)
	}
	if _, err := FindDashboard(db, "a"); err == nil || !strings.Contains(err.Error(), `matches 2`) {
		t.Fatalf("ambiguous message = %v", err)
	}
}

// The store round-trip is also where the monotonic version counter lives:
// every write an open tab must reflect moves it by exactly one.
func TestStoreRoundTrip(t *testing.T) {
	db := openDash(t)
	ctx := context.Background()
	if v, err := db.DashboardVersion(ctx); err != nil || v != 0 {
		t.Fatalf("version before first save = %d, %v; want 0", v, err)
	}
	a, err := db.SaveDashboard(ctx, "one", `{"html":"<p>1</p>"}`)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if v, _ := db.DashboardVersion(ctx); v != 1 {
		t.Fatalf("version after save = %d, want 1", v)
	}
	// Same name updates the row it owns: id and created_at survive.
	updated, err := db.SaveDashboard(ctx, "one", `{"html":"<p>1b</p>"}`)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ID != a.ID {
		t.Fatalf("update minted a new id: %s → %s", a.ID, updated.ID)
	}
	if v, _ := db.DashboardVersion(ctx); v != 2 {
		t.Fatalf("version after update = %d, want 2", v)
	}
	if list, _ := db.Dashboards(ctx); len(list) != 1 {
		t.Fatalf("same-name save duplicated a row: %d", len(list))
	}
	got, err := db.Dashboard(ctx, a.ID)
	if err != nil || got.Name != "one" {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if err := db.DeleteDashboard(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if v, _ := db.DashboardVersion(ctx); v != 3 {
		t.Fatalf("version after delete = %d, want 3", v)
	}
	if _, err := db.Dashboard(ctx, a.ID); err != store.ErrNotFound {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
	if err := db.DeleteDashboard(ctx, a.ID); err != store.ErrNotFound {
		t.Fatalf("double delete = %v, want ErrNotFound", err)
	}
}

func TestExecuteSQL(t *testing.T) {
	db := seeded(t)
	ro, err := db.ReadOnly()
	if err != nil {
		t.Fatalf("readonly: %v", err)
	}
	defer ro.Close()

	res, err := ExecuteSQL(ro, `SELECT status_category, COUNT(*) FROM issues GROUP BY 1 ORDER BY 1`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(res.Columns) != 2 || res.Columns[0] != "status_category" {
		t.Fatalf("columns = %v", res.Columns)
	}
	// SQLite TEXT arrives as []byte; cell must hand back strings.
	if len(res.Rows) != 3 || res.Rows[0][0] != "done" {
		t.Fatalf("rows = %v", res.Rows)
	}
	if res.Truncated || res.Warning != "" {
		t.Fatalf("flags = truncated:%v warning:%q", res.Truncated, res.Warning)
	}

	// NULL cells stay null, and an empty result keeps its shape ([] not null).
	res, err = ExecuteSQL(ro, `SELECT key, priority FROM issues WHERE key = 'NMA-9'`)
	if err != nil {
		t.Fatalf("null row: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0][1] != nil {
		t.Fatalf("null cell = %v", res.Rows)
	}
	res, err = ExecuteSQL(ro, `SELECT key FROM issues WHERE 0`)
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if res.Rows == nil || len(res.Rows) != 0 {
		t.Fatalf("empty rows = %#v, want non-nil empty", res.Rows)
	}
}

// The read-only connection is the whole reason arbitrary SQL is allowed:
// a datasource that tries to mutate the mirror must fail at the driver,
// not at a hope. This is a contract pin (the refusal is mode=ro, which
// predates this package) — the FAIL-first red for the feature lives in
// internal/server, whose first-cut handler lacked the CSP header.
func TestExecuteSQLWriteRefused(t *testing.T) {
	db := seeded(t)
	ro, err := db.ReadOnly()
	if err != nil {
		t.Fatalf("readonly: %v", err)
	}
	defer ro.Close()
	for _, q := range []string{
		`UPDATE issues SET title = 'pwned'`,
		`INSERT INTO issues (id) VALUES ('x')`,
		`DELETE FROM issues`,
	} {
		if _, err := ExecuteSQL(ro, q); err == nil {
			t.Errorf("%q: write accepted on read-only connection", q)
		}
	}
	// And the mirror is untouched.
	lites, err := db.IssueLites(context.Background())
	if err != nil || len(lites) != 3 {
		t.Fatalf("issue count after write attempts = %d, %v; want 3", len(lites), err)
	}
}

func TestExecuteSQLWarning(t *testing.T) {
	db := seeded(t)
	ro, err := db.ReadOnly()
	if err != nil {
		t.Fatalf("readonly: %v", err)
	}
	defer ro.Close()

	// The fixture's statuses are Korean (진행 중/완료/할 일). An English
	// display-name comparison returns 0 rows — the locale trap this project
	// refuses to let a user debug alone (CLAUDE.md schema rules).
	res, err := ExecuteSQL(ro, `SELECT key FROM issues WHERE status = 'In Progress'`)
	if err != nil {
		t.Fatalf("display-name query: %v", err)
	}
	if len(res.Rows) != 0 {
		t.Fatalf("rows = %v, want 0", res.Rows)
	}
	if res.Warning == "" {
		t.Fatal("0 rows + display-name comparison must carry a warning")
	}
	// The stable-key spelling of the same intent: rows, no warning.
	res, err = ExecuteSQL(ro, `SELECT key FROM issues WHERE status_category = 'inprogress'`)
	if err != nil {
		t.Fatalf("category query: %v", err)
	}
	if len(res.Rows) != 1 || res.Warning != "" {
		t.Fatalf("category rows = %v warning = %q", res.Rows, res.Warning)
	}
}

func TestExecuteSQLTruncation(t *testing.T) {
	db := seeded(t)
	ro, err := db.ReadOnly()
	if err != nil {
		t.Fatalf("readonly: %v", err)
	}
	defer ro.Close()

	// Row cap: 10001 generated rows, ceiling is 10000.
	res, err := ExecuteSQL(ro, `WITH RECURSIVE cnt(x) AS (
		SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 10001)
		SELECT x FROM cnt`)
	if err != nil {
		t.Fatalf("row cap query: %v", err)
	}
	if len(res.Rows) != MaxRows || !res.Truncated {
		t.Fatalf("rows = %d truncated = %v; want %d, true", len(res.Rows), res.Truncated, MaxRows)
	}

	// Byte cap: hex(zeroblob(50000)) is a ~100 KiB string per row, so the
	// 2 MiB ceiling lands around row 21 of 500.
	res, err = ExecuteSQL(ro, `WITH RECURSIVE cnt(x) AS (
		SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 500)
		SELECT x, hex(zeroblob(50000)) AS big FROM cnt`)
	if err != nil {
		t.Fatalf("byte cap query: %v", err)
	}
	if !res.Truncated || len(res.Rows) >= 500 {
		t.Fatalf("byte cap: %d rows truncated=%v", len(res.Rows), res.Truncated)
	}
	if len(res.Rows) < 15 {
		t.Fatalf("byte cap cut too early: %d rows", len(res.Rows))
	}

	// A single giant row still surfaces — truncation never yields an
	// empty, silently-cut result for a one-row query.
	res, err = ExecuteSQL(ro, `SELECT hex(zeroblob(2000000)) AS giant`)
	if err != nil {
		t.Fatalf("giant row: %v", err)
	}
	if len(res.Rows) != 1 || len(res.Rows[0][0].(string)) != 4000000 {
		t.Fatalf("giant row = %d rows, payload %d bytes", len(res.Rows), len(res.Rows[0][0].(string)))
	}
	// The read of that one row crossed the ceiling, so truncated is set —
	// the caller is told there was more, not that the row was lost.
	if !res.Truncated {
		t.Fatalf("giant row truncated = false, want true")
	}
}

func TestExecuteJQL(t *testing.T) {
	db := seeded(t)
	ctx := context.Background()
	me := jql.Identity{Email: "hc@example.com", AccountID: "acc-hc"}

	res, err := ExecuteJQL(ctx, db, me, `assignee = currentUser()`)
	if err != nil {
		t.Fatalf("currentUser: %v", err)
	}
	wantCols := []string{"issue_key", "summary", "status_category", "status", "priority_rank", "updated_at"}
	if len(res.Columns) != len(wantCols) {
		t.Fatalf("columns = %v", res.Columns)
	}
	for i, c := range wantCols {
		if res.Columns[i] != c {
			t.Fatalf("columns = %v", res.Columns)
		}
	}
	if len(res.Rows) != 1 || res.Rows[0][0] != "NMB-1" {
		t.Fatalf("currentUser rows = %v", res.Rows)
	}
	if res.Rows[0][2] != "inprogress" || res.Rows[0][3] != "진행 중" {
		t.Fatalf("row cells = %v (status_category then display status)", res.Rows[0])
	}

	// resolution is EMPTY ≈ both open buckets (jql's own semantics).
	res, err = ExecuteJQL(ctx, db, me, `resolution is EMPTY`)
	if err != nil {
		t.Fatalf("resolution EMPTY: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("open rows = %v, want NMB-1 and NMA-9", res.Rows)
	}

	// ORDER BY parity with `gadak search --jql`: asc puts NMB-2 (2026-07-20)
	// before NMB-1 (2026-08-01).
	res, err = ExecuteJQL(ctx, db, me, `project = NMB ORDER BY updated ASC`)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	if len(res.Rows) != 2 || res.Rows[0][0] != "NMB-2" || res.Rows[1][0] != "NMB-1" {
		t.Fatalf("order rows = %v", res.Rows)
	}
}

// Partial application is the live-measured trap: `assignee = currentUser()
// AND resolution is EMPTY` with no configured identity degrades to all open
// issues (currentUser() is skipped, resolution still applies) and a "Mine"
// card would read true. The warning must name the skipped clause.
func TestExecuteJQLPartialWarning(t *testing.T) {
	db := seeded(t)
	ctx := context.Background()

	res, err := ExecuteJQL(ctx, db, jql.Identity{}, `assignee = currentUser() AND resolution is EMPTY`)
	if err != nil {
		t.Fatalf("partial apply: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("rows = %v, want the open pair the applicable half matches", res.Rows)
	}
	if !strings.Contains(res.Warning, "jql skipped") || !strings.Contains(res.Warning, "currentUser()") {
		t.Fatalf("warning = %q, want the skipped clause named", res.Warning)
	}

	// Fully-applied queries carry no warning.
	res, err = ExecuteJQL(ctx, db, jql.Identity{Email: "hc@example.com"}, `resolution is EMPTY`)
	if err != nil {
		t.Fatalf("clean query: %v", err)
	}
	if res.Warning != "" {
		t.Fatalf("clean warning = %q, want empty", res.Warning)
	}
}

func TestExecuteJQLRefused(t *testing.T) {
	db := seeded(t)
	ctx := context.Background()
	me := jql.Identity{Email: "hc@example.com", AccountID: "acc-hc"}

	// A clause the subset refuses must not silently match everything.
	_, err := ExecuteJQL(ctx, db, me, `watchers > 1`)
	if err == nil || !strings.Contains(err.Error(), "cannot apply JQL") {
		t.Fatalf("unsupported-only query = %v, want cannot apply", err)
	}
	// A parse error names itself.
	_, err = ExecuteJQL(ctx, db, me, `project =`)
	if err == nil || !strings.Contains(err.Error(), "jql:") {
		t.Fatalf("parse error = %v", err)
	}
}
