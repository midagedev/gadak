package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/store"
)

// GDK-1307: a second sync process (an older app still running beside a
// newer CLI) pruned the page between UpsertPages and the stamp write, so
// ReplacePageVersions hit the page_versions→items FOREIGN KEY and the whole
// wiki pass died with it — five spaces gone from the mirror. Missing
// history degrades the mirror; it must not break the pass, for the store
// exactly as for the network (collectPageVersions' own contract). Here the
// item never existed at all, which is the same state the race leaves.
func TestCollectPageVersionsSurvivesVanishedItem(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/version") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"number": 1, "when": "2026-09-01T00:00:00.000Z", "by": map[string]any{"accountId": "u1", "displayName": "Dana"}},
				{"number": 2, "when": "2026-09-02T00:00:00.000Z", "by": map[string]any{"accountId": "u1", "displayName": "Dana"}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	c := confluence.New(srv.URL, "user@example.invalid", "secret-token")
	c.Retries, c.PauseBetween = 1, 0

	var logs []string
	opts := Options{Log: func(s string) { logs = append(logs, s) }}
	// "conf:9999" has no items row: the FK on page_versions.item_id fails.
	err = collectPageVersions(context.Background(), c, db, opts, "conf:9999", "9999", 2)
	if err != nil {
		t.Fatalf("a stamp write that fails must not fail the pass; got %v", err)
	}
	found := false
	for _, l := range logs {
		if strings.Contains(l, "page versions 9999") && strings.Contains(l, "write") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the swallowed write error must be logged with the page id; logs: %q", logs)
	}

	// A cancelled context is the caller stopping, and still propagates.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := collectPageVersions(ctx, c, db, opts, "conf:9999", "9999", 3); err == nil {
		t.Fatal("a cancelled context must propagate")
	}
}
