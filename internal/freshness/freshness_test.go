package freshness

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// The table below is the package's documentation: every State, the
// precedence between them, and the incidents that shaped each branch
// (GDK-654's leftover row, GDK-810's corrupt stamp, GDK-1677's live first
// pass) read as cases, not prose.
//
// Two clocks, on purpose. Cases without a live first sync pin fixedNow so
// every age is a constant. Cases WITH one must use the wall clock: the
// heartbeat rows are written by the store API with real timestamps, and
// Assess's liveness cutoff is derived from the `now` it is handed — a fixed
// `now` in the future of the stamped rows would read the pass as dead and
// the case would quietly test the wrong branch.

var fixedNow = time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

// mirrorAt copies examples/demo.db into a temp dir — the same fixture
// pattern cmd/gadak and internal/mcp tests use — and returns its path.
func mirrorAt(t *testing.T) string {
	t.Helper()
	in, err := os.ReadFile(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(dst, in, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}
	return dst
}

func exec(t *testing.T, path, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open mirror writable: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("mirror exec %q: %v", query, err)
	}
}

// setSyncedAt stamps one source's synced_at. Second-precision RFC3339, the
// same spelling the CLI tests plant, so the parsed age is exact.
func setSyncedAt(t *testing.T, path, id string, at time.Time) {
	t.Helper()
	exec(t, path, `UPDATE sources SET synced_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339), id)
}

// beginFirstSync writes a live first-pass heartbeat through the store API —
// the same writer a real sync uses, so the row Assess reads is row-shaped by
// construction, not by a hand-written INSERT.
func beginFirstSync(t *testing.T, path, sourceID string, fetched int, total *int) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open mirror rw: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.BeginSyncProgress(ctx, sourceID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.TouchSyncProgress(ctx, sourceID, fetched, total); err != nil {
		t.Fatal(err)
	}
}

var neverConfluence = func() bool { return false }
var alwaysConfluence = func() bool { return true }

func TestAssess(t *testing.T) {
	twoHoursAgo := fixedNow.Add(-2 * time.Hour)
	twoHoursAgoRaw := twoHoursAgo.UTC().Format(time.RFC3339)
	total3514 := sql.NullInt64{Int64: 3514, Valid: true}
	backdateConfluence := func(t *testing.T, path string) {
		t.Helper()
		exec(t, path, `UPDATE sync_progress SET started_at = ? WHERE source_id = ?`,
			time.Now().Add(-time.Hour).UTC().Format(config.ISOMilli), syncer.ConfluenceSourceID)
	}

	cases := []struct {
		name string
		// wallClock: assess with time.Now() instead of fixedNow (live
		// sync_progress rows carry wall stamps; see the file comment).
		wallClock bool
		// setup mutates the copied fixture before Assess reads it.
		setup func(t *testing.T, path string)
		// confluence is the lazy config predicate; nil means pass never.
		confluence func() bool
		want       Report
	}{
		{
			name: "fresh: every source inside the hour",
			setup: func(t *testing.T, path string) {
				setSyncedAt(t, path, "jira", fixedNow.Add(-10*time.Minute))
				setSyncedAt(t, path, "confluence", fixedNow.Add(-30*time.Minute))
			},
			want: Report{State: StateFresh},
		},
		{
			name: "condition 3: oldest source past the hour is named, not the whole mirror",
			setup: func(t *testing.T, path string) {
				setSyncedAt(t, path, "jira", fixedNow.Add(-10*time.Minute))
				setSyncedAt(t, path, "confluence", twoHoursAgo)
			},
			want: Report{State: StateStaleSource, SourceID: "confluence", SyncedAt: twoHoursAgoRaw, Age: 2 * time.Hour},
		},
		{
			name: "stale tie names the first source_id (demo.db order → confluence)",
			setup: func(t *testing.T, path string) {
				setSyncedAt(t, path, "jira", twoHoursAgo)
				setSyncedAt(t, path, "confluence", twoHoursAgo)
			},
			want: Report{State: StateStaleSource, SourceID: "confluence", SyncedAt: twoHoursAgoRaw, Age: 2 * time.Hour},
		},
		{
			name: "condition 2: a stored last_error outranks age",
			setup: func(t *testing.T, path string) {
				setSyncedAt(t, path, "jira", twoHoursAgo)
				setSyncedAt(t, path, "confluence", twoHoursAgo)
				exec(t, path, `UPDATE sync_state SET last_error = 'planted jira fail' WHERE source_id = 'jira'`)
			},
			want: Report{State: StateSyncFailed, SourceID: "jira", LastError: "planted jira fail"},
		},
		{
			name:       "condition 1: a live first sync outranks a failed sync and age",
			wallClock:  true,
			confluence: neverConfluence,
			setup: func(t *testing.T, path string) {
				now := time.Now()
				setSyncedAt(t, path, "jira", now.Add(-2*time.Hour))
				setSyncedAt(t, path, "confluence", now.Add(-2*time.Hour))
				exec(t, path, `UPDATE sync_state SET last_error = 'planted jira fail' WHERE source_id = 'jira'`)
				beginFirstSync(t, path, syncer.SourceID, 1200, nil)
			},
			want: Report{State: StateFirstSync, SourceID: syncer.SourceID, Fetched: 1200},
		},
		{
			name:      "first sync carries the heartbeat counts",
			wallClock: true,
			setup: func(t *testing.T, path string) {
				now := time.Now()
				setSyncedAt(t, path, "jira", now)
				setSyncedAt(t, path, "confluence", now)
				total := 3514
				beginFirstSync(t, path, syncer.SourceID, 1200, &total)
			},
			want: Report{State: StateFirstSync, SourceID: syncer.SourceID, Fetched: 1200, Total: total3514},
		},
		{
			name:      "first sync on confluence is the documents phase, never wiki-pending",
			wallClock: true,
			setup: func(t *testing.T, path string) {
				now := time.Now()
				setSyncedAt(t, path, "jira", now)
				setSyncedAt(t, path, "confluence", now)
				beginFirstSync(t, path, syncer.ConfluenceSourceID, 462, nil)
			},
			confluence: alwaysConfluence,
			want:       Report{State: StateFirstSync, SourceID: syncer.ConfluenceSourceID, Fetched: 462},
		},
		{
			name:      "wiki pending when confluence is configured and not yet started",
			wallClock: true,
			setup: func(t *testing.T, path string) {
				now := time.Now()
				setSyncedAt(t, path, "jira", now)
				setSyncedAt(t, path, "confluence", now)
				beginFirstSync(t, path, syncer.SourceID, 1200, nil)
			},
			confluence: alwaysConfluence,
			want:       Report{State: StateFirstSync, SourceID: syncer.SourceID, Fetched: 1200, WikiPending: true},
		},
		{
			name:      "wiki not pending when the confluence pass is already live",
			wallClock: true,
			setup: func(t *testing.T, path string) {
				now := time.Now()
				setSyncedAt(t, path, "jira", now)
				setSyncedAt(t, path, "confluence", now)
				beginFirstSync(t, path, syncer.ConfluenceSourceID, 10, nil)
				beginFirstSync(t, path, syncer.SourceID, 1200, nil)
				// Both rows carry wall stamps; pin the pick order explicitly
				// rather than trusting same-millisecond wall clocks — the
				// issues pass must be the newest started_at.
				backdateConfluence(t, path)
			},
			confluence: alwaysConfluence,
			want:       Report{State: StateFirstSync, SourceID: syncer.SourceID, Fetched: 1200},
		},
		{
			name: "no sync_state rows at all",
			setup: func(t *testing.T, path string) {
				exec(t, path, `DELETE FROM sync_state`)
			},
			want: Report{State: StateNeverSynced},
		},
		{
			name: "sync_state rows but no parseable synced_at anywhere",
			setup: func(t *testing.T, path string) {
				exec(t, path, `UPDATE sources SET synced_at = ''`)
			},
			want: Report{State: StateNeverSynced},
		},
		{
			// GDK-654: an empty leftover jira row must not claim the whole
			// mirror never synced while its sibling parsed and is fresh.
			name: "leftover empty jira beside a fresh sibling is fresh (GDK-654)",
			setup: func(t *testing.T, path string) {
				exec(t, path, `UPDATE sources SET synced_at = NULL WHERE id = 'jira'`)
				setSyncedAt(t, path, "confluence", fixedNow.Add(-5*time.Minute))
			},
			want: Report{State: StateFresh},
		},
		{
			// GDK-810 손상: a corrupt stamp is skipped, not crashed on, and
			// must not take the never-synced branch while a sibling parsed.
			name: "unparseable confluence stamp beside a fresh jira is fresh (GDK-810)",
			setup: func(t *testing.T, path string) {
				exec(t, path, `UPDATE sources SET synced_at = 'not-a-clock' WHERE id = 'confluence'`)
				setSyncedAt(t, path, "jira", fixedNow.Add(-5*time.Minute))
			},
			want: Report{State: StateFresh},
		},
		{
			name: "a sync_progress read error falls through to the staleness judgment",
			setup: func(t *testing.T, path string) {
				setSyncedAt(t, path, "jira", twoHoursAgo)
				setSyncedAt(t, path, "confluence", twoHoursAgo)
				exec(t, path, `DROP TABLE sync_progress`)
			},
			want: Report{State: StateStaleSource, SourceID: "confluence", SyncedAt: twoHoursAgoRaw, Age: 2 * time.Hour},
		},
		{
			name: "a sync_state read error is silence (doctor owns that verdict)",
			setup: func(t *testing.T, path string) {
				exec(t, path, `DROP TABLE sync_state`)
			},
			want: Report{State: StateFresh},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := mirrorAt(t)
			tc.setup(t, path)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("open mirror: %v", err)
			}
			defer db.Close()
			now := fixedNow
			if tc.wallClock {
				now = time.Now()
			}
			got := Assess(db, now, tc.confluence)
			if got != tc.want {
				t.Fatalf("Assess = %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	for s, want := range map[State]string{
		StateFresh: "fresh", StateFirstSync: "first-sync", StateNeverSynced: "never-synced",
		StateSyncFailed: "sync-failed", StateStaleSource: "stale-source",
	} {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", s, got, want)
		}
	}
}

// TestNoticeWording pins the sentence set the MCP read tools append: the
// fixed prefix (a host recognizes the line without parsing the sentence),
// the named source and echoed synced_at on the age line (the same strings
// `gadak status` prints, GDK-810), and empty for fresh — the notice exists
// only when it has something true to say.
func TestNoticeWording(t *testing.T) {
	stamp := "2026-09-12T10:00:00Z"
	cases := []struct {
		state State
		rep   Report
		want  []string
	}{
		{StateFresh, Report{State: StateFresh}, nil},
		{
			StateFirstSync,
			Report{State: StateFirstSync, SourceID: "jira", Fetched: 1200},
			[]string{"first sync in progress", "results are partial"},
		},
		{
			StateNeverSynced,
			Report{State: StateNeverSynced},
			[]string{"never finished a sync"},
		},
		{
			StateSyncFailed,
			Report{State: StateSyncFailed, SourceID: "jira", LastError: "planted jira fail"},
			[]string{"last sync failed (jira): planted jira fail", "gadak sync", "gadak_status"},
		},
		{
			StateStaleSource,
			Report{State: StateStaleSource, SourceID: "confluence", SyncedAt: stamp, Age: 2 * time.Hour},
			[]string{"confluence last synced", "synced_at " + stamp, "2h0m0s ago", "gadak sync", "gadak_status"},
		},
	}
	for _, tc := range cases {
		got := tc.rep.Notice()
		if tc.want == nil {
			if got != "" {
				t.Errorf("%s notice = %q, want empty", tc.state, got)
			}
			continue
		}
		if !strings.HasPrefix(got, "Mirror freshness: ") {
			t.Errorf("%s notice lacks the fixed prefix: %q", tc.state, got)
		}
		for _, needle := range tc.want {
			if !strings.Contains(got, needle) {
				t.Errorf("%s notice missing %q: %q", tc.state, needle, got)
			}
		}
		if strings.Contains(got, "\n") {
			t.Errorf("%s notice must be one line: %q", tc.state, got)
		}
	}
}

// TestFormatSourceIDStripsControlAndClips moved from cmd/gadak/sql_test.go
// with the sanitizer it pins (GDK-599): the guard is single-owned here now,
// and the MCP result notice interpolates through the same code.
func TestFormatSourceIDStripsControlAndClips(t *testing.T) {
	if got := FormatSourceID("jira"); got != "jira" {
		t.Fatalf("plain id: got %q", got)
	}
	if got := FormatSourceID("confluence"); got != "confluence" {
		t.Fatalf("known id: got %q", got)
	}
	got := FormatSourceID("jira\nWARNING: pwned")
	if strings.Contains(got, "\n") {
		t.Fatalf("control rune leaked: %q", got)
	}
	if !strings.Contains(got, "jira") {
		t.Fatalf("kept printable runes, got %q", got)
	}
	long := strings.Repeat("W", 200)
	got = FormatSourceID(long)
	if runewidth.StringWidth(got) > sourceIDDisplayCols {
		t.Fatalf("clipped width %d > %d: %q", runewidth.StringWidth(got), sourceIDDisplayCols, got)
	}
	if got := FormatSourceID("\x00\x07\n"); got != "?" {
		t.Fatalf("control-only id: got %q, want ?", got)
	}
}
