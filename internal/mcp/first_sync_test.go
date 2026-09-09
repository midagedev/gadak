package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// GDK-1700 FAIL-first: the status tool is the one tool a shell-less agent
// checks before trusting answers, and while a first full sync fills the
// mirror it carried nothing about it — the first_sync object existed only
// on surfaces an MCP host cannot reach (`gadak status --json`, the read
// verbs' stderr line, the web band). It must ride the same mirror-owned
// row the CLI reads (sync.FirstSync over the live sync_progress heartbeat),
// and a finished pass must emit no first_sync key at all.
func TestStatusToolCarriesFirstSync(t *testing.T) {
	dbPath := demoDB(t)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open mirror rw: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := db.BeginSyncProgress(ctx, syncer.SourceID, true); err != nil {
		t.Fatal(err)
	}
	total := 3514
	if err := db.TouchSyncProgress(ctx, syncer.SourceID, 1200, &total); err != nil {
		t.Fatal(err)
	}

	cr := callToolRaw(t, dbPath, toolStatus, map[string]any{})
	if cr.IsError {
		t.Fatalf("gadak_status errored: %s", cr.Content[0].Text)
	}
	var live struct {
		FirstSync *struct {
			InProgress bool   `json:"in_progress"`
			Phase      string `json:"phase"`
			Fetched    int    `json:"fetched"`
			Total      *int   `json:"total"`
			StartedAt  string `json:"started_at"`
		} `json:"first_sync"`
	}
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &live); err != nil {
		t.Fatalf("unmarshal status payload: %v\n%s", err, cr.Content[0].Text)
	}
	if live.FirstSync == nil {
		t.Fatalf("first_sync absent from gadak_status while a first-sync row is live:\n%s", cr.Content[0].Text)
	}
	fs := live.FirstSync
	if !fs.InProgress || fs.Phase != "issues" || fs.Fetched != 1200 || fs.Total == nil || *fs.Total != 3514 {
		t.Errorf("first_sync = {in_progress:%v phase:%q fetched:%d total:%v} — want the doc `gadak status --json` prints: {true, issues, 1200, 3514}", fs.InProgress, fs.Phase, fs.Fetched, fs.Total)
	}
	if fs.StartedAt == "" {
		t.Errorf("first_sync.started_at empty:\n%s", cr.Content[0].Text)
	}

	// The finished half (same contract as the store: End deletes the row).
	// Key-exact check — the payload also carries the unrelated retention
	// stamp first_sync_at, so a substring match would pass vacuously.
	if err := db.EndSyncProgress(ctx, syncer.SourceID); err != nil {
		t.Fatal(err)
	}
	cr = callToolRaw(t, dbPath, toolStatus, map[string]any{})
	if cr.IsError {
		t.Fatalf("gadak_status errored after the pass ended: %s", cr.Content[0].Text)
	}
	var flat map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &flat); err != nil {
		t.Fatalf("unmarshal status payload: %v\n%s", err, cr.Content[0].Text)
	}
	if _, ok := flat["first_sync"]; ok {
		t.Errorf("finished sync still emits first_sync:\n%s", cr.Content[0].Text)
	}
}

// GDK-1700 FAIL-first, GDK-816 style: the description is the only schema
// text a shell-less agent reads — it must say the payload can be partial
// and that starting a second sync is the wrong move, not just list fields.
func TestStatusDescriptionTeachesFirstSync(t *testing.T) {
	for _, needle := range []string{"first_sync", "results are partial", "do not start a second sync"} {
		if !strings.Contains(toolStatusDescription, needle) {
			t.Errorf("gadak_status description missing %q:\n%s", needle, toolStatusDescription)
		}
	}
}
