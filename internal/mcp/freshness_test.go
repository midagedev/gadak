package mcp

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// GDK-599: the fact "this mirror is behind" used to reach only stderr, which
// an MCP host never sees. The server instruction "call gadak_status before
// acting" is advice, not a guarantee — an agent that skips it answered from a
// stale mirror with full confidence. The notice is the fact riding the result
// itself: appended as a second content item only when the mirror is behind,
// never when it is fresh (a line that is always there is a line agents stop
// reading).

// setAllSourcesSyncedAt rewrites every source's synced_at in the copied
// fixture so the notice's hour arithmetic is deterministic whatever the
// fixture's vintage is — the same discipline as cmd/gadak/sql_test.go's
// setSourcesSyncedAt (tools/e2e-fixture-age-check.sh documents the class:
// an age-shaped expectation without a pinned clock is green the day the
// fixture is stamped and red on the next regeneration).
func setAllSourcesSyncedAt(t *testing.T, dbPath string, at time.Time) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open mirror writable: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE sources SET synced_at = ?`, at.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set synced_at: %v", err)
	}
}

func plantSyncState(t *testing.T, dbPath, sourceID, lastError string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open mirror writable: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO sync_state (source_id, last_error, schema_version, version)
		VALUES (?, ?, 0, 0)
		ON CONFLICT(source_id) DO UPDATE SET last_error = excluded.last_error`,
		sourceID, lastError); err != nil {
		t.Fatalf("plant %s last_error: %v", sourceID, err)
	}
}

// TestQueryAppendsFreshnessNoticeOnStaleMirror is the round's core contract:
// a stale mirror appends exactly one notice item, and the result the query
// itself produced is byte-identical to the same query on a fresh mirror —
// existing JSON parsers of the first item must not see one new byte.
func TestQueryAppendsFreshnessNoticeOnStaleMirror(t *testing.T) {
	query := map[string]any{"sql": "SELECT key, status_category FROM issues LIMIT 2"}

	fresh := demoDB(t)
	setAllSourcesSyncedAt(t, fresh, time.Now())
	freshCall := callToolRaw(t, fresh, toolQuery, query)
	if freshCall.IsError {
		t.Fatalf("fresh query errored: %s", freshCall.Content[0].Text)
	}

	stale := demoDB(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	setAllSourcesSyncedAt(t, stale, stamp)
	staleCall := callToolRaw(t, stale, toolQuery, query)
	if staleCall.IsError {
		t.Fatalf("stale query errored: %s", staleCall.Content[0].Text)
	}

	if len(freshCall.Content) != 1 {
		t.Fatalf("fresh mirror must return exactly 1 content item, got %d", len(freshCall.Content))
	}
	if len(staleCall.Content) != 2 {
		t.Fatalf("stale mirror must append the freshness notice (2 content items), got %d:\n%v",
			len(staleCall.Content), staleCall.Content)
	}
	if staleCall.Content[0].Text != freshCall.Content[0].Text {
		t.Fatalf("query result changed between fresh and stale mirror:\nfresh:\n%s\nstale:\n%s",
			freshCall.Content[0].Text, staleCall.Content[0].Text)
	}
	notice := staleCall.Content[1]
	if notice.Type != "text" {
		t.Fatalf("notice item type = %q, want text", notice.Type)
	}
	if !strings.HasPrefix(notice.Text, "Mirror freshness: ") {
		t.Fatalf("notice must start with the fixed prefix, got %q", notice.Text)
	}
	// The oldest source is named and its stored synced_at echoed, the same
	// strings `gadak status` prints (GDK-810): both sources share the stamp,
	// so the source_id tie-break names confluence.
	if !strings.Contains(notice.Text, "confluence last synced") {
		t.Fatalf("notice must name the oldest source, got %q", notice.Text)
	}
	if !strings.Contains(notice.Text, "synced_at "+stamp.Format(time.RFC3339)) {
		t.Fatalf("notice must echo the stored synced_at %s, got %q", stamp.Format(time.RFC3339), notice.Text)
	}
}

// TestQueryFreshMirrorHasNoFreshnessNotice: a fresh mirror is the default
// state; the notice exists only when it has something true to say.
func TestQueryFreshMirrorHasNoFreshnessNotice(t *testing.T) {
	db := demoDB(t)
	setAllSourcesSyncedAt(t, db, time.Now())
	cr := callToolRaw(t, db, toolQuery, map[string]any{"sql": "SELECT key FROM issues LIMIT 1"})
	if cr.IsError {
		t.Fatalf("query errored: %s", cr.Content[0].Text)
	}
	if len(cr.Content) != 1 {
		t.Fatalf("fresh mirror must return exactly 1 content item, got %d", len(cr.Content))
	}
	if strings.Contains(cr.Content[0].Text, "Mirror freshness") {
		t.Fatalf("fresh mirror's payload must not carry the notice, got %q", cr.Content[0].Text)
	}
}

func TestFreshnessNoticeSyncFailed(t *testing.T) {
	db := demoDB(t)
	setAllSourcesSyncedAt(t, db, time.Now())
	plantSyncState(t, db, "jira", "planted jira fail")
	cr := callToolRaw(t, db, toolQuery, map[string]any{"sql": "SELECT key FROM issues LIMIT 1"})
	if cr.IsError {
		t.Fatalf("query errored: %s", cr.Content[0].Text)
	}
	if len(cr.Content) != 2 || !strings.HasPrefix(cr.Content[1].Text, "Mirror freshness: ") {
		t.Fatalf("failed sync must append the notice, got %d items", len(cr.Content))
	}
	if !strings.Contains(cr.Content[1].Text, "last sync failed (jira): planted jira fail") {
		t.Fatalf("notice must carry the failing source and error, got %q", cr.Content[1].Text)
	}
}

func TestFreshnessNoticeFirstSyncInProgress(t *testing.T) {
	db := demoDB(t)
	setAllSourcesSyncedAt(t, db, time.Now())
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open mirror rw: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if err := st.BeginSyncProgress(ctx, syncer.SourceID, true); err != nil {
		t.Fatal(err)
	}
	total := 3514
	if err := st.TouchSyncProgress(ctx, syncer.SourceID, 1200, &total); err != nil {
		t.Fatal(err)
	}

	cr := callToolRaw(t, db, toolQuery, map[string]any{"sql": "SELECT key FROM issues LIMIT 1"})
	if cr.IsError {
		t.Fatalf("query errored: %s", cr.Content[0].Text)
	}
	if len(cr.Content) != 2 || !strings.HasPrefix(cr.Content[1].Text, "Mirror freshness: ") {
		t.Fatalf("live first sync must append the notice, got %d items", len(cr.Content))
	}
	if !strings.Contains(cr.Content[1].Text, "first sync in progress") {
		t.Fatalf("notice must say a first sync is in progress, got %q", cr.Content[1].Text)
	}
	if !strings.Contains(cr.Content[1].Text, "results are partial") {
		t.Fatalf("notice must say the results are partial, got %q", cr.Content[1].Text)
	}
}

func TestFreshnessNoticeNeverSynced(t *testing.T) {
	db := demoDB(t)
	handle, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Exec(`DELETE FROM sync_state`); err != nil {
		handle.Close()
		t.Fatalf("delete sync_state: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	cr := callToolRaw(t, db, toolQuery, map[string]any{"sql": "SELECT key FROM issues LIMIT 1"})
	if cr.IsError {
		t.Fatalf("query errored: %s", cr.Content[0].Text)
	}
	if len(cr.Content) != 2 || !strings.HasPrefix(cr.Content[1].Text, "Mirror freshness: ") {
		t.Fatalf("never-synced mirror must append the notice, got %d items", len(cr.Content))
	}
	if !strings.Contains(cr.Content[1].Text, "never finished a sync") {
		t.Fatalf("notice must say the mirror never finished a sync, got %q", cr.Content[1].Text)
	}
}

// TestFreshnessNoticeRidesEveryReadTool: the notice is a property of the
// mirror, not of one tool — every tool that answers from the mirror carries
// it. gadak_status is the one exclusion: its payload IS the freshness answer,
// so a notice would say what the result already says.
func TestFreshnessNoticeRidesEveryReadTool(t *testing.T) {
	db := demoDB(t)
	setAllSourcesSyncedAt(t, db, time.Now().Add(-2*time.Hour))

	cases := []struct {
		tool string
		args map[string]any
	}{
		{toolQuery, map[string]any{"sql": "SELECT key FROM issues LIMIT 1"}},
		{toolSearch, map[string]any{"query": "upload", "limit": 3}},
		{toolIssue, map[string]any{"key": "NMA-1"}},
		{toolShow, map[string]any{"keys": []string{"NMA-1"}}},
		{toolRecents, map[string]any{}},
		{toolRetro, map[string]any{}},
	}
	for _, tc := range cases {
		cr := callToolRaw(t, db, tc.tool, tc.args)
		if cr.IsError {
			t.Fatalf("%s errored: %s", tc.tool, cr.Content[0].Text)
		}
		if len(cr.Content) < 2 {
			t.Fatalf("%s on a stale mirror must carry the notice, got %d item(s)", tc.tool, len(cr.Content))
		}
		last := cr.Content[len(cr.Content)-1]
		if !strings.HasPrefix(last.Text, "Mirror freshness: ") {
			t.Fatalf("%s's last content item must be the notice, got %q", tc.tool, last.Text)
		}
	}

	status := callToolRaw(t, db, toolStatus, map[string]any{})
	if status.IsError {
		t.Fatalf("gadak_status errored: %s", status.Content[0].Text)
	}
	for _, item := range status.Content {
		if strings.HasPrefix(item.Text, "Mirror freshness: ") {
			t.Fatalf("gadak_status already answers freshness; it must not also carry the notice: %q", item.Text)
		}
	}
}

// TestErrorResultsCarryNoFreshnessNotice: isError bodies say one thing —
// the failure and its fix. A staleness line after ERROR: would dilute the
// retry teaching, and a failed call's staleness is not the fact in play.
func TestErrorResultsCarryNoFreshnessNotice(t *testing.T) {
	db := demoDB(t)
	setAllSourcesSyncedAt(t, db, time.Now().Add(-2*time.Hour))
	cr := callToolRaw(t, db, toolQuery, map[string]any{"sql": "DROP TABLE issues"})
	if !cr.IsError {
		t.Fatalf("write SQL must be an error result, got %v", cr.Content)
	}
	if len(cr.Content) != 1 {
		t.Fatalf("error result must stay a single item, got %d", len(cr.Content))
	}
}
