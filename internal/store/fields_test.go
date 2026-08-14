package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/fields"
)

// TestReingestCustomBumpsVersionAndSyncedAt is FAIL-first for C1: discovery
// reingest must move sync_state.version and items.synced_at so an open client
// does not 304 past the new custom fields.
func TestReingestCustomBumpsVersionAndSyncedAt(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"id":"1","fields":{"summary":"s","customfield_10050":{"value":"Sev1"},"description":{"type":"doc","version":1,"content":[]}}}`)
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMB-1", Title: "s", BodyText: "old body",
				CreatedAt: ago(2), UpdatedAt: ago(1),
			},
			Issue: Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "1",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
				DescriptionADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
				Custom:         map[string]any{},
				Raw:            raw,
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := db.SyncState(ctx, "jira")
	if err != nil {
		t.Fatal(err)
	}
	var syncedBefore string
	if err := db.sql.QueryRowContext(ctx, `SELECT synced_at FROM items WHERE id = 'jira:1'`).Scan(&syncedBefore); err != nil {
		t.Fatal(err)
	}

	// Now() is millisecond-granular; a same-ms reingest would look unchanged.
	time.Sleep(2 * time.Millisecond)

	n, err := db.ReingestCustom(ctx, []fields.SpecIDs{{
		Alias: "severity", IDs: []string{"customfield_10050"}, Role: "facet",
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reingest rewrote %d, want 1", n)
	}

	after, err := db.SyncState(ctx, "jira")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version <= before.Version {
		t.Fatalf("C1: version stayed %d after reingest (want bump)", after.Version)
	}
	var syncedAfter string
	if err := db.sql.QueryRowContext(ctx, `SELECT synced_at FROM items WHERE id = 'jira:1'`).Scan(&syncedAfter); err != nil {
		t.Fatal(err)
	}
	if syncedAfter == syncedBefore {
		t.Fatalf("C1: items.synced_at unchanged after reingest (%s)", syncedAfter)
	}
}
