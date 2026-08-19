package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWriteReadThenWriteAgainstHeldTxn is FAIL-first for GDK-305: a deferred
// SELECT-then-INSERT while another *DB holds an uncommitted write returns
// SQLITE_BUSY immediately (busy_timeout is not consulted on a lock upgrade).
// With _txlock=immediate the busy moves to BEGIN, which waits, so releasing
// the holder lets the write succeed. The assertion is the error (nil vs busy),
// not a duration.
func TestWriteReadThenWriteAgainstHeldTxn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	holder := openTemp(t, path)
	if err := holder.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	challenger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { challenger.Close() })

	ctx := context.Background()
	holdTx, err := holder.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer holdTx.Rollback()
	if _, err := holdTx.Exec(`INSERT INTO items (id, source_id, kind, key, updated_at)
		VALUES ('jira:hold', 'jira', 'issue', 'NMB-HOLD', '2026-08-04T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- challenger.write(ctx, func(tx *sql.Tx) error {
			select {
			case <-started:
			default:
				close(started)
			}
			var n int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&n); err != nil {
				return err
			}
			_, err := tx.Exec(`INSERT INTO items (id, source_id, kind, key, updated_at)
				VALUES ('jira:up', 'jira', 'issue', 'NMB-UP', '2026-08-04T00:00:00Z')`)
			return err
		})
	}()

	select {
	case <-started:
		// Deferred BEGIN: the callback ran while the other connection still
		// holds the write lock. That is the upgrade that ignores busy_timeout.
		err := <-done
		t.Fatalf("write callback ran under held lock: %v", err)
	case err := <-done:
		t.Fatalf("write returned before callback: %v", err)
	case <-time.After(time.Second):
		// BEGIN is waiting in busy_timeout. Drop the holder so it can proceed.
		// This case is a blocked-BEGIN detector, not a duration assertion.
		if err := holdTx.Rollback(); err != nil {
			t.Fatal(err)
		}
		err := <-done
		if err != nil {
			t.Fatalf("write after holder released: %v", err)
		}
	}
}

func TestOpenReadOnlyBusyTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ro.Close() })

	var got string
	if err := ro.QueryRow(`PRAGMA busy_timeout`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "5000" {
		t.Fatalf("OpenReadOnly busy_timeout = %s, want 5000", got)
	}
}

func TestSQLiteBusyCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("database is locked (5) (SQLITE_BUSY)"), false}, // prose only, no Code()
		{codeErr{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}, true},
		{codeErr{code: 517, msg: "database is locked (517)"}, true},
		{fmt.Errorf("wrap: %w", codeErr{code: 5, msg: "busy"}), true},
		{fmt.Errorf("wrap: %w", codeErr{code: 517, msg: "snapshot"}), true},
		{codeErr{code: 261, msg: "SQLITE_BUSY_RECOVERY"}, false},
		{codeErr{code: 773, msg: "SQLITE_BUSY_TIMEOUT"}, false},
		{codeErr{code: 6, msg: "SQLITE_LOCKED"}, false},
		{errors.New("constraint failed"), false},
		{sql.ErrNoRows, false},
		{context.Canceled, false},
	}
	for _, c := range cases {
		if got := sqliteBusy(c.err); got != c.want {
			t.Errorf("sqliteBusy(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestWriteDoesNotRetryNonBusy(t *testing.T) {
	db := openTemp(t)
	before := db.WriteBusyRetries()
	sentinel := errors.New("not busy")
	err := db.write(context.Background(), func(*sql.Tx) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want sentinel", err)
	}
	if got := db.WriteBusyRetries(); got != before {
		t.Fatalf("WriteBusyRetries %d → %d, want unchanged", before, got)
	}
}

func TestWriteBusyRetryThenSucceeds(t *testing.T) {
	db := openTemp(t)
	calls := 0
	err := retryBusy(context.Background(), &db.writeBusyRetries, func() error {
		calls++
		if calls == 1 {
			return codeErr{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if got := db.WriteBusyRetries(); got != 1 {
		t.Fatalf("WriteBusyRetries = %d, want 1", got)
	}
}

func TestWriteBusyRetryHonoursCancel(t *testing.T) {
	db := openTemp(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := retryBusy(ctx, &db.writeBusyRetries, func() error {
		calls++
		cancel()
		return codeErr{code: 517, msg: "database is locked (517)"}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (must not start another attempt)", calls)
	}
}

func TestWriteBusyRetryDoesNotRetryForever(t *testing.T) {
	db := openTemp(t)
	calls := 0
	busy := codeErr{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}
	err := retryBusy(context.Background(), &db.writeBusyRetries, func() error {
		calls++
		return busy
	})
	if !sqliteBusy(err) {
		t.Fatalf("got %v, want busy", err)
	}
	if calls != writeBusyAttempts {
		t.Fatalf("calls = %d, want %d", calls, writeBusyAttempts)
	}
	if got := db.WriteBusyRetries(); got != uint64(writeBusyAttempts-1) {
		t.Fatalf("WriteBusyRetries = %d, want %d", got, writeBusyAttempts-1)
	}
}

func TestMirrorDSNTxlockImmediate(t *testing.T) {
	t.Parallel()
	dsn := mirrorDSN("x")
	if !strings.Contains(dsn, "_txlock=immediate") {
		t.Fatalf("mirror DSN missing _txlock=immediate: %s", dsn)
	}
}

func TestWriteCanceledContextDoesNotCallFn(t *testing.T) {
	db := openTemp(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := db.write(ctx, func(*sql.Tx) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("fn ran on a canceled context")
	}
}

// A retry re-runs the callback from the start, so a counter the callback
// closes over has to be reset inside it. UpsertIssues, UpsertPages,
// DeleteItems and the feed mark-read all count rows into a variable declared
// outside their db.write call and return it; without the reset the abandoned
// attempt's rows are counted again and the caller is told more changed than
// did. Both shapes run here, so the failing one is demonstrated rather than
// described (GDK-305).
func TestWriteRetryDoesNotDoubleCountCallerCounters(t *testing.T) {
	rows := []string{"a", "b", "c"}

	for _, tc := range []struct {
		name  string
		reset bool
		want  int
	}{
		{name: "counter reset inside the callback", reset: true, want: len(rows)},
		{name: "counter captured outside (the bug)", reset: false, want: 2 * len(rows)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openTemp(t)
			attempts := 0
			counted := 0
			err := db.write(context.Background(), func(*sql.Tx) error {
				attempts++
				if tc.reset {
					counted = 0
				}
				for range rows {
					counted++
				}
				if attempts == 1 {
					return codeErr{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if attempts != 2 {
				t.Fatalf("attempts = %d, want 2 — the retry did not re-run the callback", attempts)
			}
			if counted != tc.want {
				t.Fatalf("counted = %d, want %d", counted, tc.want)
			}
		})
	}
}

type codeErr struct {
	code int
	msg  string
}

func (e codeErr) Error() string { return e.msg }
func (e codeErr) Code() int     { return e.code }
