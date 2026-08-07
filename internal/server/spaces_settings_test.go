package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/scry/internal/config"
)

// confSpacesMock is a minimal Confluence Cloud stand-in for GET settings/spaces/.
// Paths match confluence.Client (site origin + /wiki/rest/api/space).
type confSpacesMock struct {
	spaces []map[string]any
	token  string // expected Basic-auth bearer of email:token — never returned
}

func (m *confSpacesMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != "/wiki/rest/api/space" {
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results": m.spaces,
		"size":    len(m.spaces),
		"limit":   100,
		"start":   0,
	})
}

func spacesHandler(t *testing.T, cfg *config.Config, mock *confSpacesMock) http.Handler {
	t.Helper()
	t.Setenv("SCRY_HOME", t.TempDir())
	db, base := fixture(t)
	// Keep credential shape; redirect site to the mock when one is provided.
	cfg.Email = base.Email
	if cfg.Token == "" {
		cfg.Token = "spaces-test-token-secret"
	}
	if cfg.Site == "" {
		cfg.Site = base.Site
	}
	if mock != nil {
		srv := httptest.NewServer(mock)
		t.Cleanup(srv.Close)
		cfg.Site = srv.URL
		mock.token = cfg.Token
	}
	// Projects etc. from fixture are irrelevant; start from a clean copy.
	live := *cfg
	if live.Projects == nil {
		live.Projects = []string{"NMB"}
	}
	return New(db, &live)
}

// TestSettingsSpacesOffNoCredential: discovery needs a credential even when
// Confluence is off — no longer 400 confluence_not_configured.
func TestSettingsSpacesOffNoCredential(t *testing.T) {
	t.Setenv("SCRY_HOME", t.TempDir())
	db, base := fixture(t)
	// Explicitly no credential, Confluence off.
	live := &config.Config{
		Site:       "https://example.invalid",
		Projects:   base.Projects,
		Confluence: nil,
	}
	h := New(db, live)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409; body %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "credential_required" {
		t.Fatalf("error %q, want credential_required (not confluence_not_configured)", body["error"])
	}
}

// TestSettingsSpacesOffListsWithEnabledFalse: when Confluence is off but a
// credential is present, listing still works; selected and all_global stay false.
func TestSettingsSpacesOffListsWithEnabledFalse(t *testing.T) {
	mock := &confSpacesMock{spaces: []map[string]any{
		{"key": "ENG", "name": "Engineering", "type": "global"},
		{"key": "OPS", "name": "Operations", "type": "global"},
	}}
	h := spacesHandler(t, &config.Config{
		// Confluence: nil (off)
	}, mock)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Spaces []struct {
			Key      string `json:"key"`
			Selected bool   `json:"selected"`
		} `json:"spaces"`
		AllGlobalWhenEmpty bool `json:"all_global_when_empty"`
		Enabled            bool `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Enabled {
		t.Fatal("enabled should be false when Confluence is off")
	}
	if body.AllGlobalWhenEmpty {
		t.Fatal("all_global_when_empty must be false when off (off ≠ all-global)")
	}
	if len(body.Spaces) != 2 {
		t.Fatalf("spaces %+v", body.Spaces)
	}
	for _, s := range body.Spaces {
		if s.Selected {
			t.Fatalf("off must not mark selected: %+v", body.Spaces)
		}
	}
}

func TestSettingsSpacesListsSelectedAndSorts(t *testing.T) {
	mock := &confSpacesMock{spaces: []map[string]any{
		// Deliberately unsorted: personal first, globals reverse-alpha.
		{"key": "ZPER", "name": "Zed personal", "type": "personal"},
		{"key": "OPS", "name": "Operations", "type": "global"},
		{"key": "ENG", "name": "Engineering", "type": "global"},
		{"key": "APER", "name": "Alice personal", "type": "personal"},
	}}
	h := spacesHandler(t, &config.Config{
		Confluence: &config.ConfluenceConfig{Spaces: []string{"ENG", "ZPER"}},
	}, mock)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Spaces []struct {
			Key      string `json:"key"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			Selected bool   `json:"selected"`
		} `json:"spaces"`
		AllGlobalWhenEmpty bool `json:"all_global_when_empty"`
		Enabled            bool `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body %s", err, rec.Body.String())
	}
	if !body.Enabled {
		t.Fatal("enabled should be true when Confluence is configured")
	}
	if body.AllGlobalWhenEmpty {
		t.Fatal("all_global_when_empty should be false when spaces are configured")
	}
	// global first (name alpha), then personal (name alpha).
	wantKeys := []string{"ENG", "OPS", "APER", "ZPER"}
	if len(body.Spaces) != len(wantKeys) {
		t.Fatalf("spaces %+v", body.Spaces)
	}
	for i, k := range wantKeys {
		if body.Spaces[i].Key != k {
			t.Fatalf("order[%d]=%s, want %s; full %+v", i, body.Spaces[i].Key, k, body.Spaces)
		}
	}
	if !body.Spaces[0].Selected || body.Spaces[1].Selected || body.Spaces[2].Selected || !body.Spaces[3].Selected {
		t.Fatalf("selected flags %+v", body.Spaces)
	}
	if body.Spaces[0].Name != "Engineering" || body.Spaces[0].Type != "global" {
		t.Fatalf("ENG row %+v", body.Spaces[0])
	}

	// Credentials must never appear in the body (workspace-list pattern).
	raw := rec.Body.String()
	for _, secret := range []string{"spaces-test-token-secret", "secret-token", `"token"`, `"email"`, "hc@example.com"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("spaces response leaked %q: %s", secret, raw)
		}
	}
}

func TestSettingsSpacesEmptyMeansAllGlobalFlag(t *testing.T) {
	mock := &confSpacesMock{spaces: []map[string]any{
		{"key": "ENG", "name": "Engineering", "type": "global"},
		{"key": "~42", "name": "Pat", "type": "personal"},
	}}
	// Empty Spaces slice: sync treats as all global; UI must not mark selected.
	h := spacesHandler(t, &config.Config{
		Confluence: &config.ConfluenceConfig{Spaces: nil},
	}, mock)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Spaces []struct {
			Key      string `json:"key"`
			Selected bool   `json:"selected"`
			Type     string `json:"type"`
		} `json:"spaces"`
		AllGlobalWhenEmpty bool `json:"all_global_when_empty"`
		Enabled            bool `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Enabled {
		t.Fatal("enabled should be true when Confluence is configured (empty spaces = all global)")
	}
	if !body.AllGlobalWhenEmpty {
		t.Fatal("want all_global_when_empty true when config spaces empty")
	}
	if len(body.Spaces) != 2 {
		t.Fatalf("spaces %+v", body.Spaces)
	}
	for _, s := range body.Spaces {
		if s.Selected {
			t.Fatalf("empty config must not mark selected: %+v", body.Spaces)
		}
	}
	// Still global-first order.
	if body.Spaces[0].Key != "ENG" || body.Spaces[0].Type != "global" {
		t.Fatalf("order %+v", body.Spaces)
	}
}

func TestPutSettingsConfluenceSpacesRoundtrip(t *testing.T) {
	t.Setenv("SCRY_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"OLD"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	// GET exposes current spaces.
	got := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if got.Confluence == nil || len(got.Confluence.Spaces) != 1 || got.Confluence.Spaces[0] != "OLD" {
		t.Fatalf("GET confluence before PUT: %+v", got.Confluence)
	}

	// PUT non-empty list.
	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 72,
		"confluence":          map[string]any{"spaces": []string{"ENG", "OPS"}},
	}
	b, _ := json.Marshal(putBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT spaces → %d %s", rec.Code, rec.Body.String())
	}
	after := decode[settingsDoc](t, rec)
	if after.Confluence == nil || len(after.Confluence.Spaces) != 2 ||
		after.Confluence.Spaces[0] != "ENG" || after.Confluence.Spaces[1] != "OPS" {
		t.Fatalf("PUT response confluence %+v", after.Confluence)
	}

	// Config on disk / live pointer.
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Confluence == nil || len(saved.Confluence.Spaces) != 2 ||
		saved.Confluence.Spaces[0] != "ENG" || saved.Confluence.Spaces[1] != "OPS" {
		t.Fatalf("disk confluence %+v", saved.Confluence)
	}

	// Empty array = all global (valid).
	putBody["confluence"] = map[string]any{"spaces": []string{}}
	b, _ = json.Marshal(putBody)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT empty spaces → %d %s", rec.Code, rec.Body.String())
	}
	empty := decode[settingsDoc](t, rec)
	// spaces omitempty: empty list may be absent in JSON → nil after unmarshal.
	if empty.Confluence == nil || len(empty.Confluence.Spaces) != 0 {
		t.Fatalf("empty spaces response %+v", empty.Confluence)
	}
	saved, err = config.Load()
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	if saved.Confluence == nil || len(saved.Confluence.Spaces) != 0 {
		t.Fatalf("disk empty spaces %+v", saved.Confluence)
	}
	// Confluence still enabled — empty spaces must not nil the section.
	if saved.Confluence == nil {
		t.Fatal("empty spaces cleared confluence section")
	}

	// Re-GET still exposes the confluence section (spaces empty).
	again := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if again.Confluence == nil || len(again.Confluence.Spaces) != 0 {
		t.Fatalf("re-GET empty %+v", again.Confluence)
	}
}

func TestPutSettingsConfluenceSpacesNotConfigured(t *testing.T) {
	t.Setenv("SCRY_HOME", t.TempDir())
	db, cfg := fixture(t)
	// No Confluence section — PUT must not invent one.
	cfg.Confluence = nil
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	// GET omits confluence when unconfigured.
	got := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if got.Confluence != nil {
		t.Fatalf("GET should omit confluence when nil: %+v", got.Confluence)
	}

	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 72,
		"confluence":          map[string]any{"spaces": []string{"ENG"}},
	}
	b, _ := json.Marshal(putBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	var errBody map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody["error"] != "confluence_not_configured" {
		t.Fatalf("error %q", errBody["error"])
	}

	// Rejected write must leave Confluence nil on disk.
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Confluence != nil {
		t.Fatalf("PUT activated confluence: %+v", saved.Confluence)
	}
}

func TestPutSettingsOmitsConfluenceKeyLeavesSpaces(t *testing.T) {
	t.Setenv("SCRY_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"KEEP"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	// PUT without confluence key (older client) must not wipe spaces.
	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 48,
	}
	b, _ := json.Marshal(putBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT → %d %s", rec.Code, rec.Body.String())
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Confluence == nil || len(saved.Confluence.Spaces) != 1 || saved.Confluence.Spaces[0] != "KEEP" {
		t.Fatalf("spaces wiped: %+v", saved.Confluence)
	}
}
