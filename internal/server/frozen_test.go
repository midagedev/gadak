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
