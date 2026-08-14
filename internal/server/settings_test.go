package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func TestSettingsSyncIntervalsRoundtrip(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	h := New(db, cfg)

	// Valid custom intervals persist and re-read.
	put := settingsDoc{
		Projects:             cfg.Projects,
		StaleThresholdHours:  72,
		SyncIntervalSec:      30,
		ReconcileIntervalSec: 3600,
		Features:             map[string]bool{"teamGroups": true},
	}
	body, err := json.Marshal(put)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT valid intervals → %d %s", rec.Code, rec.Body.String())
	}
	got := decode[settingsDoc](t, rec)
	if got.SyncIntervalSec != 30 || got.ReconcileIntervalSec != 3600 {
		t.Fatalf("response intervals sync=%d reconcile=%d", got.SyncIntervalSec, got.ReconcileIntervalSec)
	}

	again := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if again.SyncIntervalSec != 30 || again.ReconcileIntervalSec != 3600 {
		t.Fatalf("re-GET intervals sync=%d reconcile=%d", again.SyncIntervalSec, again.ReconcileIntervalSec)
	}

	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.SyncIntervalSec != 30 || saved.ReconcileIntervalSec != 3600 {
		t.Fatalf("disk intervals %+v", saved)
	}
	// Credential block must survive.
	if saved.Token != "secret-token" || saved.Email != "hc@example.com" {
		t.Fatalf("credential lost: email=%q token=%q", saved.Email, saved.Token)
	}

	// Zero means "use defaults" and is accepted.
	put.SyncIntervalSec = 0
	put.ReconcileIntervalSec = 0
	body, _ = json.Marshal(put)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT zero intervals → %d %s", rec.Code, rec.Body.String())
	}
	got = decode[settingsDoc](t, rec)
	if got.SyncIntervalSec != 0 || got.ReconcileIntervalSec != 0 {
		t.Fatalf("zero not stored: %+v", got)
	}
}

func TestSettingsIntervalFloorsReject(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	h := New(db, cfg)

	// Seed a known-good value so a rejected PUT must not clobber the file.
	seed := settingsDoc{
		Projects:             cfg.Projects,
		StaleThresholdHours:  72,
		SyncIntervalSec:      60,
		ReconcileIntervalSec: 7200,
	}
	body, _ := json.Marshal(seed)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed → %d %s", rec.Code, rec.Body.String())
	}

	cases := []struct {
		name                  string
		syncSec, reconcileSec int
		wantSubstr            string
	}{
		{"sync below min", 5, 3600, "syncIntervalSec"},
		{"reconcile below min", 60, 60, "reconcileIntervalSec"},
		{"sync one below floor", config.MinSyncIntervalSec - 1, 0, "syncIntervalSec"},
		{"reconcile one below floor", 0, config.MinReconcileIntervalSec - 1, "reconcileIntervalSec"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := settingsDoc{
				Projects:             cfg.Projects,
				StaleThresholdHours:  72,
				SyncIntervalSec:      tc.syncSec,
				ReconcileIntervalSec: tc.reconcileSec,
			}
			b, _ := json.Marshal(doc)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
			if r.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400; body %s", r.Code, r.Body.String())
			}
			var errBody map[string]string
			if err := json.Unmarshal(r.Body.Bytes(), &errBody); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if !strings.Contains(errBody["error"], tc.wantSubstr) {
				t.Fatalf("error %q, want substring %q", errBody["error"], tc.wantSubstr)
			}
		})
	}

	// Rejected writes must leave the previous valid values on disk.
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.SyncIntervalSec != 60 || saved.ReconcileIntervalSec != 7200 {
		t.Fatalf("rejected PUT overwrote intervals: sync=%d reconcile=%d", saved.SyncIntervalSec, saved.ReconcileIntervalSec)
	}
	if saved.Token != "secret-token" {
		t.Fatalf("credential lost after rejects: %q", saved.Token)
	}
}

func TestSettingsRuntimeReadOnlyNoSecrets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	// Open the mirror at the profile's default path so runtime.dbPath matches.
	dbPath := filepath.Join(home, "gadak.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", ExternalID: "1", Key: "NMB-1",
				Title: "one", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new"},
			Comments: []store.Comment{{
				ID: "jira:c1", ExternalID: "c1", Author: "a",
				BodyADF: json.RawMessage(`{}`), BodyText: "hi", CreatedAt: "2026-07-01T00:00:00.000Z",
			}},
		}},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.RecordSync(context.Background(), "jira", store.SyncResult{Watermark: "2026-08-01T00:00:00.000Z", FullSync: true}); err != nil {
		t.Fatalf("record: %v", err)
	}

	cfg := &config.Config{
		Site: "https://x.atlassian.net", Email: "hc@example.com", Token: "secret-token-xyz",
		Projects: []string{"NMB"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save cfg: %v", err)
	}
	h := New(db, cfg)

	got := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if got.Runtime == nil {
		t.Fatal("runtime missing")
	}
	rt := got.Runtime
	if rt.Profile != "default" {
		t.Fatalf("profile %q", rt.Profile)
	}
	if rt.DBPath != dbPath {
		t.Fatalf("dbPath %q, want %q", rt.DBPath, dbPath)
	}
	if rt.ConfigPath != filepath.Join(home, "config.json") {
		t.Fatalf("configPath %q", rt.ConfigPath)
	}
	if rt.IssueCount != 1 || rt.CommentCount != 1 {
		t.Fatalf("counts issues=%d comments=%d", rt.IssueCount, rt.CommentCount)
	}
	if rt.Watermark != "2026-08-01T00:00:00.000Z" {
		t.Fatalf("watermark %q", rt.Watermark)
	}
	if rt.LastFullSyncAt == nil || *rt.LastFullSyncAt == "" {
		t.Fatal("lastFullSyncAt missing")
	}
	if rt.SchemaVersion <= 0 {
		t.Fatalf("schemaVersion %d", rt.SchemaVersion)
	}
	if rt.DefaultSyncIntervalSec != config.DefaultSyncIntervalSec {
		t.Fatalf("default sync %d", rt.DefaultSyncIntervalSec)
	}
	if rt.DefaultReconcileIntervalSec != config.DefaultReconcileIntervalSec {
		t.Fatalf("default reconcile %d", rt.DefaultReconcileIntervalSec)
	}
	if rt.GadakVersion == "" {
		t.Fatal("gadakVersion empty")
	}
	if rt.DBSizeBytes <= 0 || rt.DBSizeHuman == "" || rt.DBSizeHuman == "—" {
		t.Fatalf("db size bytes=%d human=%q", rt.DBSizeBytes, rt.DBSizeHuman)
	}
	if rt.DBModifiedAt == nil {
		t.Fatal("dbModifiedAt missing")
	}

	// Secrets must never appear in the GET body (runtime or top-level).
	raw := get(t, h, apiBase+"settings/", nil).Body.String()
	for _, secret := range []string{"secret-token-xyz", "secret-token", cfg.Token, "hc@example.com"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("settings leaked %q: %s", secret, raw)
		}
	}
	// Structural check: runtime must not grow email/token fields by accident.
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("probe: %v", err)
	}
	rtMap, _ := probe["runtime"].(map[string]any)
	for _, banned := range []string{"token", "email", "password", "authorization"} {
		if _, ok := rtMap[banned]; ok {
			t.Fatalf("runtime carries banned key %q", banned)
		}
	}

	// PUT must ignore a client-supplied runtime (and must not require one).
	putBody := map[string]any{
		"projects":             []string{"NMB"},
		"staleThresholdHours":  48,
		"syncIntervalSec":      120,
		"reconcileIntervalSec": 7200,
		"runtime": map[string]any{
			"profile": "evil",
			"token":   "should-not-be-stored",
			"dbPath":  "/tmp/evil.db",
		},
	}
	b, _ := json.Marshal(putBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT with runtime → %d %s", rec.Code, rec.Body.String())
	}
	after := decode[settingsDoc](t, rec)
	if after.Runtime == nil || after.Runtime.Profile != "default" {
		t.Fatalf("runtime not rebuilt server-side: %+v", after.Runtime)
	}
	if after.Runtime.DBPath != dbPath {
		t.Fatalf("client runtime path accepted: %q", after.Runtime.DBPath)
	}
	if after.StaleThresholdHours != 48 || after.SyncIntervalSec != 120 {
		t.Fatalf("writable fields not stored: %+v", after)
	}
	// Token still only on disk, never in response.
	if strings.Contains(rec.Body.String(), "should-not-be-stored") || strings.Contains(rec.Body.String(), "secret-token") {
		t.Fatalf("token leaked after PUT: %s", rec.Body.String())
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Token != "secret-token-xyz" {
		t.Fatalf("credential lost: %q", saved.Token)
	}
}

// --- Confluence enable/disable via PUT settings/ (product on-switch) ---

// TestPutSettingsConfluenceEnableFromOff: enabled:true + spaces turns Confluence on.
func TestPutSettingsConfluenceEnableFromOff(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Confluence = nil
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 72,
		"confluence":          map[string]any{"enabled": true, "spaces": []string{"ENG"}},
	}
	b, _ := json.Marshal(putBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT enable → %d %s", rec.Code, rec.Body.String())
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Confluence == nil || len(saved.Confluence.Spaces) != 1 || saved.Confluence.Spaces[0] != "ENG" {
		t.Fatalf("disk Confluence.Spaces want [ENG], got %+v", saved.Confluence)
	}
}

// TestPutSettingsConfluenceDisable: enabled:false turns off; pages stay on disk.
func TestPutSettingsConfluenceDisable(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixturePages(t)
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"ENG"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	before, err := db.TableCount(context.Background(), "pages")
	if err != nil {
		t.Fatalf("page count before: %v", err)
	}
	if before == 0 {
		t.Fatal("fixturePages should seed pages")
	}
	h := New(db, cfg)

	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 72,
		"confluence":          map[string]any{"enabled": false},
	}
	b, _ := json.Marshal(putBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT disable → %d %s", rec.Code, rec.Body.String())
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Confluence != nil {
		t.Fatalf("want Confluence nil after disable, got %+v", saved.Confluence)
	}
	after, err := db.TableCount(context.Background(), "pages")
	if err != nil {
		t.Fatalf("page count after: %v", err)
	}
	if after != before {
		t.Fatalf("disable must not delete pages: before=%d after=%d", before, after)
	}
	// Response omits confluence when off.
	resp := decode[settingsDoc](t, rec)
	if resp.Confluence != nil {
		t.Fatalf("response should omit confluence when off: %+v", resp.Confluence)
	}
}

// TestPutSettingsConfluenceSpacesOnlyWhileOff: spaces without enabled still 400 when off.
func TestPutSettingsConfluenceSpacesOnlyWhileOff(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Confluence = nil
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 72,
		"confluence":          map[string]any{"spaces": []string{"X"}},
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
}

// TestPutSettingsConfluenceSpacesOnlyWhileOn: spaces without enabled updates when on.
func TestPutSettingsConfluenceSpacesOnlyWhileOn(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"OLD"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 72,
		"confluence":          map[string]any{"spaces": []string{"X"}},
	}
	b, _ := json.Marshal(putBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT spaces-only → %d %s", rec.Code, rec.Body.String())
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Confluence == nil || len(saved.Confluence.Spaces) != 1 || saved.Confluence.Spaces[0] != "X" {
		t.Fatalf("disk spaces %+v", saved.Confluence)
	}
}

// TestPutSettingsOmitsConfluenceKeyPreserves: no confluence key leaves config alone.
func TestPutSettingsOmitsConfluenceKeyPreserves(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"KEEP"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

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

// TestPutSettingsConfluenceEnableReplacesSpaces: enabled:true while already on updates spaces.
func TestPutSettingsConfluenceEnableReplacesSpaces(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"OLD"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	putBody := map[string]any{
		"projects":            cfg.Projects,
		"staleThresholdHours": 72,
		"confluence":          map[string]any{"enabled": true, "spaces": []string{"ENG", "PROD"}},
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
	if saved.Confluence == nil || len(saved.Confluence.Spaces) != 2 ||
		saved.Confluence.Spaces[0] != "ENG" || saved.Confluence.Spaces[1] != "PROD" {
		t.Fatalf("disk %+v", saved.Confluence)
	}
}

func TestSettingsRuntimeProfileName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("demo")
	t.Cleanup(func() { config.SetProfile("") })

	// Profile dir is created on first Save/Path use.
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatalf("dbpath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	h := New(db, &config.Config{})
	got := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if got.Runtime == nil || got.Runtime.Profile != "demo" {
		t.Fatalf("profile %+v", got.Runtime)
	}
	if !strings.Contains(got.Runtime.DBPath, filepath.Join("profiles", "demo")) {
		t.Fatalf("dbPath should be under profiles/demo: %q", got.Runtime.DBPath)
	}
}

func TestWebConfigProfileName(t *testing.T) {
	t.Cleanup(func() { config.SetProfile("") })

	config.SetProfile("")
	doc, err := WebConfig(&config.Config{Site: "https://x.example"})
	if err != nil {
		t.Fatal(err)
	}
	var got webConfigDoc
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatal(err)
	}
	if got.Profile != "default" {
		t.Errorf("empty profile = %q, want default", got.Profile)
	}

	config.SetProfile("work")
	doc, err = WebConfig(&config.Config{Site: "https://x.example"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatal(err)
	}
	if got.Profile != "work" {
		t.Errorf("named profile = %q, want work", got.Profile)
	}

	// Workspace mount names the profile from the path, not the process primary.
	config.SetProfile("work")
	doc, err = WebConfigBase(&config.Config{Site: "https://x.example"}, "/w/demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatal(err)
	}
	if got.Profile != "demo" {
		t.Errorf("/w/demo profile = %q, want demo", got.Profile)
	}
	if got.APIBase != "/w/demo"+apiBase {
		t.Errorf("apiBase = %q", got.APIBase)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
	}
	for _, tc := range cases {
		if got := humanBytes(tc.n); got != tc.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
