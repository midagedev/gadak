package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	old := time.Now().UTC().Add(-200 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
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
	old := time.Now().UTC().Add(-200 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
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

func TestRecordVisitRejectsBadKind(t *testing.T) {
	db := openTemp(t)
	_, err := db.RecordVisit(context.Background(), "ticket", "NMB-1")
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("got %v, want kind error", err)
	}
}
