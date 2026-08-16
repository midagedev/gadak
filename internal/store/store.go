// Package store owns the SQLite mirror: schema, migrations, transactions,
// full-text index and the derived fields the source does not provide.
//
// The schema is a public contract documented in
// specs/000-product/data-model.md — agents query this database directly, so a
// column change belongs in that document before it belongs here.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/midagedev/gadak/internal/config"

	_ "modernc.org/sqlite" // pure-Go driver: the binary must build with CGO_ENABLED=0
)

// ErrNotFound means a single-row lookup (Detail, etc.) found no matching key.
// Callers map it to 404 / "not in the mirror"; store code still uses
// sql.ErrNoRows internally and wraps at the package boundary.
var ErrNotFound = errors.New("not found")

// DB is a handle on the mirror. Safe for concurrent use; writes are serialized.
type DB struct {
	sql           *sql.DB
	path          string
	schemaVersion int

	mu sync.Mutex // single-writer discipline, see write()
}

// Now returns UTC now in config.ISOMilli. The server hands this value to
// clients as the `delta` cursor; see that constant for why milliseconds
// are required.
func Now() string { return time.Now().UTC().Format(config.ISOMilli) }

// Open opens or creates the mirror at path and migrates it forward. A database
// written by a newer gadak is refused rather than used.
//
// The data directory is created (and existing dirs tightened) to 0700; the DB
// file and any -wal/-shm sidecars are set to 0600. Chmod failures are logged
// and ignored so unsupported filesystems (or Windows) still work.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := ensurePrivateDir(dir); err != nil {
			return nil, err
		}
	}
	dsn := "file:" + path + "?" + strings.Join([]string{
		"_pragma=busy_timeout(5000)",
		"_pragma=journal_mode(WAL)",
		"_pragma=foreign_keys(1)",
		"_pragma=synchronous(NORMAL)",
	}, "&")
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Personal history is a sibling file (local.db), ATTACHed as `local` by
	// attachLocalHook on every connection. Create/migrate it first so the hook
	// finds the file; a failure here must not refuse the mirror.
	if err := EnsureLocal(path); err != nil {
		log.Printf("store: local.db: %v", err)
	}
	db := &DB{sql: sqlDB, path: path}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	// After migrate: a copied or externally-produced database can carry an
	// items_fts whose DDL this build cannot write to (GDK-112). One row read
	// on a healthy mirror; a rebuild only when the shape disagrees.
	if err := db.repairItemsFTS(context.Background()); err != nil {
		sqlDB.Close()
		return nil, err
	}
	// After migrate so -wal/-shm (created on first use under WAL) are present
	// when they exist. SQLite mints new sidecars with the main file's mode, so
	// 0600 on the DB is enough for later writes; we still chmod any that exist.
	secureDBFiles(path)
	secureDBFiles(LocalPath(path))
	return db, nil
}

// ensurePrivateDir creates dir at 0700 and chmods an existing one to 0700 so
// older installs left at 0755 are quietly tightened.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		log.Printf("store: chmod %s: %v", dir, err)
	}
	return nil
}

// secureDBFiles sets the mirror and its WAL/SHM sidecars to 0600 when present.
// Missing sidecars are ignored; chmod errors are warnings only.
func secureDBFiles(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := os.Chmod(p, 0o600); err != nil {
			log.Printf("store: chmod %s: %v", p, err)
		}
	}
}

func (db *DB) Close() error { return db.sql.Close() }

// SchemaVersion is the migration level this binary applied.
func (db *DB) SchemaVersion() int { return db.schemaVersion }

func (db *DB) migrate() error {
	ctx := context.Background()
	var have int
	if err := db.sql.QueryRowContext(ctx, "PRAGMA user_version").Scan(&have); err != nil {
		return err
	}
	want := len(migrations)
	if have > want {
		return fmt.Errorf("%s: schema version %d is newer than this build of gadak supports (%d); upgrade gadak or point --db elsewhere", db.path, have, want)
	}
	db.schemaVersion = want
	if have == want {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		for i := have; i < want; i++ {
			if _, err := tx.Exec(migrations[i]); err != nil {
				return fmt.Errorf("migration %d: %w", i+1, err)
			}
			// v15: derive pages.excerpt from body_adf for rows already mirrored.
			if i+1 == 15 {
				if err := backfillPageExcerpts(tx); err != nil {
					return fmt.Errorf("migration 15 backfill: %w", err)
				}
			}
			// v16: derive item_refs (page↔issue text cross-refs) for existing rows.
			if i+1 == 16 {
				if err := backfillItemRefs(tx); err != nil {
					return fmt.Errorf("migration 16 backfill: %w", err)
				}
			}
		}
		// user_version is the migration level; sync_state.schema_version is the
		// documented mirror of it and has to move with it.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", want)); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE sync_state SET schema_version = ?`, want)
		return err
	})
}

// write runs fn in a transaction. One mutex is the whole single-writer story:
// concurrent writers under WAL would just trade this lock for SQLITE_BUSY.
func (db *DB) write(ctx context.Context, fn func(*sql.Tx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// nz maps an absent string to SQL NULL. data-model.md defines NULL as "unknown
// or absent", which is what the documented COALESCE queries assume.
func nz(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// jsonArray keeps array columns json_each-able: absent is "[]", never NULL.
func jsonArray(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func jsonObject(v map[string]any) string {
	if len(v) == 0 {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func jsonRaw(v json.RawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return string(v)
}

func parseArray(s *string) []string {
	out := []string{}
	if s == nil || *s == "" {
		return out
	}
	_ = json.Unmarshal([]byte(*s), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func rawOrNull(s *string) json.RawMessage {
	if s == nil || *s == "" {
		return nil
	}
	return json.RawMessage(*s)
}
