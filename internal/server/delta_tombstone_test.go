package server

// GDK-1890: the issue delta's deleted_keys carries issue tombstones only. A
// wiki page tombstone reaching the issue client was a stray key.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

func TestDeltaDeletedKeysExcludePageTombstones(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(ctx, []store.PageRecord{{
		Item: store.Item{ID: "confluence:p9", SourceID: "confluence", Kind: "page", ExternalID: "p9",
			Key: "p9", Title: "page", CreatedAt: store.Now(), UpdatedAt: store.Now()},
		Page: store.Page{SpaceKey: "AAA", Version: 1, Status: "current", BodyADF: adf},
	}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	cursor := store.Now()
	if _, err := db.DeleteItems(ctx, "confluence", []string{"p9"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteItems(ctx, "jira", []string{"NMB-2"}); err != nil {
		t.Fatal(err)
	}
	got := decode[deltaResponse](t, get(t, h, apiBase+"delta/?since="+cursor, nil))
	if len(got.DeletedKeys) != 1 || got.DeletedKeys[0] != "NMB-2" {
		t.Fatalf("deleted_keys = %v, want [NMB-2] (page tombstone p9 must not reach the issue client)", got.DeletedKeys)
	}
}
