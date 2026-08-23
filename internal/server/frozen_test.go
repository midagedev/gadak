package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestPutSettingsDoesNotClearFrozen: PUT settings/ copies the live config
// (`next := *prev`) so frozen must survive a scope edit. GDK-181 — a settings
// save must not un-freeze a scrubbed fixture.
func TestPutSettingsDoesNotClearFrozen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	db, cfg := fixture(t)
	cfg.Frozen = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	var kicks int
	h.s.syncKick = func(*config.Config, bool) bool {
		kicks++
		return true
	}

	rec := putSettingsJSON(t, h, map[string]any{
		"projects":            []string{"NMB", "NMA", "OPS"},
		"staleThresholdHours": 72,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT → %d %s", rec.Code, rec.Body.String())
	}
	if kicks != 0 {
		t.Fatalf("frozen workspace kicked sync %d times, want 0", kicks)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !loaded.SyncFrozen() {
		t.Fatal("Load() lost frozen after PUT settings/")
	}

	raw, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode config.json: %v", err)
	}
	if doc["frozen"] != true {
		t.Fatalf("disk config frozen=%v, want true; body %s", doc["frozen"], raw)
	}
}

// TestStartSyncFrozenIsNotInProgress: POST sync/ on a frozen workspace must
// not start a job and must not reuse the 409 sync_in_progress code.
func TestStartSyncFrozenIsNotInProgress(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	db, cfg := fixture(t)
	cfg.Frozen = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"sync/", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST sync/ → %d %s, want 409", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "workspace_frozen") {
		t.Fatalf("body %s, want workspace_frozen", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sync_in_progress") {
		t.Fatalf("frozen POST sync/ disguised as sync_in_progress: %s", rec.Body.String())
	}
}

// frozenHandler is a frozen workspace with a live-looking credential — the
// GDK-181 incident shape. The origin is never contacted: the refusal must
// come from the client mint, before any request leaves.
func frozenHandler(t *testing.T) http.Handler {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	db, cfg := fixture(t)
	cfg.Frozen = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	return New(db, cfg)
}

// TestFrozenWorkspaceRefusesWrites: GDK-507 decision (b) — frozen means no
// request leaves for the origin, writes included. A scrubbed demo fixture
// with a live credential must not be able to create real comments.
func TestFrozenWorkspaceRefusesWrites(t *testing.T) {
	h := frozenHandler(t)
	for _, tc := range []struct{ path, body string }{
		{apiBase + "NMB-1/comment/", `{"text":"hi"}`},
		{apiBase + "NMB-1/transition/", `{"transition_id":"31"}`},
		{apiBase + "NMB-1/link/", `{"type":"blocks","key":"NMB-2"}`},
	} {
		rec := send(t, h, http.MethodPost, tc.path, tc.body)
		if rec.Code != http.StatusConflict {
			t.Fatalf("POST %s → %d %s, want 409", tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "workspace_frozen") {
			t.Fatalf("POST %s body %s, want workspace_frozen", tc.path, rec.Body.String())
		}
	}
}

// TestFrozenWorkspaceRefusesResync: the per-issue resync is a pull — the
// GDK-181 class with a scale of one row. It must be gated like the bulk sync.
func TestFrozenWorkspaceRefusesResync(t *testing.T) {
	h := frozenHandler(t)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/resync/", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST resync → %d %s, want 409", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "workspace_frozen") {
		t.Fatalf("body %s, want workspace_frozen", rec.Body.String())
	}
}
