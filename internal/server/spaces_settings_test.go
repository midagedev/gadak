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

func TestSettingsSpacesNotConfigured(t *testing.T) {
	h := spacesHandler(t, &config.Config{
		Site: "https://x.atlassian.net", Email: "hc@example.com", Token: "tok",
		// Confluence: nil
	}, nil)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "confluence_not_configured" {
		t.Fatalf("error %q, want confluence_not_configured", body["error"])
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
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body %s", err, rec.Body.String())
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
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
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
