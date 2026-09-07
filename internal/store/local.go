package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fsperm"

	sqlite "modernc.org/sqlite"
)

// localDBFile is the personal-history SQLite file, next to the mirror.
// Never sent to Jira, never copied by snapshot (a separate file, not a table).
const localDBFile = "local.db"

// localRetention is how long raw visit/search events are kept. Counts older
// than this window are not rolled up — derive from whatever is still here.
const localRetention = 180 * 24 * time.Hour

// localMigrations is independent of the mirror's migrations slice. Index+1 is
// PRAGMA user_version on local.db.
var localMigrations = []string{localSchemaV1, localSchemaV2, localSchemaV3, localSchemaV4, localSchemaV5, localSchemaV6, localSchemaV7}

const localSchemaV1 = `
CREATE TABLE visits (
  id        INTEGER PRIMARY KEY,
  kind      TEXT NOT NULL,
  key       TEXT NOT NULL,
  viewed_at TEXT NOT NULL
);
CREATE INDEX visits_viewed_at ON visits(viewed_at);
CREATE INDEX visits_kind_key ON visits(kind, key);

CREATE TABLE searches (
  id           INTEGER PRIMARY KEY,
  query        TEXT NOT NULL,
  searched_at  TEXT NOT NULL,
  result_count INTEGER NOT NULL,
  opened_kind  TEXT,
  opened_key   TEXT
);
CREATE INDEX searches_searched_at ON searches(searched_at);
`

// localSchemaV2 is recent-use history (picker ranking: assignee, transition,
// create-type, …). Newest-first, de-duped, cap recentCap per kind — the same
// contract the web localStorage helper used to own alone.
const localSchemaV2 = `
CREATE TABLE recents (
  id      INTEGER PRIMARY KEY,
  kind    TEXT NOT NULL,
  value   TEXT NOT NULL,
  used_at TEXT NOT NULL,
  UNIQUE(kind, value)
);
CREATE INDEX recents_kind_used_at ON recents(kind, used_at);
`

// localSchemaV3 gives history a generation. Replacing a workspace's origin
// (localOrigin → a real site) leaves visit and search rows naming keys the new
// origin can mint too, and the timeline resolves a bare key against whatever
// mirror is current — so `STD-1 viewed yesterday` starts pointing at a
// stranger's issue (GDK-418). Deleting them is not available: data-model.md
// names visit and search history as data gadak may hold and must be able to
// export. So rows carry the epoch they were recorded under, the timeline shows
// only the current one, and the retired rows stay readable through
// `gadak sql` (local.visits / local.searches).
//
// The counter lives in local_meta rather than PRAGMA user_version, which the
// migration index already owns.
const localSchemaV3 = `
ALTER TABLE visits ADD COLUMN origin_epoch INTEGER NOT NULL DEFAULT 0;
ALTER TABLE searches ADD COLUMN origin_epoch INTEGER NOT NULL DEFAULT 0;
CREATE INDEX visits_epoch ON visits(origin_epoch);
CREATE INDEX searches_epoch ON searches(origin_epoch);
CREATE TABLE local_meta (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
`

// localSchemaV4 is the personal-state move (GDK-105): saved_views, watches,
// favorites and feed_reads leave the mirror schema and live here, so
// `rm gadak.db` — the documented one-line recovery — can no longer delete the
// only copy of a view the user authored. Same DDL shape as the mirror tables
// they replace (schemaV1 / schemaV4 there). Rows cross the file boundary in
// the mirror-side schemaV26 copy migration, not here: this file has no
// attachment of the mirror to read from.
const localSchemaV4 = `
CREATE TABLE saved_views (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  config     TEXT NOT NULL,
  created_at TEXT,
  updated_at TEXT
);

CREATE TABLE watches (
  key        TEXT PRIMARY KEY,
  created_at TEXT
);

CREATE TABLE favorites (
  key        TEXT PRIMARY KEY,
  created_at TEXT
);

CREATE TABLE feed_reads (
  event_id TEXT PRIMARY KEY,
  read_at  TEXT NOT NULL
);
`

// localSchemaV5 is named read-only SQL (GDK-503). Distinct from saved_views:
// those store ViewConfig JSON that the UI and `gadak views` consume. A recipe
// is a name for a mirror SELECT; rank still comes from the mirror's
// priority_rank / status_category, not a private ranking engine.
const localSchemaV5 = `
CREATE TABLE recipes (
  name       TEXT PRIMARY KEY,
  sql        TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`

// localSchemaV6 is agent-authored dashboards (GDK-780/781): an HTML document
// plus named datasources, stored the same shape as saved_views so the same
// personal-state rules apply (survives `rm gadak.db`, exportable). config is
// validated by internal/dashboards at every writer; the store only owns rows
// and the change counter open tabs poll (local_meta dashboards_version).
const localSchemaV6 = `
CREATE TABLE dashboards (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  config     TEXT NOT NULL,
  created_at TEXT,
  updated_at TEXT
);
`

// localSchemaV7 separates person reads from agent reads. visits gains a
// source column naming the surface that caused the read: cli (a gadak
// command), ui (the web client's POST), mcp (reserved; no surface records
// it yet). The empty default is a row recorded before V7 — source unknown —
// and wherever person reads are separated from agent reads, unknown counts
// with the person. No index: the only new predicate (source IN ...) rides
// the visits_viewed_at range scan of a weekly read, not a lookup.
const localSchemaV7 = `
ALTER TABLE visits ADD COLUMN source TEXT NOT NULL DEFAULT '';
`

func init() {
	// Single ATTACH owner: every connection the registered "sqlite" driver
	// opens (store.Open, gadak sql, MCP gadak_query, raw tests) goes through
	// here. Creation/migration of the file is EnsureLocal; this hook only
	// attaches an existing sibling and never fails the connection.
	//
	// BEGIN IMMEDIATE takes RESERVED on every attached rw database (SQLite
	// lang_transaction), so a mirror writer also write-locks local.db.
	// History inserts therefore use a dedicated local.db connection
	// (GDK-753) and do not take the *mirror* lock. The ATTACH stays rw:
	// saved_views / feed_reads / v26 still write local.* on this handle.
	sqlite.RegisterConnectionHook(attachLocalHook)
}

// LocalPath is local.db beside the mirror at mirrorPath.
func LocalPath(mirrorPath string) string {
	return filepath.Join(filepath.Dir(mirrorPath), localDBFile)
}

func attachLocalHook(conn sqlite.ExecQuerierContext, dsn string) error {
	path, ro, ok := parseSQLiteFileDSN(dsn)
	if !ok {
		return nil
	}
	if filepath.Base(path) == localDBFile {
		return nil
	}
	local := LocalPath(path)
	if _, err := os.Stat(local); err != nil {
		// MCP / raw mode=ro open gadak.db without going through Open.
		// Create an empty local.db so SELECT local.visits does not fail;
		// other filenames are left alone (tests, snapshots, scratch DBs).
		if filepath.Base(path) != "gadak.db" {
			return nil
		}
		if err := EnsureLocal(path); err != nil {
			log.Printf("store: local.db: %v", err)
			return nil
		}
	}
	mode := "rw"
	if ro {
		mode = "ro"
	}
	stmt := "ATTACH DATABASE " + sqliteAttachLiteral(local, mode) + " AS local"
	if _, err := conn.ExecContext(context.Background(), stmt, []driver.NamedValue{}); err != nil {
		// Re-ATTACH of a name this connection already has is reuse (the hook
		// can run twice on one *conn). Classify by PRAGMA database_list, not
		// the driver's Error() sentence (GDK-285). modernc *Error.Code() for
		// that case is SQLITE_ERROR (1), the same code as "too many attached
		// databases" and incomplete SQL, so a result code would swallow real
		// failures. Unclassifiable errors (PRAGMA fails, or `local` is absent)
		// are logged — same bias as before.
		if schemaAttached(conn, "local") {
			localAttachReuses.Add(1)
			return nil
		}
		log.Printf("store: ATTACH local.db: %v", err)
	}
	return nil
}

// localAttachReuses counts silent re-ATTACHes of schema `local` — how many
// times attachLocalHook found the schema already present and skipped the
// failure log. Process-wide because the hook is registered on the driver, not
// on a *DB. Same shape as WriteBusyRetries: atomic, free when unread, no logs.
// Read by this package's tests; nothing in production reads it.
var localAttachReuses atomic.Uint64

// localNewerSchemaWarned is path → struct{} of local.db files that have
// already logged the "schema newer than this build" notice this process.
// Keyed by filepath.Clean(path) so two Opens of one file share one line;
// a bare sync.Once would hide a second profile. Bounded by the number of
// distinct local.db paths a process opens (one per profile/workspace) —
// gadak serve does not grow this without a new home.
var localNewerSchemaWarned sync.Map

// localNewerSchemaWarnsSuppressed counts duplicate notices that were not
// printed — how many times migrateLocal skipped a repeat of the newer-schema
// notice for a path already warned. Process-wide because EnsureLocal runs on
// every Open/OpenReadOnly. Same shape as localAttachReuses: atomic, free when
// unread, no logs. Read by this package's tests; nothing in production reads it.
var localNewerSchemaWarnsSuppressed atomic.Uint64

// warnLocalNewerSchema logs the downgrade notice once per local.db path.
// The file is left as-is (reads of tables this build knows still work);
// returning nil from migrateLocal is what keeps callers from re-logging.
func warnLocalNewerSchema(path string, have, want int) {
	key := filepath.Clean(path)
	if _, dup := localNewerSchemaWarned.LoadOrStore(key, struct{}{}); dup {
		localNewerSchemaWarnsSuppressed.Add(1)
		return
	}
	log.Printf("store: local.db: %s: schema version %d is newer than this build of gadak supports (%d); leaving personal history as-is; upgrade gadak, or use a different --workspace / GADAK_HOME", path, have, want)
}

func schemaAttached(conn sqlite.ExecQuerierContext, name string) bool {
	rows, err := conn.QueryContext(context.Background(), "PRAGMA database_list", nil)
	if err != nil {
		return false
	}
	defer rows.Close()
	n := len(rows.Columns())
	if n < 2 {
		n = 3
	}
	dest := make([]driver.Value, n)
	for {
		if err := rows.Next(dest); err != nil {
			return false
		}
		if schemaNameEqual(dest[1], name) {
			return true
		}
	}
}

func schemaNameEqual(v driver.Value, name string) bool {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case []byte:
		s = string(t)
	default:
		return false
	}
	return strings.EqualFold(s, name)
}

// parseSQLiteFileDSN extracts the filesystem path and mode=ro from a modernc
// DSN of the form file:<path>[?k=v&...]. In-memory and empty DSNs are skipped.
func parseSQLiteFileDSN(dsn string) (path string, readOnly bool, ok bool) {
	s := strings.TrimPrefix(dsn, "file:")
	if s == dsn {
		// Not a file: URI (":memory:" etc.).
		return "", false, false
	}
	if s == ":memory:" || strings.HasPrefix(s, ":memory:?") {
		return "", false, false
	}
	path, query, found := strings.Cut(s, "?")
	if path == "" {
		return "", false, false
	}
	if found {
		q, err := url.ParseQuery(query)
		if err == nil && q.Get("mode") == "ro" {
			readOnly = true
		}
	}
	return path, readOnly, true
}

// sqliteAttachLiteral is a file: URI quoted for ATTACH, with mode=ro|rw.
func sqliteAttachLiteral(path, mode string) string {
	p := filepath.ToSlash(path)
	var b strings.Builder
	b.WriteString("file:")
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case ' ', '?', '#', '%', '&':
			fmt.Fprintf(&b, "%%%02X", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("?mode=")
	b.WriteString(mode)
	return "'" + strings.ReplaceAll(b.String(), "'", "''") + "'"
}

// EnsureLocal creates local.db next to the mirror if needed and migrates it.
// Failures are returned; callers of Open log them and still open the mirror.
func EnsureLocal(mirrorPath string) error {
	return ensureLocalDB(LocalPath(mirrorPath))
}

// localPersonalTablesReady reports whether the personal-state tables the
// schemaV26 copy writes to exist and are reachable — i.e. schema `local` is
// attached AND migrated to localSchemaV4. Any error (no attachment, older
// local.db, closed handle) reads as not-ready: the caller stays a mirror
// version behind rather than failing the copy. One probe per Open.
func (db *DB) localPersonalTablesReady(ctx context.Context) bool {
	var one int
	return db.sql.QueryRowContext(ctx,
		`SELECT 1 FROM local.sqlite_master WHERE type = 'table' AND name = 'saved_views'`).Scan(&one) == nil
}

// localDSN opens local.db as the main database (no mirror ATTACH). Same
// pragmas as the ensure path: busy_timeout(5000), DELETE journal, NORMAL
// sync. attachLocalHook skips basename local.db, so this connection never
// waits on the mirror write lock (GDK-753).
func localDSN(path string) string {
	return "file:" + path + "?" + strings.Join([]string{
		"_pragma=busy_timeout(5000)",
		"_pragma=journal_mode(DELETE)",
		"_pragma=synchronous(NORMAL)",
	}, "&")
}

func ensureLocalDB(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := fsperm.EnsurePrivateDir(dir); err != nil {
			if errors.Is(err, fsperm.ErrChmod) {
				log.Printf("store: %v", err)
			} else {
				return err
			}
		}
	}
	// DELETE journal: this file is small and opened from many short-lived
	// connections (sql, MCP). WAL sidecars next to a test TempDir have been
	// seen to survive Close and fail cleanup; the mirror stays WAL.
	sqlDB, err := sql.Open("sqlite", localDSN(path))
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := migrateLocal(sqlDB, path); err != nil {
		return err
	}
	secureDBFiles(path)
	return nil
}

func migrateLocal(sqlDB *sql.DB, path string) error {
	ctx := context.Background()
	var have int
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&have); err != nil {
		return err
	}
	want := len(localMigrations)
	if have > want {
		// Owner of the once-per-path notice. EnsureLocal is called from
		// Open, OpenReadOnly, and (only when local.db is missing) the
		// attach hook; gadak sql opens twice (cmdSQL + warnIfStale).
		// Logging at callers tracks command shape, not this decision.
		// Nil: the file is usable as-is, so callers' `if err != nil { log }`
		// must not reprint this.
		warnLocalNewerSchema(path, have, want)
		return nil
	}
	if have == want {
		return nil
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := have; i < want; i++ {
		if _, err := tx.Exec(localMigrations[i]); err != nil {
			return fmt.Errorf("local migration %d: %w", i+1, err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", want)); err != nil {
		return err
	}
	return tx.Commit()
}

// OpenReadOnly opens the mirror with SQLite mode=ro and ATTACHes local.db
// (created empty if missing so SELECT local.visits never fails the connection).
func OpenReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if err := EnsureLocal(path); err != nil {
		log.Printf("store: local.db: %v", err)
	}
	return sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
}

// VisitKind is visits.kind / searches.opened_kind: "issue" or "page".
const (
	VisitKindIssue = "issue"
	VisitKindPage  = "page"
)

func validVisitKind(k string) bool {
	return k == VisitKindIssue || k == VisitKindPage
}

// Visit sources: the surface that caused the read. Decided by the recorder
// (the CLI knows it is the CLI; the server knows the request is its UI), so
// a client can never claim one — the POST body does not carry source.
const (
	VisitSourceCLI = "cli"
	VisitSourceUI  = "ui"
	VisitSourceMCP = "mcp"
)

func validVisitSource(s string) bool {
	return s == VisitSourceCLI || s == VisitSourceUI || s == VisitSourceMCP
}

// Visit is one append-only view event.
type Visit struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	ViewedAt string `json:"viewed_at"`
	Source   string `json:"source"`
}

// Search is one append-only search execution. OpenedKind/OpenedKey name the
// item opened from that search, when one was.
type Search struct {
	ID          int64   `json:"id"`
	Query       string  `json:"query"`
	SearchedAt  string  `json:"searched_at"`
	ResultCount int     `json:"result_count"`
	OpenedKind  *string `json:"opened_kind"`
	OpenedKey   *string `json:"opened_key"`
}

// currentEpochSQLOnLocal is currentEpochSQL when local.db is the main
// database (no `local.` schema prefix). Keep in lockstep with origin_scope.go.
const currentEpochSQLOnLocal = `COALESCE((SELECT CAST(v AS INTEGER) FROM local_meta WHERE k = 'origin_epoch'), 0)`

// openLocalWriter opens local.db as a main database so history inserts do
// not take the mirror's BEGIN IMMEDIATE lock. MaxOpenConns(1) pins the
// PRAGMA to the connection BeginTx uses.
func openLocalWriter(mirrorPath string) (*sql.DB, error) {
	path := LocalPath(mirrorPath)
	if err := ensureLocalDB(path); err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("sqlite", localDSN(path))
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	return sqlDB, nil
}

// withLocalWrite runs fn in a transaction on a dedicated local.db connection.
// Failures name local.db so CLI best-effort warnings identify the file
// (GDK-753 debug layer). ErrNotFound is left unwrapped for errors.Is.
func (db *DB) withLocalWrite(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sqlDB, err := openLocalWriter(db.path)
	if err != nil {
		return fmt.Errorf("local.db: %w", err)
	}
	defer sqlDB.Close()
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("local.db: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		if errors.Is(err, ErrNotFound) {
			return err
		}
		return fmt.Errorf("local.db: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("local.db: %w", err)
	}
	return nil
}

// RecordVisit appends one view. Same (kind, key) twice is two rows.
// The insert uses a dedicated local.db connection so a writer holding the
// mirror (BEGIN IMMEDIATE / _txlock=immediate) does not block recording
// (GDK-753). Reads that join visits to the mirror stay on the ATTACH path.
// source is decided here, once, from the caller's identity: the CLI passes
// cli, the server passes ui — never anything a request body named.
func (db *DB) RecordVisit(ctx context.Context, kind, key, source string) (Visit, error) {
	var zero Visit
	if !validVisitKind(kind) {
		return zero, fmt.Errorf("kind must be %q or %q", VisitKindIssue, VisitKindPage)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return zero, errors.New("key required")
	}
	if !validVisitSource(source) {
		return zero, errors.New(`source must be "cli", "ui" or "mcp"`)
	}
	at := Now()
	var id int64
	err := db.withLocalWrite(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO visits (kind, key, viewed_at, origin_epoch, source)
			VALUES (?,?,?,`+currentEpochSQLOnLocal+`,?)`, kind, key, at, source)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return zero, err
	}
	return Visit{ID: id, Kind: kind, Key: key, ViewedAt: at, Source: source}, nil
}

// RecordSearch appends one search. openedKind/openedKey may both be empty, or
// both set to the item opened from this search.
func (db *DB) RecordSearch(ctx context.Context, query string, resultCount int, openedKind, openedKey string) (Search, error) {
	var zero Search
	if resultCount < 0 {
		return zero, errors.New("result_count must be >= 0")
	}
	openedKind = strings.TrimSpace(openedKind)
	openedKey = strings.TrimSpace(openedKey)
	if (openedKind == "") != (openedKey == "") {
		return zero, errors.New("opened_kind and opened_key must be set together")
	}
	if openedKind != "" && !validVisitKind(openedKind) {
		return zero, fmt.Errorf("opened_kind must be %q or %q", VisitKindIssue, VisitKindPage)
	}
	at := Now()
	var id int64
	err := db.withLocalWrite(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO searches (query, searched_at, result_count, opened_kind, opened_key, origin_epoch)
			VALUES (?,?,?,?,?,`+currentEpochSQLOnLocal+`)`,
			query, at, resultCount, nz(openedKind), nz(openedKey))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return zero, err
	}
	s := Search{ID: id, Query: query, SearchedAt: at, ResultCount: resultCount}
	if openedKind != "" {
		s.OpenedKind = &openedKind
		s.OpenedKey = &openedKey
	}
	return s, nil
}

// SetSearchOpened stamps the item opened from an existing search row.
func (db *DB) SetSearchOpened(ctx context.Context, id int64, kind, key string) (Search, error) {
	var zero Search
	if !validVisitKind(kind) {
		return zero, fmt.Errorf("opened_kind must be %q or %q", VisitKindIssue, VisitKindPage)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return zero, errors.New("opened_key required")
	}
	err := db.withLocalWrite(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE searches SET opened_kind = ?, opened_key = ? WHERE id = ?`, kind, key, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return zero, err
	}
	return db.getSearch(ctx, id)
}

func (db *DB) getSearch(ctx context.Context, id int64) (Search, error) {
	var s Search
	var openedKind, openedKey sql.NullString
	err := db.sql.QueryRowContext(ctx, `
		SELECT id, query, searched_at, result_count, opened_kind, opened_key
		FROM local.searches WHERE id = ?`, id).
		Scan(&s.ID, &s.Query, &s.SearchedAt, &s.ResultCount, &openedKind, &openedKey)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	if err != nil {
		return s, err
	}
	if openedKind.Valid && openedKind.String != "" {
		s.OpenedKind = &openedKind.String
	}
	if openedKey.Valid && openedKey.String != "" {
		s.OpenedKey = &openedKey.String
	}
	return s, nil
}

// HistoryOpts filters the mixed visit+search timeline.
type HistoryOpts struct {
	// Kind is "issue", "page", or "search". Empty means all three.
	Kind   string
	Limit  int
	Cursor string
}

// HistoryItem is one row of the mixed timeline (newest first).
type HistoryItem struct {
	Type        string  `json:"type"` // visit | search
	ID          int64   `json:"id"`
	Kind        string  `json:"kind,omitempty"`
	Key         string  `json:"key,omitempty"`
	Query       string  `json:"query,omitempty"`
	ResultCount *int    `json:"result_count,omitempty"`
	OpenedKind  *string `json:"opened_kind,omitempty"`
	OpenedKey   *string `json:"opened_key,omitempty"`
	At          string  `json:"at"`
}

// HistoryPage is one cursor page of History.
type HistoryPage struct {
	Items      []HistoryItem `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 200
)

// History returns visits and searches newest-first. Cursor is opaque.
func (db *DB) History(ctx context.Context, opts HistoryOpts) (HistoryPage, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}
	kind := strings.TrimSpace(opts.Kind)
	switch kind {
	case "", VisitKindIssue, VisitKindPage, "search":
	default:
		return HistoryPage{}, fmt.Errorf("kind must be %q, %q, or %q", VisitKindIssue, VisitKindPage, "search")
	}

	cur, err := parseHistoryCursor(opts.Cursor)
	if err != nil {
		return HistoryPage{}, err
	}

	q, args := historySQL(kind, cur, limit+1)
	rows, err := db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return HistoryPage{}, err
	}
	defer rows.Close()

	out := HistoryPage{Items: []HistoryItem{}}
	for rows.Next() {
		var it HistoryItem
		var kindNS, keyNS, queryNS, openedKind, openedKey sql.NullString
		var result sql.NullInt64
		if err := rows.Scan(&it.Type, &it.ID, &kindNS, &keyNS, &queryNS, &result, &openedKind, &openedKey, &it.At); err != nil {
			return HistoryPage{}, err
		}
		it.Kind = kindNS.String
		it.Key = keyNS.String
		it.Query = queryNS.String
		if result.Valid {
			n := int(result.Int64)
			it.ResultCount = &n
		}
		if openedKind.Valid && openedKind.String != "" {
			it.OpenedKind = &openedKind.String
		}
		if openedKey.Valid && openedKey.String != "" {
			it.OpenedKey = &openedKey.String
		}
		out.Items = append(out.Items, it)
	}
	if err := rows.Err(); err != nil {
		return HistoryPage{}, err
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		out.NextCursor = formatHistoryCursor(last)
	}
	return out, nil
}

type historyCursor struct {
	at  string
	typ string
	id  int64
	set bool
}

func formatHistoryCursor(it HistoryItem) string {
	return it.At + "|" + it.Type + "|" + strconv.FormatInt(it.ID, 10)
}

func parseHistoryCursor(s string) (historyCursor, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return historyCursor{}, nil
	}
	parts := strings.Split(s, "|")
	if len(parts) != 3 {
		return historyCursor{}, ErrInvalidCursor
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return historyCursor{}, ErrInvalidCursor
	}
	if parts[1] != "visit" && parts[1] != "search" {
		return historyCursor{}, ErrInvalidCursor
	}
	return historyCursor{at: parts[0], typ: parts[1], id: id, set: true}, nil
}

func historySQL(kind string, cur historyCursor, limit int) (string, []any) {
	// Mixed timeline: visits + searches, newest first. Cursor is (at DESC, type DESC, id DESC).
	//
	// The epoch clause is the timeline's whole defence against a replaced
	// origin: rows carry the generation they were recorded under, and a bare
	// key resolves against whatever mirror is current, so showing a retired row
	// would put the new site's STD-1 behind yesterday's visit to a different
	// STD-1 (GDK-418). The rows stay in the file — `gadak sql` reads them.
	visitWhere := []string{"origin_epoch = " + currentEpochSQL}
	searchWhere := []string{"origin_epoch = " + currentEpochSQL}
	args := []any{}
	if kind == VisitKindIssue || kind == VisitKindPage {
		visitWhere = append(visitWhere, "kind = ?")
		args = append(args, kind)
		searchWhere = []string{"0"} // drop searches
	}
	if kind == "search" {
		visitWhere = []string{"0"}
	}
	visitSQL := `SELECT 'visit' AS type, id, kind, key, NULL AS query, NULL AS result_count,
		NULL AS opened_kind, NULL AS opened_key, viewed_at AS at
		FROM local.visits WHERE ` + strings.Join(visitWhere, " AND ")
	searchSQL := `SELECT 'search' AS type, id, NULL, NULL, query, result_count,
		opened_kind, opened_key, searched_at AS at
		FROM local.searches WHERE ` + strings.Join(searchWhere, " AND ")
	inner := visitSQL + " UNION ALL " + searchSQL
	outerWhere := ""
	if cur.set {
		// Rows strictly older than the cursor in (at DESC, type DESC, id DESC).
		outerWhere = ` WHERE at < ? OR (at = ? AND (type < ? OR (type = ? AND id < ?)))`
		args = append(args, cur.at, cur.at, cur.typ, cur.typ, cur.id)
	}
	q := `SELECT type, id, kind, key, query, result_count, opened_kind, opened_key, at FROM (` + inner + `)` +
		outerWhere + ` ORDER BY at DESC, type DESC, id DESC LIMIT ?`
	args = append(args, limit)
	return q, args
}

// RecentVisit is one (kind, key) pair folded to its newest visit — the row
// `gadak recents` prints. Unlike Visit there is no ID: dedup happened in SQL.
type RecentVisit struct {
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	ViewedAt string `json:"viewed_at"`
}

// RecentVisits lists the distinct (kind, key) pairs newest-first, at most
// limit: docs/MIRROR.md's "recently viewed" recipe (MAX(viewed_at) GROUP BY key)
// as a typed accessor, both kinds at once. It carries History's epoch clause
// for the same reason History does: after a workspace changes origin, a
// retired pair names a key the new origin can mint (GDK-418), and this list
// exists precisely so its reader goes on to open those keys.
func (db *DB) RecentVisits(ctx context.Context, limit int) ([]RecentVisit, error) {
	return db.recentVisits(ctx, "", limit)
}

// VisitedKeys is RecentVisits narrowed to one kind — the set a list needs to
// mark rows already opened (GDK-1344), newest first. The cap is the caller's;
// the list asks for the whole set once and answers per row from a map.
func (db *DB) VisitedKeys(ctx context.Context, kind string, limit int) ([]RecentVisit, error) {
	switch kind {
	case VisitKindIssue, VisitKindPage:
	default:
		return nil, fmt.Errorf("kind must be %q or %q", VisitKindIssue, VisitKindPage)
	}
	return db.recentVisits(ctx, kind, limit)
}

func (db *DB) recentVisits(ctx context.Context, kind string, limit int) ([]RecentVisit, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be > 0")
	}
	where := "origin_epoch = " + currentEpochSQL
	args := []any{}
	if kind != "" {
		where += " AND kind = ?"
		args = append(args, kind)
	}
	args = append(args, limit)
	rows, err := db.sql.QueryContext(ctx, `
		SELECT kind, key, MAX(viewed_at) AS viewed_at
		FROM local.visits
		WHERE `+where+`
		GROUP BY kind, key
		ORDER BY viewed_at DESC, MAX(id) DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecentVisit{}
	for rows.Next() {
		var r RecentVisit
		if err := rows.Scan(&r.Kind, &r.Key, &r.ViewedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LastVisits returns the newest n visits of one (kind, key), newest first —
// the two rows the issue detail turns into last/previous_visit_at. Person
// reads only: source ui, or the empty source of a row recorded before V7
// (unknown counts with the person — the same rule `gadak retro`'s sessions
// use). Agent cli reads are excluded on purpose: a terminal `gadak issue`
// run is not the reader returning. RecentVisits' epoch clause for the same
// reason — a visit from a replaced origin names a key the current origin
// may re-mint (GDK-418).
func (db *DB) LastVisits(ctx context.Context, kind, key string, n int) ([]Visit, error) {
	switch kind {
	case VisitKindIssue, VisitKindPage:
	default:
		return nil, fmt.Errorf("kind must be %q or %q", VisitKindIssue, VisitKindPage)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("key required")
	}
	if n <= 0 {
		return nil, errors.New("n must be > 0")
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, kind, key, viewed_at, source
		FROM local.visits
		WHERE kind = ? AND key = ? AND source IN ('ui','')
			AND origin_epoch = `+currentEpochSQL+`
		ORDER BY viewed_at DESC, id DESC
		LIMIT ?`, kind, key, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Visit{}
	for rows.Next() {
		var v Visit
		if err := rows.Scan(&v.ID, &v.Kind, &v.Key, &v.ViewedAt, &v.Source); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// lastSessionVisitsBound caps the rows LastSessionEnd walks: the newest
// person reads, newest first. A person reading more than this inside the
// current session (the only place the walk can continue) is not a shape the
// product has; the bound keeps the read O(bound) whatever the history file
// holds. localRetention prunes the table long before 2000 rows of one session
// can accumulate.
const lastSessionVisitsBound = 2000

// LastSessionEnd returns the viewed_at of the newest person read that is not
// part of the session containing `now` — i.e. where the previous session
// ended. nil when there is no previous session (zero visits, or only the
// current session's reads).
//
// The session rule is retro's (cmd/gadak/retro.go retroSessionGap): person
// reads are visits with source ui or the empty pre-V7 source, and a gap to
// the previous read *exceeding* `gap` starts a new session — exactly `gap` is
// still the same session. The current session is the chain walked backwards
// from `now` while every step is ≤ gap (the chain may be empty); a mid-session
// tab reload only lengthens that chain, so the boundary stays the previous
// session's last read. Person reads only, and LastVisits' epoch clause for
// the same GDK-418 reason: a visit from a replaced origin is not this
// workspace's session.
func (db *DB) LastSessionEnd(ctx context.Context, now time.Time, gap time.Duration) (*time.Time, error) {
	if gap <= 0 {
		return nil, errors.New("gap must be > 0")
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT viewed_at
		FROM local.visits
		WHERE source IN ('ui','') AND origin_epoch = `+currentEpochSQL+`
		ORDER BY viewed_at DESC, id DESC
		LIMIT ?`, lastSessionVisitsBound)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Parse then re-sort: viewed_at is ISOMilli UTC in practice, where string
	// order is time order, but rows written by older builds (parseStamp's
	// RFC3339 fallback) must not be walked in string order.
	var stamps []time.Time
	for rows.Next() {
		var at string
		if err := rows.Scan(&at); err != nil {
			return nil, err
		}
		if t, ok := parseStamp(at); ok {
			stamps = append(stamps, t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(stamps, func(i, j int) bool { return stamps[j].Before(stamps[i]) })
	prev := now
	for _, t := range stamps {
		if prev.Sub(t) <= gap {
			// Same session as `now`'s chain — keep walking.
			prev = t
			continue
		}
		// First read before a >gap break: the previous session's newest read.
		return &t, nil
	}
	return nil, nil
}

// PruneLocalHistory deletes visit/search rows older than localRetention.
// Called from the same pass as tombstone expiry (DeleteItems).
func (db *DB) PruneLocalHistory(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-localRetention).Format(config.ISOMilli)
	return db.write(ctx, func(tx *sql.Tx) error {
		return pruneLocalHistoryTx(ctx, tx, cutoff)
	})
}

func pruneLocalHistoryTx(ctx context.Context, tx *sql.Tx, cutoff string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM local.visits WHERE viewed_at < ?`, cutoff); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM local.searches WHERE searched_at < ?`, cutoff)
	return err
}

// recentCap is the per-kind ceiling. Matches web/src/lib/recency.ts MAX.
const recentCap = 10

// Recent is one (kind, value) pair in local.recents. Unlike visits, the same
// pair is one row: recording it again only moves used_at to now.
type Recent struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	UsedAt string `json:"used_at"`
}

// RecordRecent puts value at the front of kind (de-dup, cap recentCap).
// Empty kind or value is refused. Kind is an opaque string — same names the
// web helper already used (assignee, transition:<project>, create-type:<project>, …).
func (db *DB) RecordRecent(ctx context.Context, kind, value string) (Recent, error) {
	var zero Recent
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if kind == "" {
		return zero, errors.New("kind required")
	}
	if value == "" {
		return zero, errors.New("value required")
	}
	at := Now()
	err := db.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO local.recents (kind, value, used_at) VALUES (?,?,?)
			ON CONFLICT(kind, value) DO UPDATE SET used_at = excluded.used_at`,
			kind, value, at); err != nil {
			return err
		}
		return trimRecentsKind(ctx, tx, kind)
	})
	if err != nil {
		return zero, err
	}
	return Recent{Kind: kind, Value: value, UsedAt: at}, nil
}

// Recents returns recent-use rows newest-first. Empty kind lists every kind.
func (db *DB) Recents(ctx context.Context, kind string) ([]Recent, error) {
	kind = strings.TrimSpace(kind)
	q := `SELECT kind, value, used_at FROM local.recents`
	args := []any{}
	if kind != "" {
		q += ` WHERE kind = ?`
		args = append(args, kind)
	}
	q += ` ORDER BY used_at DESC, id DESC`
	rows, err := db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recent{}
	for rows.Next() {
		var r Recent
		if err := rows.Scan(&r.Kind, &r.Value, &r.UsedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AbsorbRecents merges a localStorage dump into local.recents. Existing
// server rows stay in front (they are already the owner); incoming values
// not already present fill the remainder, still capped at recentCap per kind.
// Incoming slices are newest-first, same as recentOf().
func (db *DB) AbsorbRecents(ctx context.Context, kinds map[string][]string) error {
	if len(kinds) == 0 {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		for kind, incoming := range kinds {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			existing, err := recentsKindTx(ctx, tx, kind)
			if err != nil {
				return err
			}
			merged := mergeRecentValues(existing, incoming)
			if err := replaceKindRecents(ctx, tx, kind, merged); err != nil {
				return err
			}
		}
		return nil
	})
}

// ImportRecents upserts export-file rows. File used_at wins for a (kind, value)
// pair; each kind is then trimmed to recentCap newest. Local-only pairs stay
// if they still fit in the cap.
func (db *DB) ImportRecents(ctx context.Context, items []Recent) error {
	if len(items) == 0 {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		affected := map[string]bool{}
		for _, it := range items {
			kind := strings.TrimSpace(it.Kind)
			value := strings.TrimSpace(it.Value)
			if kind == "" || value == "" {
				continue
			}
			at := strings.TrimSpace(it.UsedAt)
			if at == "" {
				at = Now()
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO local.recents (kind, value, used_at) VALUES (?,?,?)
				ON CONFLICT(kind, value) DO UPDATE SET used_at = excluded.used_at`,
				kind, value, at); err != nil {
				return err
			}
			affected[kind] = true
		}
		for kind := range affected {
			if err := trimRecentsKind(ctx, tx, kind); err != nil {
				return err
			}
		}
		return nil
	})
}

func recentsKindTx(ctx context.Context, tx *sql.Tx, kind string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT value FROM local.recents
		WHERE kind = ?
		ORDER BY used_at DESC, id DESC`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func mergeRecentValues(existing, incoming []string) []string {
	seen := make(map[string]bool, recentCap)
	out := make([]string, 0, recentCap)
	add := func(v string) bool {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return true
		}
		seen[v] = true
		out = append(out, v)
		return len(out) < recentCap
	}
	for _, v := range existing {
		if !add(v) {
			return out
		}
	}
	for _, v := range incoming {
		if !add(v) {
			return out
		}
	}
	return out
}

func replaceKindRecents(ctx context.Context, tx *sql.Tx, kind string, values []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM local.recents WHERE kind = ?`, kind); err != nil {
		return err
	}
	base := time.Now().UTC()
	for i, v := range values {
		at := base.Add(-time.Duration(i) * time.Millisecond).Format(config.ISOMilli)
		if _, err := tx.ExecContext(ctx, `INSERT INTO local.recents (kind, value, used_at) VALUES (?,?,?)`, kind, v, at); err != nil {
			return err
		}
	}
	return nil
}

func trimRecentsKind(ctx context.Context, tx *sql.Tx, kind string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM local.recents
		WHERE kind = ?
		ORDER BY used_at DESC, id DESC`, kind)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(ids) <= recentCap {
		return nil
	}
	for _, id := range ids[recentCap:] {
		if _, err := tx.ExecContext(ctx, `DELETE FROM local.recents WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// Recipe is a named read-only SQL query in local.recipes (GDK-503).
type Recipe struct {
	Name      string `json:"name"`
	SQL       string `json:"sql"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// recipeNameRe is the save-time identifier: start with a letter so a name
// cannot look like a flag, charset [a-z0-9-] as specified, cap 64 like
// pairing labels (internal/pairing/store.go). Neighbors searched:
// config.themeIdentRe (`^[a-z0-9-]{1,32}$`), deeplink actionPattern
// (`^[a-z][a-z0-9-]{0,31}$`).
var recipeNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// ValidateRecipeName is the recipes save NAME rule. Empty and anything
// outside recipeNameRe are refused.
func ValidateRecipeName(name string) error {
	if !recipeNameRe.MatchString(name) {
		return fmt.Errorf("recipe name must match [a-z][a-z0-9-]{0,63} (got %q)", name)
	}
	return nil
}

// Recipes lists every recipe, newest update first.
func (db *DB) Recipes(ctx context.Context) ([]Recipe, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT name, sql, created_at, updated_at FROM local.recipes
		ORDER BY updated_at DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recipe{}
	for rows.Next() {
		var r Recipe
		if err := rows.Scan(&r.Name, &r.SQL, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecipeNames is the saved-name list a missing-name error quotes.
func (db *DB) RecipeNames(ctx context.Context) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT name FROM local.recipes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Recipe looks up one named recipe. Unknown name is ErrNotFound.
func (db *DB) Recipe(ctx context.Context, name string) (Recipe, error) {
	var r Recipe
	err := db.sql.QueryRowContext(ctx, `
		SELECT name, sql, created_at, updated_at FROM local.recipes WHERE name = ?`, name).
		Scan(&r.Name, &r.SQL, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// PutRecipe inserts or overwrites a recipe. Same name keeps created_at and
// refreshes sql / updated_at. Name and SQL are validated here so every
// writer shares the rule.
func (db *DB) PutRecipe(ctx context.Context, name, query string) (Recipe, error) {
	var zero Recipe
	if err := ValidateRecipeName(name); err != nil {
		return zero, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return zero, errors.New("recipe sql is empty")
	}
	now := Now()
	err := db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO local.recipes (name, sql, created_at, updated_at) VALUES (?,?,?,?)
			ON CONFLICT(name) DO UPDATE SET sql = excluded.sql, updated_at = excluded.updated_at`,
			name, query, now, now)
		return err
	})
	if err != nil {
		return zero, err
	}
	return db.Recipe(ctx, name)
}

// DeleteRecipe removes one recipe. Unknown name is ErrNotFound.
func (db *DB) DeleteRecipe(ctx context.Context, name string) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM local.recipes WHERE name = ?`, name)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ReadOnly opens this DB's mirror with OpenReadOnly — mode=ro, local.db
// attached — for executing untrusted SQL (dashboard datasources). The single
// purpose is that a datasource statement cannot take the mirror's write lock
// or mutate a row no matter what the config contains: the "writes pass
// through origin" invariant's integrity axis for arbitrary SQL.
func (db *DB) ReadOnly() (*sql.DB, error) {
	return OpenReadOnly(db.path)
}

// Dashboard is one row of local.dashboards (GDK-781). Config is the
// {html, datasources} document internal/dashboards owns; the store keeps it
// as raw bytes so a newer config shape round-trips through an older binary.
type Dashboard struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedAt string          `json:"created_at,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
}

// dashboardsVersionKey is the local_meta row every dashboard write bumps.
// The GET list response exposes it so an open tab can poll "did anything
// change" with one integer instead of diffing the list.
const dashboardsVersionKey = "dashboards_version"

// bumpDashboardsVersion advances the change counter inside the caller's
// transaction, so the row and the version land together or not at all.
func bumpDashboardsVersion(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO local.local_meta (k, v) VALUES (?, '1')
		ON CONFLICT(k) DO UPDATE SET v = CAST(CAST(v AS INTEGER) + 1 AS TEXT)`,
		dashboardsVersionKey)
	return err
}

// SaveDashboard inserts a dashboard, or updates the row that already owns
// name (keeping its id and created_at). The id is minted here so the CLI and
// the API cannot disagree about who owns id generation. Config is stored
// verbatim; validation of the document is internal/dashboards' job, at every
// writer that accepts one from a user.
func (db *DB) SaveDashboard(ctx context.Context, name, config string) (Dashboard, error) {
	var zero Dashboard
	name = strings.TrimSpace(name)
	if name == "" {
		return zero, errors.New("dashboard name required")
	}
	now := Now()
	var out Dashboard
	err := db.write(ctx, func(tx *sql.Tx) error {
		var id, createdAt string
		err := tx.QueryRowContext(ctx,
			`SELECT id, COALESCE(created_at, '') FROM local.dashboards WHERE name = ?`, name).
			Scan(&id, &createdAt)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			id = freshViewID()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO local.dashboards (id, name, config, created_at, updated_at) VALUES (?,?,?,?,?)`,
				id, name, config, now, now); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if createdAt == "" {
				createdAt = now
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE local.dashboards SET config = ?, updated_at = ? WHERE id = ?`,
				config, now, id); err != nil {
				return err
			}
		}
		out = Dashboard{ID: id, Name: name, Config: json.RawMessage(config), CreatedAt: createdAt, UpdatedAt: now}
		return bumpDashboardsVersion(ctx, tx)
	})
	if err != nil {
		return zero, err
	}
	return out, nil
}

// Dashboards lists every dashboard by name.
func (db *DB) Dashboards(ctx context.Context) ([]Dashboard, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, name, config, created_at, updated_at FROM local.dashboards
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Dashboard{}
	for rows.Next() {
		var d Dashboard
		var cfg string
		var created, updated sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &cfg, &created, &updated); err != nil {
			return nil, err
		}
		d.Config = json.RawMessage(cfg)
		d.CreatedAt, d.UpdatedAt = created.String, updated.String
		out = append(out, d)
	}
	return out, rows.Err()
}

// Dashboard looks up one row by id. Unknown id is ErrNotFound.
func (db *DB) Dashboard(ctx context.Context, id string) (Dashboard, error) {
	var d Dashboard
	var cfg string
	var created, updated sql.NullString
	err := db.sql.QueryRowContext(ctx, `
		SELECT id, name, config, created_at, updated_at FROM local.dashboards WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &cfg, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.Config = json.RawMessage(cfg)
	d.CreatedAt, d.UpdatedAt = created.String, updated.String
	return d, nil
}

// DeleteDashboard removes one row by id and bumps the change counter. Unknown
// id is ErrNotFound.
func (db *DB) DeleteDashboard(ctx context.Context, id string) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM local.dashboards WHERE id = ?`, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return bumpDashboardsVersion(ctx, tx)
	})
}

// DashboardVersion is the monotonic change counter for dashboard writes
// (0 before the first save). It moves on save and delete — the two events an
// open tab must reflect.
func (db *DB) DashboardVersion(ctx context.Context) (int64, error) {
	var v sql.NullString
	err := db.sql.QueryRowContext(ctx,
		`SELECT v FROM local.local_meta WHERE k = ?`, dashboardsVersionKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v.String), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("local.dashboards version counter: %w", err)
	}
	return n, nil
}

// AbsorbDashboards merges incoming rows (an export file, another machine)
// into local.dashboards — AbsorbViews, for dashboards. Same rules: a name
// the server already owns wins (incoming row dropped), an id that collides
// gets a fresh one so an insert can never overwrite a stored row.
func (db *DB) AbsorbDashboards(ctx context.Context, incoming []Dashboard) error {
	if len(incoming) == 0 {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		for _, d := range incoming {
			d.Name = strings.TrimSpace(d.Name)
			if d.ID == "" || d.Name == "" || len(d.Config) == 0 {
				continue
			}
			var one int
			err := tx.QueryRowContext(ctx,
				`SELECT 1 FROM local.dashboards WHERE name = ? LIMIT 1`, d.Name).Scan(&one)
			if err == nil {
				continue // a stored row already owns this name
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			id := d.ID
			err = tx.QueryRowContext(ctx,
				`SELECT 1 FROM local.dashboards WHERE id = ? LIMIT 1`, id).Scan(&one)
			if err == nil {
				id = freshViewID()
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			at := Now()
			if d.CreatedAt == "" {
				d.CreatedAt = at
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO local.dashboards (id, name, config, created_at, updated_at) VALUES (?,?,?,?,?)`,
				id, d.Name, string(d.Config), d.CreatedAt, at); err != nil {
				return err
			}
		}
		return bumpDashboardsVersion(ctx, tx)
	})
}
