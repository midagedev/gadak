package sync

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// TestRecordDoesNotWriteLastErrorOnBusy is FAIL-first for GDK-754: a SQLITE_BUSY
// failure must not pay a second write() to persist last_error, and the returned
// error must name the lock holder.
func TestRecordDoesNotWriteLastErrorOnBusy(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), store.Source{ID: SourceID, Kind: KindJira}); err != nil {
		t.Fatal(err)
	}
	busy := busyCodeError{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}
	start := time.Now()
	err = record(context.Background(), nil, db, SourceID, busy)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("record() returned nil")
	}
	if !strings.Contains(err.Error(), "another gadak process") {
		t.Fatalf("error missing holder hint: %v", err)
	}
	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != nil && *st.LastError != "" {
		t.Fatalf("last_error was written on BUSY: %q", *st.LastError)
	}
	if elapsed > time.Second {
		t.Fatalf("record(BUSY) took %s — last_error write must not wait out busy_timeout", elapsed)
	}
}

// TestRunSourceRetriesBusyPass is FAIL-first for GDK-754 pass-level retry:
// SQLITE_BUSY from pass() is retried once, and the final error carries the
// holder hint.
func TestRunSourceRetriesBusyPass(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := &config.Config{Site: "https://example.invalid", Email: "dev@example.invalid", Token: "tok"}
	busy := busyCodeError{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}
	calls := 0
	_, err = runSource(context.Background(), cfg, db, Options{},
		sourceIdent{ID: SourceID, Kind: KindJira},
		false, "",
		func() (string, usageTaker, error) { return "https://example.invalid", nil, nil },
		func(store.SyncState, *Result) error {
			calls++
			return busy
		},
	)
	if calls != 2 {
		t.Fatalf("pass calls = %d, want 2 (one BUSY retry)", calls)
	}
	if err == nil {
		t.Fatal("runSource returned nil")
	}
	if !strings.Contains(err.Error(), "another gadak process") {
		t.Fatalf("error missing holder hint: %v", err)
	}
}
