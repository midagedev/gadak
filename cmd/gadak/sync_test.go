package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func TestSyncStale(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	thresh := time.Hour
	recent := now.Add(-10 * time.Minute).Format(time.RFC3339)
	old := now.Add(-2 * time.Hour).Format(time.RFC3339)
	exact := now.Add(-thresh).Format(time.RFC3339)
	justOver := now.Add(-thresh - time.Nanosecond).Format(time.RFC3339)
	milli := now.Add(-10 * time.Minute).UTC().Format(config.ISOMilli)
	cases := []struct {
		name, synced, last string
		want               bool
	}{
		{"fresh", recent, "", false},
		{"old", old, "", true},
		{"never", "", "", true},
		{"last_error_recent", recent, "boom", true},
		{"last_error_never", "", "boom", true},
		{"exact_threshold", exact, "", false},
		{"just_over_threshold", justOver, "", true},
		{"iso_milli_fresh", milli, "", false},
		{"unparseable", "not-a-time", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := syncStale(tc.synced, tc.last, now, thresh)
			if got != tc.want {
				t.Fatalf("syncStale(%q, %q) = %v, want %v", tc.synced, tc.last, got, tc.want)
			}
		})
	}
}

func TestSyncHelpDocumentsIfStale(t *testing.T) {
	out := formatHelp("sync", nil)
	if !strings.Contains(out, "[--if-stale DUR]") {
		t.Fatalf("sync usage missing [--if-stale DUR]:\n%s", out)
	}
	if !strings.Contains(out, "gadak sync --if-stale 15m") {
		t.Fatalf("sync examples missing --if-stale 15m:\n%s", out)
	}
}

func TestSyncIfStaleWatchIsUsageError(t *testing.T) {
	_, err := capture(t, func() error {
		return cmdSync([]string{"--watch", "--if-stale", "1h"})
	})
	if err == nil {
		t.Fatal("--watch --if-stale must be a usage error")
	}
	if !strings.Contains(err.Error(), "--watch") || !strings.Contains(err.Error(), "--if-stale") {
		t.Fatalf("want combined-flag usage, got %v", err)
	}
	if !strings.Contains(err.Error(), `run "gadak sync --help"`) {
		t.Fatalf("want usageError help pointer, got %v", err)
	}
}

func TestSyncIfStaleInvalidDurationIsUsageError(t *testing.T) {
	_, err := capture(t, func() error {
		return cmdSync([]string{"--if-stale", "nope"})
	})
	if err == nil {
		t.Fatal("invalid --if-stale must be a usage error")
	}
	if !strings.Contains(err.Error(), "--if-stale") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("usage must echo the bad value, got %v", err)
	}
}

func TestSyncIfStaleZeroDurationIsUsageError(t *testing.T) {
	_, err := capture(t, func() error {
		return cmdSync([]string{"--if-stale", "0s"})
	})
	if err == nil {
		t.Fatal("--if-stale 0s must be a usage error")
	}
	if !strings.Contains(err.Error(), "--if-stale") {
		t.Fatalf("usage must name --if-stale, got %v", err)
	}
}

func TestSyncIfStaleDoesNotBypassFrozen(t *testing.T) {
	frozenHome(t)
	_, err := capture(t, func() error {
		return cmdSync([]string{"--if-stale", "1h"})
	})
	if err == nil {
		t.Fatal("frozen workspace must still refuse --if-stale")
	}
	if !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("want frozen error, got %v", err)
	}
}

func TestSyncIfStaleDoesNotBypassCredential(t *testing.T) {
	emptyHome(t)
	_, err := capture(t, func() error {
		return cmdSync([]string{"--if-stale", "1h"})
	})
	if err == nil {
		t.Fatal("empty workspace must still refuse --if-stale")
	}
	if !strings.Contains(err.Error(), config.ErrNotConfigured.Error()) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestSyncIfStaleFreshSkipsOrigin(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits++
	}))
	t.Cleanup(srv.Close)
	mirror(t, srv.URL)
	plantJiraSync(t, time.Now().Add(-5*time.Minute), "")

	out, err := capture(t, func() error {
		return cmdSync([]string{"--if-stale", "1h"})
	})
	if err != nil {
		t.Fatalf("fresh --if-stale: %v\n%s", err, out)
	}
	if hits != 0 {
		t.Fatalf("fresh --if-stale must not hit the origin, hits=%d", hits)
	}
	if !strings.Contains(out, "sync: fresh") {
		t.Fatalf("want fresh log, got %q", out)
	}
	if !strings.Contains(out, "jira") || !strings.Contains(out, "threshold 1h") {
		t.Fatalf("fresh log must name jira and threshold, got %q", out)
	}
}

func TestSyncIfStaleLastErrorRuns(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	mirror(t, srv.URL)
	plantJiraSync(t, time.Now(), "planted fail")

	out, err := capture(t, func() error {
		return cmdSync([]string{"--if-stale", "1h"})
	})
	if hits == 0 {
		t.Fatalf("last_error must attempt the origin; err=%v out=%q", err, out)
	}
	if strings.Contains(out, "sync: fresh") {
		t.Fatalf("last_error must not take the fresh no-op, got %q", out)
	}
}

func plantJiraSync(t *testing.T, syncedAt time.Time, lastErr string) {
	t.Helper()
	path := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sr := store.SyncResult{}
	if lastErr != "" {
		sr.Err = errors.New(lastErr)
	} else {
		sr.Watermark = "w"
	}
	if err := db.RecordSync(context.Background(), "jira", sr); err != nil {
		db.Close()
		t.Fatalf("RecordSync: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if lastErr != "" || syncedAt.IsZero() {
		return
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`UPDATE sources SET synced_at = ? WHERE id = 'jira'`, syncedAt.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("rewind synced_at: %v", err)
	}
}
