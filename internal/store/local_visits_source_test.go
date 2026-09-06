package store

// visits.source (localSchemaV7): which surface caused the read — cli, ui or
// mcp — so agent reads do not pollute a person's resume time. The contract
// assertions this file owns:
//
//	C1a RecordVisit accepts cli/ui/mcp and persists the value -> TestRecordVisitSourcePersisted
//	C1b RecordVisit rejects anything else, exact message     -> TestRecordVisitSourceRejected
//	C7  a V6 local.db migrates to 7; rows recorded before V7
//	    read back with the empty (unknown) source            -> TestLocalV6MigratesToV7KeepsOldRows

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordVisitSourcePersisted(t *testing.T) {
	db := openTemp(t)
	for _, src := range []string{VisitSourceCLI, VisitSourceUI, VisitSourceMCP} {
		v, err := db.RecordVisit(context.Background(), VisitKindIssue, "STD-1", src)
		if err != nil {
			t.Fatalf("RecordVisit source %q: %v", src, err)
		}
		if v.Source != src {
			t.Fatalf("returned Visit.Source = %q, want %q", v.Source, src)
		}
		var got string
		if err := db.sql.QueryRowContext(context.Background(),
			`SELECT source FROM local.visits WHERE id = ?`, v.ID).Scan(&got); err != nil {
			t.Fatalf("read row %d back: %v", v.ID, err)
		}
		if got != src {
			t.Fatalf("stored source = %q, want %q", got, src)
		}
	}
	// The server POST returns the Visit as JSON; source is part of that shape.
	raw, err := json.Marshal(Visit{Kind: VisitKindIssue, Key: "STD-2", Source: VisitSourceUI})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"source":"ui"`) {
		t.Fatalf("Visit JSON = %s, want a source field", raw)
	}
}

func TestRecordVisitSourceRejected(t *testing.T) {
	db := openTemp(t)
	for _, src := range []string{"", "web", "CLI", "agent"} {
		_, err := db.RecordVisit(context.Background(), VisitKindIssue, "STD-1", src)
		if err == nil {
			t.Fatalf("source %q accepted, want rejection", src)
		}
		if !strings.Contains(err.Error(), `source must be "cli", "ui" or "mcp"`) {
			t.Fatalf("error for %q = %q, want the source-must message", src, err)
		}
	}
	// The rejection must not leave a row behind.
	var n int
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM local.visits`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rejected writes left %d rows, want 0", n)
	}
}

// TestLocalV6MigratesToV7KeepsOldRows builds the file an older build leaves
// behind — migrations 1..6 applied, rows without a source column — and runs
// the EnsureLocal path Open uses. FAIL-first evidence (pre-change tree):
// sqlite3 "select source from visits" on a V6 local.db errored
// "no such column: source" (failfirst/c7-source-column-prechange.out).
func TestLocalV6MigratesToV7KeepsOldRows(t *testing.T) {
	dir := t.TempDir()
	mirror := filepath.Join(dir, "gadak.db")
	local := LocalPath(mirror)

	raw, err := sql.Open("sqlite", "file:"+local)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range localMigrations[:6] {
		if _, err := raw.Exec(m); err != nil {
			t.Fatalf("apply V6-era migration: %v", err)
		}
	}
	if _, err := raw.Exec(`INSERT INTO visits (kind, key, viewed_at) VALUES ('issue','STD-9','2026-08-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 6`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := EnsureLocal(mirror); err != nil {
		t.Fatalf("EnsureLocal on a V6 local.db: %v", err)
	}
	chk, err := sql.Open("sqlite", "file:"+local)
	if err != nil {
		t.Fatal(err)
	}
	defer chk.Close()
	var uv int
	if err := chk.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != 7 {
		t.Fatalf("user_version after migration = %d, want 7", uv)
	}
	var kind, key, src string
	if err := chk.QueryRow(`SELECT kind, key, source FROM visits WHERE key = 'STD-9'`).Scan(&kind, &key, &src); err != nil {
		t.Fatalf("read pre-V7 row through the new column: %v", err)
	}
	if src != "" {
		t.Fatalf("pre-V7 row source = %q, want the empty default", src)
	}

	// And the migrated file keeps taking writes with a source, through the
	// same Open path production uses.
	db, err := Open(mirror)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v, err := db.RecordVisit(context.Background(), VisitKindIssue, "STD-10", VisitSourceCLI)
	if err != nil {
		t.Fatalf("RecordVisit on migrated file: %v", err)
	}
	if v.Source != VisitSourceCLI {
		t.Fatalf("new row source = %q, want %q", v.Source, VisitSourceCLI)
	}
	var old string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT source FROM local.visits WHERE key = 'STD-9'`).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if old != "" {
		t.Fatalf("old row source drifted to %q, want empty", old)
	}
}
