package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"

	sqlite "modernc.org/sqlite"
)

// localDBFile is the personal-history SQLite file, next to the mirror.
// Never sent to Jira, never copied by snapshot (a separate file, not a table).
const localDBFile = "local.db"

// LocalRetention is how long raw visit/search events are kept. Counts older
// than this window are not rolled up — derive from whatever is still here.
const LocalRetention = 180 * 24 * time.Hour

// localMigrations is independent of the mirror's migrations slice. Index+1 is
// PRAGMA user_version on local.db.
var localMigrations = []string{localSchemaV1}

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

func init() {
	// Single ATTACH owner: every connection the registered "sqlite" driver
	// opens (store.Open, gadak sql, MCP gadak_query, raw tests) goes through
	// here. Creation/migration of the file is EnsureLocal; this hook only
	// attaches an existing sibling and never fails the connection.
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
		// Already attached (pool reuse) or permission/path — history off, mirror on.
		if !strings.Contains(err.Error(), "already in use") {
			log.Printf("store: ATTACH local.db: %v", err)
		}
	}
	return nil
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

func ensureLocalDB(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := ensurePrivateDir(dir); err != nil {
			return err
		}
	}
	// DELETE journal: this file is small and opened from many short-lived
	// connections (sql, MCP). WAL sidecars next to a test TempDir have been
	// seen to survive Close and fail cleanup; the mirror stays WAL.
	dsn := "file:" + path + "?" + strings.Join([]string{
		"_pragma=busy_timeout(5000)",
		"_pragma=journal_mode(DELETE)",
		"_pragma=synchronous(NORMAL)",
	}, "&")
	sqlDB, err := sql.Open("sqlite", dsn)
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
		return fmt.Errorf("%s: schema version %d is newer than this build of gadak supports (%d)", path, have, want)
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
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

// VisitKind is visits.kind / searches.opened_kind: "issue" or "page".
const (
	VisitKindIssue = "issue"
	VisitKindPage  = "page"
)

func validVisitKind(k string) bool {
	return k == VisitKindIssue || k == VisitKindPage
}

// Visit is one append-only view event.
type Visit struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	ViewedAt string `json:"viewed_at"`
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

// RecordVisit appends one view. Same (kind, key) twice is two rows.
func (db *DB) RecordVisit(ctx context.Context, kind, key string) (Visit, error) {
	var zero Visit
	if !validVisitKind(kind) {
		return zero, fmt.Errorf("kind must be %q or %q", VisitKindIssue, VisitKindPage)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return zero, errors.New("key required")
	}
	at := Now()
	var id int64
	err := db.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO local.visits (kind, key, viewed_at) VALUES (?,?,?)`, kind, key, at)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return zero, err
	}
	return Visit{ID: id, Kind: kind, Key: key, ViewedAt: at}, nil
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
	err := db.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO local.searches (query, searched_at, result_count, opened_kind, opened_key)
			VALUES (?,?,?,?,?)`,
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
	err := db.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE local.searches SET opened_kind = ?, opened_key = ? WHERE id = ?`, kind, key, id)
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
		return historyCursor{}, errors.New("invalid cursor")
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return historyCursor{}, errors.New("invalid cursor")
	}
	if parts[1] != "visit" && parts[1] != "search" {
		return historyCursor{}, errors.New("invalid cursor")
	}
	return historyCursor{at: parts[0], typ: parts[1], id: id, set: true}, nil
}

func historySQL(kind string, cur historyCursor, limit int) (string, []any) {
	// Mixed timeline: visits + searches, newest first. Cursor is (at DESC, type DESC, id DESC).
	visitWhere := []string{"1=1"}
	searchWhere := []string{"1=1"}
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

// PruneLocalHistory deletes visit/search rows older than LocalRetention.
// Called from the same pass as tombstone expiry (DeleteItems).
func (db *DB) PruneLocalHistory(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-LocalRetention).Format(config.ISOMilli)
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
