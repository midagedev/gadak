// Package store owns the SQLite mirror: schema, migrations, transactions,
// full-text index and the derived fields the source does not provide.
//
// 0.x promises three things, documented in specs/000-product/data-model.md:
// the issues_full view plus the RECIPES queries, gadak sql stdout, and
// views open --keys - semantics. Everything else in the schema is
// documented so you can read it, not promised so you can build on it.
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
	"sync/atomic"
	"time"

	"github.com/midagedev/gadak/internal/config"

	_ "modernc.org/sqlite" // pure-Go driver: the binary must build with CGO_ENABLED=0
)

// ErrNotFound means a single-row lookup (Detail, etc.) found no matching key.
// Callers map it to 404 / "not in the mirror"; store code still uses
// sql.ErrNoRows internally and wraps at the package boundary.
var ErrNotFound = errors.New("not found")

// SchemaTooNewError means the file was migrated by a gadak that reads a later
// schema than this build does — the app and the CLI ship as separately
// versioned formulae, so one build opening the mirror once is enough to lock
// the other out of that workspace (GDK-498).
//
// It is a type, not a string, because more than one surface has to say the
// same true thing about it: nothing is lost, since the mirror is a cache the
// origin can rebuild. Recognise it with errors.As rather than matching text.
type SchemaTooNewError struct {
	Path      string // the mirror file
	Have      int    // schema version found in the file
	Supported int    // highest schema version this build migrates to
}

func (e *SchemaTooNewError) Error() string {
	return fmt.Sprintf("%s: mirror schema %d was written by a newer gadak (this build reads up to %d); "+
		"the mirror is a cache the origin can rebuild — run the newer gadak, or set this file aside and re-sync: "+
		"mv %s %s.bak && gadak sync",
		e.Path, e.Have, e.Supported, e.Path, e.Path)
}

// DB is a handle on the mirror. Safe for concurrent use; writes are serialized.
type DB struct {
	sql           *sql.DB
	path          string
	schemaVersion int

	mu               sync.Mutex // process-local writer serialisation, see write()
	writeBusyRetries atomic.Uint64
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
	dsn := mirrorDSN(path)
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
		return &SchemaTooNewError{Path: db.path, Have: have, Supported: want}
	}
	// The v26 copy writes into local.* in the same transaction that advances
	// user_version. When local.db cannot be attached or migrated, Open's
	// contract still holds — a local.db failure "must not refuse the mirror"
	// (see Open) — so the copy waits instead of failing: stay a version
	// behind, and user_version still gates the copy onto the next Open once
	// local answers. Nothing is lost in the gap; the mirror-side tables keep
	// the rows because schemaV26 deliberately does not drop them.
	if want >= personalStateCopyVersion && have < personalStateCopyVersion && !db.localPersonalTablesReady(ctx) {
		want = personalStateCopyVersion - 1
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

// mirrorDSN is the Open DSN. _txlock=immediate makes BeginTx take the write
// lock up front so a read-then-write callback cannot upgrade a deferred
// transaction (that upgrade returns SQLITE_BUSY without consulting
// busy_timeout). modernc.org/sqlite v1.56.0: driver.go documents _txlock,
// sqlite.go stores it as conn.beginMode, tx.go issues "begin "+beginMode.
func mirrorDSN(path string) string {
	return "file:" + path + "?" + strings.Join([]string{
		"_pragma=busy_timeout(5000)",
		"_pragma=journal_mode(WAL)",
		"_pragma=foreign_keys(1)",
		"_pragma=synchronous(NORMAL)",
		"_txlock=immediate",
	}, "&")
}

// writeBusyAttempts is 2 because each attempt can already block for the 5s
// busy_timeout. One retry covers SQLITE_BUSY_SNAPSHOT (which does not wait)
// and a writer that dropped the lock on the timeout edge. A third attempt
// would stack another 5s onto a request that already waited 10s.
const writeBusyAttempts = 2

// writeBusyBackoff is 1% of busy_timeout(5000). Code 5 has already waited 5s;
// code 517 returns immediately and needs a beat for the other transaction
// to commit.
const writeBusyBackoff = 50 * time.Millisecond

// sqliteCodeError is the modernc.org/sqlite v1.56.0 *Error surface: Code()
// returns the SQLite result code. That driver's Error() string appends
// " (SQLITE_BUSY)" only for primary 5, so 517 is "database is locked (517)"
// with no suffix — do not match the prose.
type sqliteCodeError interface {
	error
	Code() int
}

func sqliteBusy(err error) bool {
	var se sqliteCodeError
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code() {
	case 5: // SQLITE_BUSY
		return true
	case 517: // SQLITE_BUSY_SNAPSHOT
		return true
	default:
		return false
	}
}

func retryBusy(ctx context.Context, retries *atomic.Uint64, fn func() error) error {
	var last error
	for attempt := 1; attempt <= writeBusyAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		last = fn()
		if last == nil || !sqliteBusy(last) || attempt == writeBusyAttempts {
			return last
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		retries.Add(1)
		timer := time.NewTimer(writeBusyBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}

// write runs fn in a transaction. db.mu serialises writers in this process.
// Other processes (serve, desktop, CLI, plugins) are separate connections:
// SQLite's lock, busy_timeout, BEGIN IMMEDIATE, and a bounded SQLITE_BUSY
// retry cover those. Short overlapping writes are fine; the failure mode this
// retry exists for is another process holding the write lock.
func (db *DB) write(ctx context.Context, fn func(*sql.Tx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return retryBusy(ctx, &db.writeBusyRetries, func() error {
		return db.writeOnce(ctx, fn)
	})
}

func (db *DB) writeOnce(ctx context.Context, fn func(*sql.Tx) error) error {
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

// WriteBusyRetries is how many times write() has retried SQLITE_BUSY (5 or
// 517) on this handle. Same shape as PoolStats: a cheap accessor, no logs.
func (db *DB) WriteBusyRetries() uint64 {
	return db.writeBusyRetries.Load()
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
