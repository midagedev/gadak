package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// TestOpenCreatesLocalSchemaAndSelectWorks is FAIL-first: before local.db,
// SELECT local.visits after Open was "no such table".
func TestOpenCreatesLocalSchemaAndSelectWorks(t *testing.T) {
	db := openTemp(t)
	var n int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM local.visits`).Scan(&n); err != nil {
		t.Fatalf("local.visits after Open: %v", err)
	}
	if n != 0 {
		t.Fatalf("fresh visits count = %d, want 0", n)
	}
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM local.searches`).Scan(&n); err != nil {
		t.Fatalf("local.searches after Open: %v", err)
	}
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM local.recents`).Scan(&n); err != nil {
		t.Fatalf("local.recents after Open: %v", err)
	}
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM local.recipes`).Scan(&n); err != nil {
		t.Fatalf("local.recipes after Open: %v", err)
	}
}

// Existing install: mirror only, no local.db. Open must recreate the schema
// rather than fail, and SELECT local.visits must work.
func TestOpenRecreatesMissingLocalDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	local := LocalPath(path)
	if err := os.Remove(local); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	_ = os.Remove(local + "-wal")
	_ = os.Remove(local + "-shm")

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open after local.db removed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM local.visits`).Scan(&n); err != nil {
		t.Fatalf("local.visits after recreate: %v", err)
	}
}

func TestOpenReadOnlyCreatesMissingLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	local := LocalPath(path)
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ro.Close() })
	var n int
	if err := ro.QueryRow(`SELECT COUNT(*) FROM local.visits`).Scan(&n); err != nil {
		t.Fatalf("read-only after missing local.db: %v", err)
	}
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("OpenReadOnly should create empty local.db: %v", err)
	}
}

func TestOpenReadOnlySelectsLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(context.Background(), "issue", "NMB-1"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ro.Close() })
	var key string
	if err := ro.QueryRow(`SELECT key FROM local.visits`).Scan(&key); err != nil {
		t.Fatalf("read-only local.visits: %v", err)
	}
	if key != "NMB-1" {
		t.Fatalf("key = %q", key)
	}
}

// MCP / gadak sql open sqlite with mode=ro themselves. Once local.db exists,
// that connection must see local.* without calling store.Open.
// MCP opens `file:<gadak.db>?mode=ro` itself. An existing-user mirror
// without local.db must still answer SELECT local.visits.
func TestRawReadOnlyGadakDBCreatesMissingLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.Remove(LocalPath(path)); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM local.visits`).Scan(&n); err != nil {
		t.Fatalf("MCP-shaped open after missing local.db: %v", err)
	}
}

func TestRawReadOnlyConnectionSeesLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(context.Background(), "issue", "NMB-9"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	var key string
	if err := raw.QueryRow(`SELECT key FROM local.visits`).Scan(&key); err != nil {
		t.Fatalf("raw mode=ro local.visits: %v", err)
	}
	if key != "NMB-9" {
		t.Fatalf("key = %q", key)
	}
}

func TestRecordVisitIsAppendOnly(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := db.RecordVisit(ctx, "issue", "NMB-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(ctx, "issue", "NMB-1"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM local.visits WHERE kind = 'issue' AND key = 'NMB-1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("visit count = %d, want 2 (append, not upsert)", n)
	}
}

func TestRecordSearchAndSetOpened(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	got, err := db.RecordSearch(ctx, "flaky upload", 3, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 {
		t.Fatal("expected non-zero search id")
	}
	if _, err := db.SetSearchOpened(ctx, got.ID, "issue", "NMB-1"); err != nil {
		t.Fatal(err)
	}
	id := got.ID
	var q, kind, key string
	var n int
	if err := db.sql.QueryRowContext(ctx, `
		SELECT query, result_count, opened_kind, opened_key FROM local.searches WHERE id = ?`, id).
		Scan(&q, &n, &kind, &key); err != nil {
		t.Fatal(err)
	}
	if q != "flaky upload" || n != 3 || kind != "issue" || key != "NMB-1" {
		t.Fatalf("search row = %q %d %q %q", q, n, kind, key)
	}
}

func TestPruneLocalHistoryDropsOnlyOldRows(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-200 * 24 * time.Hour).Format(config.ISOMilli)
	recent := Now()
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO local.visits (kind, key, viewed_at) VALUES ('issue', 'OLD-1', ?), ('issue', 'NEW-1', ?)`, old, recent); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO local.searches (query, searched_at, result_count) VALUES ('old q', ?, 1), ('new q', ?, 1)`, old, recent); err != nil {
		t.Fatal(err)
	}
	if err := db.PruneLocalHistory(ctx); err != nil {
		t.Fatal(err)
	}
	var keys string
	if err := db.sql.QueryRowContext(ctx, `SELECT GROUP_CONCAT(key) FROM local.visits`).Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if keys != "NEW-1" {
		t.Fatalf("visits after prune = %q, want NEW-1", keys)
	}
	var qs string
	if err := db.sql.QueryRowContext(ctx, `SELECT GROUP_CONCAT(query) FROM local.searches`).Scan(&qs); err != nil {
		t.Fatal(err)
	}
	if qs != "new q" {
		t.Fatalf("searches after prune = %q, want new q", qs)
	}
}

func TestPruneLocalHistoryWiredToDeleteItems(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	ctx := context.Background()
	old := time.Now().UTC().Add(-200 * 24 * time.Hour).Format(config.ISOMilli)
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO local.visits (kind, key, viewed_at) VALUES ('issue', 'OLD-1', ?)`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteItems(ctx, "jira", []string{"NMB-2"}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM local.visits WHERE key = 'OLD-1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("old visit survived DeleteItems prune wiring, count=%d", n)
	}
}

func TestLocalDBSurvivesMirrorDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(context.Background(), "issue", "NMB-keep"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Remove(p)
	}
	if _, err := os.Stat(LocalPath(path)); err != nil {
		t.Fatalf("local.db missing after rm gadak.db: %v", err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var key string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT key FROM local.visits`).Scan(&key); err != nil {
		t.Fatalf("visit after remirroring: %v", err)
	}
	if key != "NMB-keep" {
		t.Fatalf("key = %q, want NMB-keep", key)
	}
}

func TestOpenSucceedsWhenLocalAttachImpossible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	// A directory where local.db should be makes ATTACH/create fail.
	if err := os.Mkdir(LocalPath(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open must succeed without history: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM issues`).Scan(&n); err != nil {
		t.Fatalf("mirror query after failed local attach: %v", err)
	}
}

func TestLocalDBFileMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if got := fileMode(t, LocalPath(path)); got != 0o600 {
		t.Errorf("local.db mode = %04o, want 0600", got)
	}
}

func TestHistoryListCursorAndKind(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := db.RecordVisit(ctx, "issue", "NMB-1"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := db.RecordVisit(ctx, "page", "622723"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := db.RecordSearch(ctx, "retry", 4, "issue", "NMB-1"); err != nil {
		t.Fatal(err)
	}

	all, err := db.History(ctx, HistoryOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Items) != 3 {
		t.Fatalf("mixed items = %d, want 3", len(all.Items))
	}
	if all.Items[0].Type != "search" || all.Items[0].Query != "retry" {
		t.Fatalf("newest = %+v, want search retry", all.Items[0])
	}

	issues, err := db.History(ctx, HistoryOpts{Kind: "issue"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues.Items) != 1 || issues.Items[0].Key != "NMB-1" {
		t.Fatalf("kind=issue: %+v", issues.Items)
	}

	page1, err := db.History(ctx, HistoryOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 1 || page1.NextCursor == "" {
		t.Fatalf("page1 = %+v", page1)
	}
	page2, err := db.History(ctx, HistoryOpts{Limit: 10, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("page2 items = %d, want 2", len(page2.Items))
	}
}

// TestHistoryInvalidCursorSentinel is FAIL-first (GDK-609): all three cursor
// parse failures used to return fresh errors.New("invalid cursor") values, so
// errors.Is against the sentinel failed. The server's 400 branch keys on this
// identity; matching the message text there was the fragile old path.
func TestHistoryInvalidCursorSentinel(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for _, cur := range []string{
		"no-pipes",                        // not three fields
		"2026-08-22T00:00:00Z|visit|oops", // id is not an integer
		"2026-08-22T00:00:00Z|block|7",    // type is neither visit nor search
	} {
		if _, err := db.History(ctx, HistoryOpts{Cursor: cur}); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("cursor %q: err = %v, want ErrInvalidCursor", cur, err)
		}
	}
}

func TestRecentVisits(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	visit := func(kind, key string) {
		t.Helper()
		if _, err := db.RecordVisit(ctx, kind, key); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct viewed_at, as in the History test
	}
	visit("issue", "NMB-1")
	visit("page", "622723")
	visit("issue", "NMB-1") // same pair again: still one row, at the newer time

	rows, err := db.RecentVisits(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (the repeated pair folds to one)", len(rows))
	}
	if rows[0].Key != "NMB-1" || rows[0].ViewedAt == "" {
		t.Fatalf("newest = %+v, want NMB-1 at its second visit's time", rows[0])
	}
	if rows[1].Kind != "page" || rows[1].Key != "622723" {
		t.Fatalf("second = %+v, want the page visit", rows[1])
	}

	one, err := db.RecentVisits(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Key != "NMB-1" {
		t.Fatalf("limit 1 = %+v", one)
	}
	if _, err := db.RecentVisits(ctx, 0); err == nil {
		t.Fatal("limit 0: want an error, got none")
	}
}

// TestRecentVisitsSkipsRetiredEpoch pins the GDK-418 defence RecentVisits
// shares with History: a visit row from before an origin replacement names a
// key the new origin can mint, and this list exists so its reader opens keys.
func TestRecentVisitsSkipsRetiredEpoch(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO local.local_meta (k, v) VALUES ('origin_epoch', '1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx, `INSERT INTO local.visits (kind, key, viewed_at, origin_epoch)
		VALUES ('issue', 'OLD-1', '2026-08-22T00:00:00.000Z', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(ctx, "issue", "NEW-1"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.RecentVisits(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != "NEW-1" {
		t.Fatalf("rows = %+v, want only the current-epoch visit", rows)
	}
}

func TestRecordVisitRejectsBadKind(t *testing.T) {
	db := openTemp(t)
	_, err := db.RecordVisit(context.Background(), "ticket", "NMB-1")
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("got %v, want kind error", err)
	}
}

// TestRecordRecentReadableViaSQL is the GDK-224 parity gate: a value written
// the way the web/API will write it must come back from gadak sql (the CLI
// / Raycast / MCP path). FAIL-first: before localSchemaV2 this SELECT was
// "no such table: local.recents".
func TestRecordRecentReadableViaSQL(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := db.RecordRecent(ctx, "create-type:NMB", "10002"); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.sql.QueryRowContext(ctx, `
		SELECT value FROM local.recents
		WHERE kind = 'create-type:NMB'
		ORDER BY used_at DESC, id DESC
		LIMIT 1`).Scan(&value); err != nil {
		t.Fatalf("SQL read after RecordRecent: %v", err)
	}
	if value != "10002" {
		t.Fatalf("value = %q, want 10002", value)
	}
}

func TestRecordRecentDedupCapAndOrder(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := db.RecordRecent(ctx, "assignee", "old"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := db.RecordRecent(ctx, "assignee", "mid"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := db.RecordRecent(ctx, "assignee", "old"); err != nil {
		t.Fatal(err)
	}
	got, err := db.Recents(ctx, "assignee")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Value != "old" || got[1].Value != "mid" {
		t.Fatalf("after re-record old: %+v", got)
	}

	for i := 0; i < RecentCap+3; i++ {
		time.Sleep(time.Millisecond)
		if _, err := db.RecordRecent(ctx, "label", "l"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	labels, err := db.Recents(ctx, "label")
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != RecentCap {
		t.Fatalf("label count = %d, want %d", len(labels), RecentCap)
	}
}

func TestAbsorbRecentsFillsWithoutPromotingOverExisting(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := db.RecordRecent(ctx, "create-type:NMB", "server-first"); err != nil {
		t.Fatal(err)
	}
	if err := db.AbsorbRecents(ctx, map[string][]string{
		"create-type:NMB": {"ls-new", "server-first", "ls-old"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := db.Recents(ctx, "create-type:NMB")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("items = %+v", got)
	}
	if got[0].Value != "server-first" {
		t.Fatalf("server row must stay first, got %+v", got)
	}
	if got[1].Value != "ls-new" || got[2].Value != "ls-old" {
		t.Fatalf("absorbed fill order: %+v", got)
	}

	if err := db.AbsorbRecents(ctx, map[string][]string{
		"create-type:NMB": {"ls-new", "server-first"},
	}); err != nil {
		t.Fatal(err)
	}
	again, err := db.Recents(ctx, "create-type:NMB")
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Fatalf("re-absorb must not drop or duplicate: %+v", again)
	}
}

func TestLocalMigrationV2AddsRecents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	local := LocalPath(path)
	raw, err := sql.Open("sqlite", "file:"+local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(localSchemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM local.recents`).Scan(&n); err != nil {
		t.Fatalf("recents after v1→v2: %v", err)
	}
}

func TestRecordRecentRejectsEmpty(t *testing.T) {
	db := openTemp(t)
	if _, err := db.RecordRecent(context.Background(), "", "x"); err == nil {
		t.Fatal("empty kind accepted")
	}
	if _, err := db.RecordRecent(context.Background(), "assignee", ""); err == nil {
		t.Fatal("empty value accepted")
	}
}

// GDK-285: re-ATTACH of schema `local` must be silent without sniffing the
// driver's English sentence. The fake error is SQLITE_ERROR (1) with no
// "already in use" prose — that is the sentence the old classifier keyed on.
func TestAttachReuseIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	conn := hookConn{
		exec: func(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
			if strings.HasPrefix(q, "ATTACH ") {
				return nil, codeErr{code: 1, msg: "SQL logic error (1)"}
			}
			return driver.RowsAffected(0), nil
		},
		query: func(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
			if strings.Contains(q, "database_list") {
				return &valueRows{
					cols: []string{"seq", "name", "file"},
					data: [][]driver.Value{
						{int64(0), "main", path},
						{int64(2), "local", LocalPath(path)},
					},
				}, nil
			}
			return nil, fmt.Errorf("unexpected query %q", q)
		},
	}
	before := LocalAttachReuses()
	got := captureLog(t, func() {
		if err := attachLocalHook(conn, "file:"+path); err != nil {
			t.Errorf("hook: %v", err)
		}
	})
	if strings.Contains(got, "ATTACH local.db") {
		t.Fatalf("reuse path logged %q", got)
	}
	if LocalAttachReuses() != before+1 {
		t.Fatalf("LocalAttachReuses %d → %d, want +1", before, LocalAttachReuses())
	}

	// Same connection, real driver: hook already attached on Open; calling it
	// again is the re-ATTACH the production comment calls "pool reuse".
	live, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	live.sql.SetMaxOpenConns(1)
	before = LocalAttachReuses()
	got = captureLog(t, func() {
		if err := attachLocalHook(sqlExecQuerier{live.sql}, "file:"+path); err != nil {
			t.Errorf("live hook: %v", err)
		}
	})
	if strings.Contains(got, "ATTACH local.db") {
		t.Fatalf("live reuse path logged %q", got)
	}
	if LocalAttachReuses() != before+1 {
		t.Fatalf("live LocalAttachReuses %d → %d, want +1", before, LocalAttachReuses())
	}
}

// GDK-285: a real ATTACH failure must still be logged. Two shapes: a path
// that cannot be attached, and an error whose prose looks like reuse while
// PRAGMA database_list says `local` is absent (the old string sniff swallows
// that second one).
func TestAttachFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	if err := os.Mkdir(LocalPath(path), 0o700); err != nil {
		t.Fatal(err)
	}
	var opened *DB
	got := captureLog(t, func() {
		db, err := Open(path)
		if err != nil {
			t.Errorf("Open must succeed without history: %v", err)
			return
		}
		opened = db
	})
	if opened != nil {
		opened.Close()
	}
	if !strings.Contains(got, "ATTACH local.db") && !strings.Contains(got, "store: local.db") {
		t.Fatalf("unattachable path swallowed; log=%q", got)
	}

	// Error text matches the old sniff, but the schema is not attached.
	okPath := filepath.Join(t.TempDir(), "gadak.db")
	ready, err := Open(okPath)
	if err != nil {
		t.Fatal(err)
	}
	ready.Close()
	conn := hookConn{
		exec: func(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
			if strings.HasPrefix(q, "ATTACH ") {
				return nil, codeErr{code: 1, msg: "database local is already in use"}
			}
			return driver.RowsAffected(0), nil
		},
		query: func(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
			if strings.Contains(q, "database_list") {
				return &valueRows{
					cols: []string{"seq", "name", "file"},
					data: [][]driver.Value{
						{int64(0), "main", okPath},
					},
				}, nil
			}
			return nil, fmt.Errorf("unexpected query %q", q)
		},
	}
	before := LocalAttachReuses()
	got = captureLog(t, func() {
		if err := attachLocalHook(conn, "file:"+okPath); err != nil {
			t.Errorf("hook: %v", err)
		}
	})
	if !strings.Contains(got, "ATTACH local.db") {
		t.Fatalf("genuine failure with reuse-shaped prose was swallowed; log=%q", got)
	}
	if LocalAttachReuses() != before {
		t.Fatalf("LocalAttachReuses moved on a real failure: %d → %d", before, LocalAttachReuses())
	}
}

func TestLocalAttachDoesNotSniffDriverProse(t *testing.T) {
	src, err := os.ReadFile("local.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), `strings.Contains(err.Error()`) {
		t.Fatal("local.go still decides from err.Error() prose")
	}
	if strings.Contains(string(src), "already in use") {
		t.Fatal("local.go still names the driver's English sentence")
	}
}

// GDK-310: a newer local.db must warn once per path, not once per Open.
// gadak sql already opens twice (cmdSQL + warnIfStale → OpenReadOnly each).
// Modelled on TestAttachReuseIsSilent / TestAttachFailureIsReported (captureLog).
func TestNewerLocalDBWarnsOncePerPath(t *testing.T) {
	path := newerLocalFixture(t, len(localMigrations)+1)
	local := LocalPath(path)
	before := LocalNewerSchemaWarnsSuppressed()
	got := captureLog(t, func() {
		openReadOnlyQuery(t, path)
		openReadOnlyQuery(t, path)
	})
	const prefix = "store: local.db:"
	if n := strings.Count(got, prefix); n != 1 {
		t.Errorf("newer-local warning count = %d, want 1; log=%q", n, got)
	}
	if !strings.Contains(got, local) {
		t.Errorf("warning must name the path %q; log=%q", local, got)
	}
	if strings.Contains(got, "--db") {
		t.Errorf("local.db advice must not mention --db (demo/export-static only); log=%q", got)
	}
	if !strings.Contains(got, "upgrade gadak") {
		t.Errorf("message must say what to do (upgrade gadak), not only restate versions; log=%q", got)
	}
	if !strings.Contains(got, "--workspace") || !strings.Contains(got, "GADAK_HOME") {
		t.Errorf("message must name --workspace / GADAK_HOME; log=%q", got)
	}
	if gotn := LocalNewerSchemaWarnsSuppressed(); gotn != before+1 {
		t.Errorf("LocalNewerSchemaWarnsSuppressed %d → %d, want +1", before, gotn)
	}
}

// Two profiles in one process are two files; a global Once would swallow the second.
func TestNewerLocalDBWarnsSeparatelyPerPath(t *testing.T) {
	pathA := newerLocalFixture(t, len(localMigrations)+1)
	pathB := newerLocalFixture(t, len(localMigrations)+1)
	before := LocalNewerSchemaWarnsSuppressed()
	got := captureLog(t, func() {
		openReadOnlyQuery(t, pathA)
		openReadOnlyQuery(t, pathB)
	})
	if n := strings.Count(got, "store: local.db:"); n != 2 {
		t.Errorf("two paths should warn twice, got %d; log=%q", n, got)
	}
	if !strings.Contains(got, LocalPath(pathA)) || !strings.Contains(got, LocalPath(pathB)) {
		t.Errorf("each path must appear; log=%q", got)
	}
	if gotn := LocalNewerSchemaWarnsSuppressed(); gotn != before {
		t.Errorf("two distinct first-seen paths must not suppress; %d → %d", before, gotn)
	}
}

func newerLocalFixture(t *testing.T, version int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	raw, err := sql.Open("sqlite", "file:"+LocalPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func openReadOnlyQuery(t *testing.T, path string) {
	t.Helper()
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()
	var n int
	if err := ro.QueryRow(`SELECT COUNT(*) FROM local.visits`).Scan(&n); err != nil {
		t.Fatalf("history must remain readable: %v", err)
	}
}

type hookConn struct {
	exec  func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error)
	query func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error)
}

func (c hookConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.exec(ctx, query, args)
}

func (c hookConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type valueRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *valueRows) Columns() []string { return r.cols }
func (r *valueRows) Close() error      { return nil }
func (r *valueRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

// sqlExecQuerier adapts *sql.DB to sqlite.ExecQuerierContext so a test can
// re-enter attachLocalHook on a live pool. Callers must SetMaxOpenConns(1)
// so Exec and Query share one connection.
type sqlExecQuerier struct{ db *sql.DB }

func (s sqlExecQuerier) ExecContext(ctx context.Context, q string, nv []driver.NamedValue) (driver.Result, error) {
	return s.db.ExecContext(ctx, q, namedDriverArgs(nv)...)
}

func (s sqlExecQuerier) QueryContext(ctx context.Context, q string, nv []driver.NamedValue) (driver.Rows, error) {
	rows, err := s.db.QueryContext(ctx, q, namedDriverArgs(nv)...)
	if err != nil {
		return nil, err
	}
	return &sqlDriverRows{rows: rows}, nil
}

func namedDriverArgs(nv []driver.NamedValue) []any {
	out := make([]any, len(nv))
	for i, v := range nv {
		out[i] = v.Value
	}
	return out
}

type sqlDriverRows struct{ rows *sql.Rows }

func (r *sqlDriverRows) Columns() []string {
	c, err := r.rows.Columns()
	if err != nil {
		return nil
	}
	return c
}
func (r *sqlDriverRows) Close() error { return r.rows.Close() }
func (r *sqlDriverRows) Next(dest []driver.Value) error {
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return io.EOF
	}
	ptrs := make([]any, len(dest))
	for i := range dest {
		ptrs[i] = &dest[i]
	}
	return r.rows.Scan(ptrs...)
}

// TestLocalMigrationV5AddsRecipes is FAIL-first GDK-503: scratch-503-failfirst.log
// recorded PRAGMA user_version=4 and no recipes table on 6aaaef0 before this
// migration existed. Open of a v4 local.db must create local.recipes.
func TestLocalMigrationV5AddsRecipes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	local := LocalPath(path)
	raw, err := sql.Open("sqlite", "file:"+local)
	if err != nil {
		t.Fatal(err)
	}
	for i, ddl := range []string{localSchemaV1, localSchemaV2, localSchemaV3, localSchemaV4} {
		if _, err := raw.Exec(ddl); err != nil {
			t.Fatalf("apply localSchemaV%d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatal(err)
	}
	var n, uv int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='recipes'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("pre-migration recipes table count = %d, want 0", n)
	}
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != 4 {
		t.Fatalf("pre-migration user_version = %d, want 4", uv)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM local.recipes`).Scan(&n); err != nil {
		t.Fatalf("recipes after v4→v5: %v", err)
	}
	after, err := sql.Open("sqlite", "file:"+local)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	if err := after.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != 5 {
		t.Fatalf("post-migration user_version = %d, want 5", uv)
	}
}

func TestRecipeCRUDOverwriteAndDelete(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	q1 := "select 1 as n"
	got, err := db.PutRecipe(ctx, "next", q1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "next" || got.SQL != q1 || got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatalf("insert = %+v", got)
	}
	created := got.CreatedAt
	time.Sleep(2 * time.Millisecond)
	q2 := "select 2 as n"
	got, err = db.PutRecipe(ctx, "next", q2)
	if err != nil {
		t.Fatal(err)
	}
	if got.SQL != q2 {
		t.Fatalf("overwrite sql = %q, want %q", got.SQL, q2)
	}
	if got.CreatedAt != created {
		t.Fatalf("created_at moved on overwrite: %q → %q", created, got.CreatedAt)
	}
	if got.UpdatedAt == created {
		t.Fatal("updated_at did not move on overwrite")
	}
	if _, err := db.Recipe(ctx, "next"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRecipe(ctx, "next"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Recipe(ctx, "next"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v, want ErrNotFound", err)
	}
	if err := db.DeleteRecipe(ctx, "next"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v, want ErrNotFound", err)
	}
}

func TestRecipeNameRejected(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for _, name := range []string{"", "Next", "has space", "-lead", "under_score", "1num"} {
		if _, err := db.PutRecipe(ctx, name, "select 1"); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	if _, err := db.PutRecipe(ctx, "ok-name", "   "); err == nil {
		t.Fatal("empty sql accepted")
	}
}
