package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

// TestSchemaAuditCleanMirror is the noise-floor: a freshly migrated file must
// not be reported as damaged. FTS shadow tables, sqlite_sequence, and
// local.db (ATTACH) have to stay out of the diff or this is red forever.
func TestSchemaAuditCleanMirror(t *testing.T) {
	db := openTemp(t)
	got, err := db.SchemaAudit(context.Background())
	if err != nil {
		t.Fatalf("SchemaAudit: %v", err)
	}
	if got.Supported != len(migrations) {
		t.Errorf("Supported = %d, want %d", got.Supported, len(migrations))
	}
	if got.Stamp != db.SchemaVersion() {
		t.Errorf("Stamp = %d, want live SchemaVersion %d", got.Stamp, db.SchemaVersion())
	}
	if len(got.Missing) != 0 {
		t.Errorf("clean mirror Missing = %v, want empty", got.Missing)
	}
	if len(got.Extra) != 0 {
		t.Errorf("clean mirror Extra = %v, want empty", got.Extra)
	}
	if !got.OK() {
		t.Errorf("clean mirror OK() = false")
	}
}

// TestSchemaAuditFrankensteinMissingTable is the 2026-08-17 accident: stamp
// matches this build so migrate() no-ops, but a table the head schema has is
// gone. Doctor (and anything else) has to name the hole instead of surfacing
// a raw "no such table" later.
func TestSchemaAuditFrankensteinMissingTable(t *testing.T) {
	db := plantFrankenstein(t, func(raw *sql.DB) {
		if _, err := raw.Exec(`DROP TABLE versions`); err != nil {
			t.Fatalf("DROP TABLE versions: %v", err)
		}
	})
	got, err := db.SchemaAudit(context.Background())
	if err != nil {
		t.Fatalf("SchemaAudit: %v", err)
	}
	if got.OK() {
		t.Fatal("frankenstein with versions dropped: OK() = true")
	}
	if !containsName(got.Missing, "versions") {
		t.Fatalf("Missing = %v, want versions", got.Missing)
	}
	if got.Stamp != len(migrations) || got.Supported != len(migrations) {
		t.Errorf("stamp/supported = %d/%d, want %d/%d (head no-op)",
			got.Stamp, got.Supported, len(migrations), len(migrations))
	}
}

// TestSchemaAuditFrankensteinMissingColumn drops one ADD COLUMN from a table
// no view depends on, so SQLite will actually remove it, and the audit has
// to report table.column rather than only the table.
func TestSchemaAuditFrankensteinMissingColumn(t *testing.T) {
	db := plantFrankenstein(t, func(raw *sql.DB) {
		if _, err := raw.Exec(`ALTER TABLE changelog DROP COLUMN author_id`); err != nil {
			t.Fatalf("DROP COLUMN changelog.author_id: %v", err)
		}
	})
	got, err := db.SchemaAudit(context.Background())
	if err != nil {
		t.Fatalf("SchemaAudit: %v", err)
	}
	if got.OK() {
		t.Fatal("frankenstein with changelog.author_id dropped: OK() = true")
	}
	if !containsName(got.Missing, "changelog.author_id") {
		t.Fatalf("Missing = %v, want changelog.author_id", got.Missing)
	}
}

// TestSchemaAuditExtraIsNotMismatch: surplus objects are informational. A
// leftover table must not trip the damaged-mirror diagnosis.
func TestSchemaAuditExtraIsNotMismatch(t *testing.T) {
	db := openTemp(t)
	if _, err := db.sql.Exec(`CREATE TABLE leftover_audit_probe (id TEXT)`); err != nil {
		t.Fatal(err)
	}
	got, err := db.SchemaAudit(context.Background())
	if err != nil {
		t.Fatalf("SchemaAudit: %v", err)
	}
	if !got.OK() || len(got.Missing) != 0 {
		t.Fatalf("extra table counted as mismatch: Missing=%v Extra=%v", got.Missing, got.Extra)
	}
	if !containsName(got.Extra, "leftover_audit_probe") {
		t.Fatalf("Extra = %v, want leftover_audit_probe", got.Extra)
	}
}

// TestSchemaAuditDoesNotWrite is the recovery contract: the function is a
// diagnosis, not a repair. A dropped table must stay dropped.
func TestSchemaAuditDoesNotWrite(t *testing.T) {
	db := plantFrankenstein(t, func(raw *sql.DB) {
		if _, err := raw.Exec(`DROP TABLE versions`); err != nil {
			t.Fatalf("DROP TABLE versions: %v", err)
		}
	})
	stampBefore := db.SchemaVersion()
	if _, err := db.SchemaAudit(context.Background()); err != nil {
		t.Fatalf("SchemaAudit: %v", err)
	}
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='versions'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("SchemaAudit recreated versions — it must be read-only")
	}
	var stamp int
	if err := db.sql.QueryRow(`PRAGMA user_version`).Scan(&stamp); err != nil {
		t.Fatal(err)
	}
	if stamp != stampBefore {
		t.Fatalf("user_version moved %d → %d", stampBefore, stamp)
	}
}

// plantFrankenstein builds a head-schema file, mutates the DDL under a stamp
// that still equals this build, and reopens through Open so migrate() no-ops
// (have==want) the way the 2026-08-17 file did.
func plantFrankenstein(t *testing.T, mutate func(*sql.DB)) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stamp := db.SchemaVersion()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	mutate(raw)
	if _, err := raw.Exec(fmt.Sprintf("PRAGMA user_version = %d", stamp)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func containsName(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
