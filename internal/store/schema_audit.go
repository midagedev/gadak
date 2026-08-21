package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SchemaAuditResult is the diagnosis of "does this file's sqlite_master
// match the schema this build migrates to?". Missing is the damage signal
// (expected tables/columns/views/indexes that are absent). Extra is
// informational leftover on the file.
type SchemaAuditResult struct {
	Stamp     int      // PRAGMA user_version of the open file
	Supported int      // len(migrations) — the level this build writes
	Missing   []string // "table", "table.column", or a named index
	Extra     []string
}

// OK reports whether the file is missing anything this build expects.
// Surplus objects do not fail the audit.
func (r SchemaAuditResult) OK() bool { return len(r.Missing) == 0 }

// SchemaAudit compares the open mirror against a fresh :memory: database
// that ran this build's migrate() path. It issues only reads against the
// open file. It is a doctor entry point, not part of Open: running
// migrate() on :memory: on every command would be wasted work.
func (db *DB) SchemaAudit(ctx context.Context) (SchemaAuditResult, error) {
	want, err := expectedSchema()
	if err != nil {
		return SchemaAuditResult{}, err
	}
	have, err := inspectSchema(ctx, db.sql)
	if err != nil {
		return SchemaAuditResult{}, err
	}
	var stamp int
	if err := db.sql.QueryRowContext(ctx, "PRAGMA user_version").Scan(&stamp); err != nil {
		return SchemaAuditResult{}, err
	}
	missing, extra := diffSchema(want, have)
	return SchemaAuditResult{
		Stamp:     stamp,
		Supported: len(migrations),
		Missing:   missing,
		Extra:     extra,
	}, nil
}

type schemaSnapshot struct {
	cols    map[string]map[string]struct{} // table/view → columns
	indexes map[string]string              // index name → table name
}

var (
	expectedSchemaOnce sync.Once
	expectedSchemaSnap schemaSnapshot
	expectedSchemaErr  error
)

func expectedSchema() (schemaSnapshot, error) {
	expectedSchemaOnce.Do(func() {
		expectedSchemaSnap, expectedSchemaErr = buildExpectedSchema()
	})
	return expectedSchemaSnap, expectedSchemaErr
}

// buildExpectedSchema opens an in-memory SQLite, ATTACHes a throwaway
// local.db so migrate() can pass personalStateCopyVersion, and runs the
// same migrate() used by Open. Migration SQL is not copied.
func buildExpectedSchema() (schemaSnapshot, error) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return schemaSnapshot{}, err
	}
	defer sqlDB.Close()
	// :memory: is per-connection; the pool must not hand out a second empty db.
	sqlDB.SetMaxOpenConns(1)

	dir, err := os.MkdirTemp("", "gadak-schema-audit-")
	if err != nil {
		return schemaSnapshot{}, err
	}
	defer os.RemoveAll(dir)

	placeholder := filepath.Join(dir, "gadak.db")
	if err := EnsureLocal(placeholder); err != nil {
		return schemaSnapshot{}, err
	}
	attach := "ATTACH DATABASE " + sqliteAttachLiteral(LocalPath(placeholder), "rw") + " AS local"
	if _, err := sqlDB.Exec(attach); err != nil {
		return schemaSnapshot{}, err
	}

	db := &DB{sql: sqlDB, path: ":memory:"}
	if err := db.migrate(); err != nil {
		return schemaSnapshot{}, err
	}
	if db.schemaVersion != len(migrations) {
		return schemaSnapshot{}, fmt.Errorf("schema audit expected migrate reached %d, want %d", db.schemaVersion, len(migrations))
	}
	return inspectSchema(context.Background(), sqlDB)
}

// schemaAuditSkipName is the same exclusion origin_scope_test.go liveTables
// uses for sqlite_master noise: SQLite's own bookkeeping and the fts5
// shadow tables under items_fts. local.db is excluded by reading main
// sqlite_master only (not local.sqlite_master).
func schemaAuditSkipName(name string) bool {
	return strings.HasPrefix(name, "sqlite_") || strings.HasPrefix(name, "items_fts_")
}

func inspectSchema(ctx context.Context, sqlDB *sql.DB) (schemaSnapshot, error) {
	snap := schemaSnapshot{
		cols:    make(map[string]map[string]struct{}),
		indexes: make(map[string]string),
	}
	rows, err := sqlDB.QueryContext(ctx, `SELECT type, name, tbl_name FROM sqlite_master`)
	if err != nil {
		return schemaSnapshot{}, err
	}
	defer rows.Close()

	type obj struct{ typ, name, tbl string }
	var objs []obj
	for rows.Next() {
		var o obj
		if err := rows.Scan(&o.typ, &o.name, &o.tbl); err != nil {
			return schemaSnapshot{}, err
		}
		if schemaAuditSkipName(o.name) {
			continue
		}
		objs = append(objs, o)
	}
	if err := rows.Err(); err != nil {
		return schemaSnapshot{}, err
	}

	for _, o := range objs {
		switch o.typ {
		case "table", "view":
			cols, err := relationColumns(ctx, sqlDB, o.name)
			if err != nil {
				return schemaSnapshot{}, err
			}
			snap.cols[o.name] = cols
		case "index":
			if schemaAuditSkipName(o.tbl) {
				continue
			}
			snap.indexes[o.name] = o.tbl
		}
	}
	return snap, nil
}

func relationColumns(ctx context.Context, sqlDB *sql.DB, name string) (map[string]struct{}, error) {
	// Quote rather than bind: PRAGMA table_info is not a table-valued
	// function on every driver DSN, and names come from sqlite_master.
	q := "PRAGMA table_info(" + quoteIdent(name) + ")"
	rows, err := sqlDB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var colName, typ string
		var notnull int
		var dflt *string
		var pk int
		if err := rows.Scan(&cid, &colName, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[colName] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func diffSchema(want, have schemaSnapshot) (missing, extra []string) {
	missingRel := map[string]struct{}{}
	for name, cols := range want.cols {
		got, ok := have.cols[name]
		if !ok {
			missing = append(missing, name)
			missingRel[name] = struct{}{}
			continue
		}
		for col := range cols {
			if _, ok := got[col]; !ok {
				missing = append(missing, name+"."+col)
			}
		}
	}
	extraRel := map[string]struct{}{}
	for name, cols := range have.cols {
		wantCols, ok := want.cols[name]
		if !ok {
			extra = append(extra, name)
			extraRel[name] = struct{}{}
			continue
		}
		for col := range cols {
			if _, ok := wantCols[col]; !ok {
				extra = append(extra, name+"."+col)
			}
		}
	}
	for idx, tbl := range want.indexes {
		if _, ok := have.indexes[idx]; ok {
			continue
		}
		if _, skip := missingRel[tbl]; skip {
			continue
		}
		missing = append(missing, idx)
	}
	for idx, tbl := range have.indexes {
		if _, ok := want.indexes[idx]; ok {
			continue
		}
		if _, skip := extraRel[tbl]; skip {
			continue
		}
		extra = append(extra, idx)
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}
