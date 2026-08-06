package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/store"
)

// seedProfile writes config.json and an empty migrated scry.db under SCRY_HOME
// for the named profile ("" = default root).
func seedProfile(t *testing.T, home, name string, cfg *config.Config) {
	t.Helper()
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// LoadFor-style dir so Save targets this profile even if global profile differs.
	loaded, err := config.LoadFor(name)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Site = cfg.Site
	loaded.Email = cfg.Email
	loaded.Token = cfg.Token
	loaded.Projects = cfg.Projects
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "scry.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Distinct issue so bootstrap proves we hit the right mirror.
	key := "AAA-1"
	if name != "" && name != "default" {
		key = "BBB-1"
	}
	if err := db.UpsertSource(store.Source{ID: "jira", Kind: "jira", BaseURL: cfg.Site}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:" + key, SourceID: "jira", ExternalID: key, Key: key,
				Title: "fixture " + key, CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: strings.Split(key, "-")[0], Status: "To Do", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}

func testServeMux(t *testing.T) (*http.ServeMux, *workspaceRegistry) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	// Primary (default) + one named profile.
	seedProfile(t, home, "", &config.Config{
		Site:     "https://aaa.example.invalid",
		Email:    "user@example.invalid",
		Token:    "tok-aaa-secret-never-leak",
		Projects: []string{"AAA"},
	})
	seedProfile(t, home, "work", &config.Config{
		Site:     "https://bbb.example.invalid",
		Email:    "other@example.invalid",
		Token:    "tok-bbb-secret-never-leak",
		Projects: []string{"BBB"},
	})

	primaryCfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	api := server.New(db, primaryCfg)
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>spa</html>"))
	})
	reg := newWorkspaceRegistry()
	t.Cleanup(func() { reg.Close() })
	return buildServeMux(api, spa, reg), reg
}

func TestWorkspacesListNoSecrets(t *testing.T) {
	mux, _ := testServeMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, banned := range []string{
		"token", "Token", "tok-aaa-secret-never-leak", "tok-bbb-secret-never-leak",
		"user@example.invalid", "other@example.invalid", "Email", "email",
	} {
		// "email" as a JSON key would be `"email"`; bare substring "email" is too
		// broad if a site URL ever contained it — check structural secrets only
		// for the token strings and the word as a JSON key.
		if banned == "email" || banned == "Email" {
			if strings.Contains(body, `"email"`) || strings.Contains(body, `"Email"`) {
				t.Fatalf("workspaces leaked key %q: %s", banned, body)
			}
			continue
		}
		if banned == "token" || banned == "Token" {
			if strings.Contains(body, `"token"`) || strings.Contains(body, `"Token"`) ||
				strings.Contains(strings.ToLower(body), `"token"`) {
				t.Fatalf("workspaces leaked key %q: %s", banned, body)
			}
			// Also reject the bare word as a field name elsewhere.
			if strings.Contains(body, "token") || strings.Contains(body, "Token") {
				t.Fatalf("workspaces response contains %q: %s", banned, body)
			}
			continue
		}
		if strings.Contains(body, banned) {
			t.Fatalf("workspaces leaked %q: %s", banned, body)
		}
	}

	var doc struct {
		Workspaces []struct {
			Name     string   `json:"name"`
			Site     string   `json:"site"`
			Projects []string `json:"projects"`
			Active   bool     `json:"active"`
			Error    string   `json:"error"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.Workspaces) != 2 {
		t.Fatalf("want 2 workspaces, got %+v", doc.Workspaces)
	}
	if doc.Workspaces[0].Name != "default" || !doc.Workspaces[0].Active {
		t.Fatalf("first entry %+v", doc.Workspaces[0])
	}
	if doc.Workspaces[0].Site != "https://aaa.example.invalid" {
		t.Fatalf("default site %q", doc.Workspaces[0].Site)
	}
	if doc.Workspaces[1].Name != "work" || doc.Workspaces[1].Active {
		t.Fatalf("second entry %+v", doc.Workspaces[1])
	}
	if doc.Workspaces[1].Site != "https://bbb.example.invalid" {
		t.Fatalf("work site %q", doc.Workspaces[1].Site)
	}
}

func TestWorkspaceBootstrap(t *testing.T) {
	mux, _ := testServeMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/w/work/api/v1/issues/bootstrap/", nil)
	// httptest defaults Host to example.com, which the browser guard rejects
	// as a rebinding name; real clients arrive with a loopback Host.
	req.Host = "127.0.0.1:7777"
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var boot struct {
		Issues []struct {
			IssueKey string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &boot); err != nil {
		t.Fatalf("decode: %v body %s", err, rec.Body.String())
	}
	if len(boot.Issues) != 1 || boot.Issues[0].IssueKey != "BBB-1" {
		t.Fatalf("want BBB-1 from work mirror, got %+v", boot.Issues)
	}

	// Primary still has AAA-1.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/issues/bootstrap/", nil)
	req2.Host = "127.0.0.1:7777"
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("primary status %d", rec2.Code)
	}
	var boot2 struct {
		Issues []struct {
			IssueKey string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &boot2); err != nil {
		t.Fatal(err)
	}
	if len(boot2.Issues) != 1 || boot2.Issues[0].IssueKey != "AAA-1" {
		t.Fatalf("primary want AAA-1, got %+v", boot2.Issues)
	}
}

func TestWorkspaceConfigJSON(t *testing.T) {
	mux, _ := testServeMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/w/work/config.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		APIBase  string `json:"apiBase"`
		AuthBase string `json:"authBase"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.APIBase != "/w/work/api/v1/issues/" {
		t.Fatalf("apiBase %q", doc.APIBase)
	}
	if doc.AuthBase != "/w/work/api/v1/auth/" {
		t.Fatalf("authBase %q", doc.AuthBase)
	}
	// No secrets.
	body := rec.Body.String()
	for _, banned := range []string{"tok-bbb-secret-never-leak", "other@example.invalid", `"token"`, `"email"`} {
		if strings.Contains(body, banned) {
			t.Fatalf("config.json leaked %q: %s", banned, body)
		}
	}
}

func TestWorkspaceBadName404(t *testing.T) {
	mux, _ := testServeMux(t)

	for _, path := range []string{
		"/w/bad..name/api/v1/issues/bootstrap/",
		"/w/bad..name/config.json",
		"/w/no-such-profile/api/v1/issues/bootstrap/",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s → %d, want 404", path, rec.Code)
		}
	}
}
