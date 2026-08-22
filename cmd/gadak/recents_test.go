package main

// GDK-502: the CLI read verbs leave the personal-history rows the UI already
// leaves — `gadak issue` appends a visit per key it actually loaded, `gadak
// search` appends one row per execution (FTS and JQL both). Assertions read
// the file through a second connection, the shape a next gadak process would
// see, never the handle the command itself used.

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// localDB opens the mirror read-only for the assertion pass. OpenReadOnly is
// what a separate `gadak sql` process uses; it ATTACHes local.db, so
// local.visits / local.searches are queryable without wiring anything.
func localDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.OpenReadOnly(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func localVisitCount(t *testing.T, kind, key string) int {
	t.Helper()
	var n int
	if err := localDB(t).QueryRow(`SELECT COUNT(*) FROM local.visits WHERE kind = ? AND key = ?`, kind, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIssueRecordsVisit(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, out)
	}
	if n := localVisitCount(t, "issue", "NMB-1"); n != 1 {
		t.Fatalf("local.visits rows for issue NMB-1 = %d, want 1 — the read verb must append the visit the UI's POST would", n)
	}
}

func TestSearchRecordsSearch(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error { return cmdSearch([]string{"--json", "batch"}) })
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}
	var doc struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &doc); err != nil {
		t.Fatalf("search --json: %v\n%s", err, out)
	}
	if doc.Total < 1 {
		t.Fatalf("fixture drifted: search 'batch' matched nothing\n%s", out)
	}
	db := localDB(t)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM local.searches WHERE query = 'batch'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("local.searches rows for 'batch' = %d, want 1 — the read verb must append the search the UI's POST would", n)
	}
	var rc int
	if err := db.QueryRow(`SELECT result_count FROM local.searches WHERE query = 'batch'`).Scan(&rc); err != nil {
		t.Fatal(err)
	}
	if rc != doc.Total {
		t.Fatalf("result_count = %d, want the search's own total %d (the web client posts res.total too)", rc, doc.Total)
	}
}

func TestSearchJQLRecordsSearch(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error { return cmdSearch([]string{"--jql", "--json", "project = NMB"}) })
	if err != nil {
		t.Fatalf("search --jql: %v\n%s", err, out)
	}
	var doc struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &doc); err != nil {
		t.Fatalf("search --jql --json: %v\n%s", err, out)
	}
	db := localDB(t)
	var n, rc int
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(result_count), 0) FROM local.searches WHERE query = 'project = NMB'`).Scan(&n, &rc); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("local.searches rows for the JQL run = %d, want 1 — the JQL path records the same way the FTS path does", n)
	}
	if rc != doc.Total {
		t.Fatalf("result_count = %d, want the command's own total %d", rc, doc.Total)
	}
}

func TestIssueSkipsVisitForKeyNotLoaded(t *testing.T) {
	mirror(t, "https://unused.example.com")
	// A mixed call: NMB-1 loads, BAD-1 does not. The command reports the
	// miss as its error; only the key that actually loaded is recorded.
	_, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "BAD-1"}) })
	if err == nil {
		t.Fatal("mixed load should report the miss as its error")
	}
	if n := localVisitCount(t, "issue", "NMB-1"); n != 1 {
		t.Fatalf("local.visits rows for NMB-1 = %d, want 1", n)
	}
	if n := localVisitCount(t, "issue", "BAD-1"); n != 0 {
		t.Fatalf("local.visits rows for BAD-1 = %d, want 0 — a key the mirror does not have was not read", n)
	}
}

func TestIssueLinkRecordsNoVisit(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "--link"}) })
	if err != nil {
		t.Fatalf("issue --link: %v\n%s", err, out)
	}
	var n int
	if err := localDB(t).QueryRow(`SELECT COUNT(*) FROM local.visits`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("local.visits rows = %d, want 0 — --link prints an address, it does not load the detail", n)
	}
}

func TestSearchEmitRecordsNoSearch(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error { return cmdSearch([]string{"--jql", "--emit", "project = NMB"}) })
	if err != nil {
		t.Fatalf("search --emit: %v\n%s", err, out)
	}
	var n int
	if err := localDB(t).QueryRow(`SELECT COUNT(*) FROM local.searches`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("local.searches rows = %d, want 0 — --emit prints JQL without searching", n)
	}
}

// dropLocalTable removes one local.db history table through a second
// connection, the way an unreadable or half-migrated local.db behaves for
// RecordVisit / RecordSearch.
func dropLocalTable(t *testing.T, table string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+filepath.Join(os.Getenv("GADAK_HOME"), "local.db")+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`DROP TABLE ` + table); err != nil {
		t.Fatal(err)
	}
}

func TestIssueRecordingIsBestEffort(t *testing.T) {
	mirror(t, "https://unused.example.com")
	dropLocalTable(t, "visits")
	stdout, stderr, err := captureErr(t, func() error { return cmdIssue([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("issue must not fail because its history row could not be written: %v", err)
	}
	if !strings.Contains(stdout, "NMB-1") {
		t.Fatalf("stdout missing the issue it read:\n%s", stdout)
	}
	if !strings.Contains(stderr, "could not record this visit") {
		t.Fatalf("stderr missing the recording warning:\n%s", stderr)
	}
}

func TestSearchRecordingIsBestEffort(t *testing.T) {
	mirror(t, "https://unused.example.com")
	dropLocalTable(t, "searches")
	stdout, stderr, err := captureErr(t, func() error { return cmdSearch([]string{"batch"}) })
	if err != nil {
		t.Fatalf("search must not fail because its history row could not be written: %v", err)
	}
	if !strings.Contains(stdout, "NMB-1") {
		t.Fatalf("stdout missing the hit:\n%s", stdout)
	}
	if !strings.Contains(stderr, "could not record this search") {
		t.Fatalf("stderr missing the recording warning:\n%s", stderr)
	}
}

func seedAndReadTwoIssues(t *testing.T) {
	t.Helper()
	seedNamedIssue(t, "NMB-2", "second issue")
	if _, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "NMB-2", "NMB-1"}) }); err != nil {
		t.Fatalf("issue: %v", err)
	}
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.RecordVisit(context.Background(), store.VisitKindPage, "987654"); err != nil {
		t.Fatal(err)
	}
}

func TestRecentsListsNewestFirstDeduped(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedAndReadTwoIssues(t)
	out, err := capture(t, func() error { return cmdRecents([]string{}) })
	if err != nil {
		t.Fatalf("recents: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("recents printed %d lines, want header + 3 rows:\n%s", len(lines), out)
	}
	if lines[0] != "kind\tkey\tviewed_at" {
		t.Fatalf("header = %q, want the sql-style TSV header", lines[0])
	}
	// Newest first: the page visit is the last insert, NMB-2 precedes NMB-1,
	// and NMB-1 — read twice — folds to one row.
	wantOrder := []string{"987654", "NMB-2", "NMB-1"}
	for i, want := range wantOrder {
		fields := strings.Split(lines[i+1], "\t")
		if len(fields) != 3 {
			t.Fatalf("row %d = %q, want 3 tab-separated fields", i+1, lines[i+1])
		}
		if fields[1] != want {
			t.Fatalf("row %d key = %q, want %q (newest first, deduped):\n%s", i+1, fields[1], want, out)
		}
		if fields[0] != "page" && fields[0] != "issue" {
			t.Fatalf("row %d kind = %q", i+1, fields[0])
		}
	}
	if strings.Count(out, "NMB-1") != 1 {
		t.Fatalf("NMB-1 appears %d times, want 1 (deduped to its newest visit):\n%s", strings.Count(out, "NMB-1"), out)
	}
}

func TestRecentsJSONAndLimit(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedAndReadTwoIssues(t)
	out, err := capture(t, func() error { return cmdRecents([]string{"--json"}) })
	if err != nil {
		t.Fatalf("recents --json: %v\n%s", err, out)
	}
	var rows []struct {
		Kind     string `json:"kind"`
		Key      string `json:"key"`
		ViewedAt string `json:"viewed_at"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &rows); err != nil {
		t.Fatalf("recents --json: %v\n%s", err, out)
	}
	if len(rows) != 3 || rows[0].Key != "987654" || rows[0].ViewedAt == "" {
		t.Fatalf("rows = %+v, want 3 newest-first with viewed_at set", rows)
	}
	out, err = capture(t, func() error { return cmdRecents([]string{"--limit", "1"}) })
	if err != nil {
		t.Fatalf("recents --limit 1: %v\n%s", err, out)
	}
	if got := strings.Count(out, "\n"); got != 2 {
		t.Fatalf("--limit 1 printed %d newlines, want header + 1 row:\n%s", got, out)
	}
}

func TestRecentsEmptyPrintsHeaderOnly(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error { return cmdRecents([]string{}) })
	if err != nil {
		t.Fatalf("recents: %v\n%s", err, out)
	}
	if out != "kind\tkey\tviewed_at\n" {
		t.Fatalf("empty recents = %q, want the header alone", out)
	}
	out, err = capture(t, func() error { return cmdRecents([]string{"--json"}) })
	if err != nil {
		t.Fatalf("recents --json: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty recents --json = %q, want []", out)
	}
}

func TestRecentsRejectsBadLimit(t *testing.T) {
	mirror(t, "https://unused.example.com")
	for _, bad := range []string{"0", "-3"} {
		_, err := capture(t, func() error { return cmdRecents([]string{"--limit", bad}) })
		if err == nil || !strings.Contains(err.Error(), "--limit") {
			t.Fatalf("--limit %s: err = %v, want a usage error naming --limit", bad, err)
		}
	}
}
