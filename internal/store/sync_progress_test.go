package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// syncProgressDB is the plain opener every store test uses.
func syncProgressDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestSyncProgressBeginTouchEnd is the round-trip (GDK-1677): Begin writes a
// live row with fetched 0 and no total, Touch advances fetched and updates the
// heartbeat, a Touch without a total keeps the stored one (COALESCE — the
// reconcile phase has no denominator and must not wipe a known count), and End
// deletes the row.
func TestSyncProgressBeginTouchEnd(t *testing.T) {
	db := syncProgressDB(t)
	ctx := context.Background()

	if err := db.BeginSyncProgress(ctx, "jira", true); err != nil {
		t.Fatal(err)
	}
	rows, err := db.SyncProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows after begin = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.SourceID != "jira" || !r.First || r.Fetched != 0 || r.Total != nil {
		t.Fatalf("row after begin = %+v, want jira/first/fetched 0/total nil", r)
	}
	if r.StartedAt == "" || r.UpdatedAt == "" {
		t.Fatalf("row after begin = %+v, want both stamps", r)
	}

	total := 3514
	if err := db.TouchSyncProgress(ctx, "jira", 1200, &total); err != nil {
		t.Fatal(err)
	}
	// Touch with no total: the stored denominator survives (the Jira reconcile
	// phase resets its unitDenom to unknown mid-pass).
	if err := db.TouchSyncProgress(ctx, "jira", 2400, nil); err != nil {
		t.Fatal(err)
	}
	rows, err = db.SyncProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Fetched != 2400 {
		t.Fatalf("rows after touch = %+v, want fetched 2400", rows)
	}
	if rows[0].Total == nil || *rows[0].Total != 3514 {
		t.Fatalf("total after nil touch = %v, want 3514 kept", rows[0].Total)
	}
	if !rows[0].First {
		t.Error("touch must not clear first")
	}

	if err := db.EndSyncProgress(ctx, "jira"); err != nil {
		t.Fatal(err)
	}
	if rows, err = db.SyncProgress(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("rows after end = %v (err %v), want none", rows, err)
	}
}

// TestSyncProgressStaleRowReadsAbsent is the liveness contract: a row whose
// heartbeat is older than the window is not returned — a crashed sync process
// must read as "no first sync in progress", not as one that froze. The next
// full pass overwrites the row (BeginSyncProgress upserts).
func TestSyncProgressStaleRowReadsAbsent(t *testing.T) {
	db := syncProgressDB(t)
	ctx := context.Background()

	if err := db.BeginSyncProgress(ctx, "jira", true); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	stale := time.Now().UTC().Add(-(SyncProgressLiveWindow + time.Second)).Format(config.ISOMilli)
	if _, err := conn.Exec(`UPDATE sync_progress SET updated_at = ?`, stale); err != nil {
		t.Fatal(err)
	}

	if rows, err := db.SyncProgress(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("stale row = %v (err %v), want none", rows, err)
	}

	// Upsert: the next full pass reuses the row rather than stacking a second one.
	if err := db.BeginSyncProgress(ctx, "jira", true); err != nil {
		t.Fatal(err)
	}
	rows, err := db.SyncProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows after re-begin = %d, want 1 (upsert, not insert)", len(rows))
	}
}

// TestSyncProgressFirstFlagRoundTrip pins the two meanings of the row: first=1
// is a mirror that was empty when the pass started (what every reader
// surfaces); first=0 is a forced --full over a filled mirror (readers show
// nothing). BeginSyncProgress is what writes the flag, so both values round-trip.
func TestSyncProgressFirstFlagRoundTrip(t *testing.T) {
	db := syncProgressDB(t)
	ctx := context.Background()

	if err := db.BeginSyncProgress(ctx, "jira", true); err != nil {
		t.Fatal(err)
	}
	total := 3
	if err := db.TouchSyncProgress(ctx, "jira", 2, &total); err != nil {
		t.Fatal(err)
	}
	// A --full pass over the now-filled mirror reuses the row with first=0.
	if err := db.BeginSyncProgress(ctx, "jira", false); err != nil {
		t.Fatal(err)
	}
	rows, err := db.SyncProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].First {
		t.Fatalf("row after non-first begin = %+v, want first=false", rows)
	}
	if rows[0].Fetched != 0 {
		t.Errorf("fetched after re-begin = %d, want 0 (counters reset)", rows[0].Fetched)
	}
	if rows[0].Total != nil {
		t.Errorf("total after re-begin = %v, want nil (counters reset)", rows[0].Total)
	}
}

// TestSyncProgressCutoffShape pins that the cutoff the readers filter on is
// the live window behind now, in the fixed-width UTC layout the writer uses —
// the reason a lexicographic >= in SQL is a time comparison.
func TestSyncProgressCutoffShape(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 30, 0, time.UTC)
	got := SyncProgressCutoff(now)
	if want := now.Add(-SyncProgressLiveWindow).Format(config.ISOMilli); got != want {
		t.Fatalf("cutoff = %q, want %q", got, want)
	}
	if _, err := time.Parse(config.ISOMilli, got); err != nil {
		t.Fatalf("cutoff %q is not ISOMilli: %v", got, err)
	}
}
