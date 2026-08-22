package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
)

// sqlDemoHome copies examples/demo.db into a throwaway GADAK_HOME, the same
// fixture pattern as TestDoctorDemoDBCounts.
func sqlDemoHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	src := filepath.Join("..", "..", "examples", "demo.db")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db"), raw, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}
}

func TestSQLNoHeaderOmitsTSVHeader(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--no-header", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --no-header: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("empty stdout: %q", out)
	}
	if lines[0] == "key" {
		t.Fatalf("first row is the header, want an issue key: %q", out)
	}
	if !looksLikeIssueKey(lines[0]) {
		t.Fatalf("first row %q is not an issue key", lines[0])
	}
	if len(lines) != 3 {
		t.Fatalf("want 3 data rows, got %d: %q", len(lines), out)
	}
}

func TestSQLDefaultKeepsTSVHeader(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql default: %v\n%s", err, out)
	}
	first, _, _ := strings.Cut(out, "\n")
	if first != "key" {
		t.Fatalf("default first row must stay the header, got %q", first)
	}
}

func TestSQLJSONNoHeaderIsNoop(t *testing.T) {
	sqlDemoHome(t)
	plain, err := capture(t, func() error {
		return cmdSQL([]string{"--json", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --json: %v\n%s", err, plain)
	}
	with, err := capture(t, func() error {
		return cmdSQL([]string{"--json", "--no-header", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --json --no-header: %v\n%s", err, with)
	}
	if with != plain {
		t.Fatalf("--json --no-header must match --json\n--json:\n%s\n--json --no-header:\n%s", plain, with)
	}
}

func TestSQLNoHeaderOmitsCSVHeader(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--csv", "--no-header", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --csv --no-header: %v\n%s", err, out)
	}
	first, _, _ := strings.Cut(out, "\n")
	if first == "key" {
		t.Fatalf("csv first row is the header, want an issue key: %q", out)
	}
	if !looksLikeIssueKey(first) {
		t.Fatalf("csv first row %q is not an issue key", first)
	}
}

func TestSQLUnknownFlagIsUsageError(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--pretty", "select key from issues limit 1"})
	})
	if err == nil {
		t.Fatalf("unknown flag --pretty must be a usage error, got nil err and stdout %q", out)
	}
	if !strings.Contains(err.Error(), "--pretty") {
		t.Fatalf("usage error must echo --pretty, got %v", err)
	}
	if !strings.Contains(err.Error(), `run "gadak sql --help"`) {
		t.Fatalf("want usageError help pointer, got %v", err)
	}
}

func TestSQLQuotedCommentQueryStillRuns(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"-- comment\nselect key from issues limit 1"})
	})
	if err != nil {
		t.Fatalf("quoted SQL starting with -- comment: %v\n%s", err, out)
	}
	first, _, _ := strings.Cut(out, "\n")
	if first != "key" {
		t.Fatalf("want header, got %q\n%s", first, out)
	}
}

// setSourcesSyncedAt rewrites every source's synced_at in the test-home
// mirror so warnIfStale's hour arithmetic (agent.go: staleAfter, synced_at
// parsed as RFC3339, last_error empty in demo.db) is deterministic whatever
// demo.db's fixture vintage is. warnIfStale keys on the oldest row across
// sync_state (GDK-598), so bumping only jira would leave confluence stale.
func setSourcesSyncedAt(t *testing.T, at time.Time) {
	t.Helper()
	path, err := config.DBPath()
	if err != nil {
		t.Fatalf("db path: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open mirror writable: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE sources SET synced_at = ?`, at.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set synced_at: %v", err)
	}
}

func TestSQLWarnsOnStaleMirror(t *testing.T) {
	sqlDemoHome(t)
	setSourcesSyncedAt(t, time.Now().Add(-2*time.Hour))
	staleOut, staleErr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql on stale mirror: %v\n%s", err, staleOut)
	}
	lines := strings.Split(strings.TrimSuffix(staleErr, "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "warning: mirror last synced") {
		t.Fatalf("stale mirror must warn exactly once on stderr, got %q", staleErr)
	}
	// GDK-598: the age warning teaches `sync --if-stale 1h` instead of a bare
	// `gadak sync` (original pin was the prefix only).
	if !strings.Contains(lines[0], "gadak sync --if-stale 1h") {
		t.Fatalf("stale warning must recommend `gadak sync --if-stale 1h`, got %q", staleErr)
	}

	setSourcesSyncedAt(t, time.Now())
	freshOut, freshErr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql on fresh mirror: %v\n%s", err, freshOut)
	}
	if freshErr != "" {
		t.Fatalf("fresh mirror must stay quiet on stderr, got %q", freshErr)
	}
	if freshOut != staleOut {
		t.Fatalf("stdout must be identical stale vs fresh\nstale:\n%s\nfresh:\n%s", staleOut, freshOut)
	}
	first, _, _ := strings.Cut(freshOut, "\n")
	if first != "key" {
		t.Fatalf("first stdout row must stay the header, got %q", first)
	}
	if !looksLikeIssueKey(strings.Split(strings.TrimSuffix(freshOut, "\n"), "\n")[1]) {
		t.Fatalf("stdout must still carry data rows, got %q", freshOut)
	}
}

// TestSQLWarnsOnLinearLastError is GDK-598 FAIL-first: warnIfStale used to
// query only source_id='jira', so a Linear-only last_error never warned.
func TestSQLWarnsOnLinearLastError(t *testing.T) {
	sqlDemoHome(t)
	setSourcesSyncedAt(t, time.Now())
	path, err := config.DBPath()
	if err != nil {
		t.Fatalf("db path: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open mirror: %v", err)
	}
	if _, err := db.Exec(`UPDATE sync_state SET last_error = NULL WHERE source_id = 'jira'`); err != nil {
		db.Close()
		t.Fatalf("clear jira last_error: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sources (id, kind) VALUES ('linear', 'linear')
		ON CONFLICT(id) DO UPDATE SET kind = excluded.kind`); err != nil {
		db.Close()
		t.Fatalf("plant linear source: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sync_state (source_id, last_error, schema_version, version)
		VALUES ('linear', 'planted linear fail', 0, 0)
		ON CONFLICT(source_id) DO UPDATE SET last_error = excluded.last_error`); err != nil {
		db.Close()
		t.Fatalf("plant linear last_error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, stderr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql with linear last_error: %v\n%s", err, out)
	}
	if !strings.Contains(stderr, "last sync failed (linear): planted linear fail") {
		t.Fatalf("linear last_error must warn on stderr, got %q", stderr)
	}
	if strings.Contains(out, "warning:") {
		t.Fatalf("warning leaked to stdout: %q", out)
	}
	first, _, _ := strings.Cut(out, "\n")
	if first != "key" {
		t.Fatalf("stdout must stay the TSV header, got %q", first)
	}
}

func TestSQLWarnsOnJiraLastErrorNamesSource(t *testing.T) {
	sqlDemoHome(t)
	setSourcesSyncedAt(t, time.Now())
	path, err := config.DBPath()
	if err != nil {
		t.Fatalf("db path: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open mirror: %v", err)
	}
	if _, err := db.Exec(`UPDATE sync_state SET last_error = ? WHERE source_id = 'jira'`, "planted jira fail"); err != nil {
		db.Close()
		t.Fatalf("plant jira last_error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, stderr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql with jira last_error: %v\n%s", err, out)
	}
	if !strings.Contains(stderr, "last sync failed (jira): planted jira fail") {
		t.Fatalf("jira last_error must still warn (with source), got %q", stderr)
	}
	if strings.Contains(out, "warning:") {
		t.Fatalf("warning leaked to stdout: %q", out)
	}
}

// GDK-654 FAIL-first: an empty leftover jira sync_state row used to make
// warnIfStale claim the mirror had never finished a sync while Linear had.
func TestSQLDoesNotWarnNeverSyncedWhenLinearIsFresh(t *testing.T) {
	sqlDemoHome(t)
	path, err := config.DBPath()
	if err != nil {
		t.Fatalf("db path: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open mirror: %v", err)
	}
	if _, err := db.Exec(`UPDATE sources SET synced_at = NULL WHERE id = 'jira'`); err != nil {
		db.Close()
		t.Fatalf("clear jira synced_at: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sources (id, kind, synced_at) VALUES ('linear', 'linear', ?)
		ON CONFLICT(id) DO UPDATE SET kind = excluded.kind, synced_at = excluded.synced_at`,
		time.Now().UTC().Format(time.RFC3339)); err != nil {
		db.Close()
		t.Fatalf("plant linear source: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sync_state (source_id, last_error, schema_version, version)
		VALUES ('linear', NULL, 0, 0)
		ON CONFLICT(source_id) DO UPDATE SET last_error = NULL`); err != nil {
		db.Close()
		t.Fatalf("plant linear sync_state: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, stderr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql with fresh linear: %v\n%s", err, out)
	}
	if strings.Contains(stderr, "never finished a sync") {
		t.Fatalf("empty jira synced_at must not poison a fresh Linear source, got %q", stderr)
	}
	if strings.Contains(out, "warning:") {
		t.Fatalf("warning leaked to stdout: %q", out)
	}
}

func TestSQLDisplayNameZeroRowHint(t *testing.T) {
	sqlDemoHome(t)
	// demo.db has English "In Progress" rows, so the 0-row case uses the
	// same display-name column with a key that cannot match (GDK-553).
	out, stderr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues where status = 'In Progress' and key = 'NO-SUCH'"})
	})
	if err != nil {
		t.Fatalf("sql: %v\n%s", err, out)
	}
	if !strings.Contains(stderr, "zero rows with a display-name filter") {
		t.Fatalf("0-row status= filter must print the display-name hint on stderr, got %q", stderr)
	}
	for _, col := range []string{"status_category", "priority_rank", "issue_type_id"} {
		if !strings.Contains(stderr, col) {
			t.Fatalf("hint must point at %s, got %q", col, stderr)
		}
	}

	_, safeErr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues where status_category = 'nonexistent'"})
	})
	if err != nil {
		t.Fatalf("sql safe filter: %v", err)
	}
	if strings.Contains(safeErr, "display-name") {
		t.Fatalf("status_category filter must not warn, got %q", safeErr)
	}
}

// GDK-255: agents copy issue_key from JSON into SQL; issues_full has key.
// The suggestion is on the error (stderr via main), never on stdout.
func TestSQLDidYouMeanIssueKey(t *testing.T) {
	sqlDemoHome(t)
	out, stderr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select issue_key from issues_full limit 1"})
	})
	if err == nil {
		t.Fatalf("want no such column, got stdout %q", out)
	}
	msg := err.Error()
	if !strings.Contains(msg, "no such column") {
		t.Fatalf("want original sqlite error, got %v", err)
	}
	if !strings.Contains(msg, `did you mean "key"`) {
		t.Fatalf("want did you mean \"key\", got %v (stderr %q)", err, stderr)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("stdout must stay empty (gadak sql stdout is a contract), got %q", out)
	}
}

func TestSQLDidYouMeanOmitsDistant(t *testing.T) {
	sqlDemoHome(t)
	_, _, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select zzqx from issues_full limit 1"})
	})
	if err == nil {
		t.Fatal("want no such column")
	}
	if strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("distant name must not suggest, got %v", err)
	}
}
