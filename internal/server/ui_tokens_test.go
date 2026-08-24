package server

// GDK-786/791 server-side tests: config.json carries the merged ui block,
// PUT settings/ validates it, and the ui-focus poll always carries the
// configVersion the web compares to refetch without a reload.
//
// FAIL-first: written before the wiring — WebConfig ignored cfg.UI, PUT
// ignored a ui key, and ui-focus answered 204 with no body when nothing was
// pending.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func uiTestConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("GADAK_HOME", t.TempDir())
	_, cfg := fixture(t)
	cfg.UI = &config.UIConfig{
		Tokens: &config.UITokens{Colors: map[string]string{"accent": "#7a4bd0"}},
		TokensByTheme: map[string]*config.UITokens{
			"dark": {Colors: map[string]string{"accent": "#9a6be0"}},
		},
		DataColors: map[string]map[string]string{
			"label":  {"urgent": "#c03030"},
			"type":   {"10007": "#d07020"},
			"status": {"inprogress": "#7e5904"},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// The web binds config.json without the settings catalog: the ui block must
// arrive server-merged (final per-palette CSS variable map + data inks) and
// the document must carry the configVersion the poll compares against.
func TestWebConfigCarriesUI(t *testing.T) {
	cfg := uiTestConfig(t)
	raw, err := WebConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		UI struct {
			Vars       map[string]map[string]string `json:"vars"`
			DataColors map[string]map[string]string `json:"dataColors"`
		} `json:"ui"`
		ConfigVersion string `json:"configVersion"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.UI.Vars["light"]["--color-accent"] != "#7a4bd0" {
		t.Errorf("light vars wrong: %+v", doc.UI.Vars)
	}
	if doc.UI.Vars["dark"]["--color-accent"] != "#9a6be0" {
		t.Errorf("dark overlay must win in vars: %+v", doc.UI.Vars)
	}
	if doc.UI.DataColors["label"]["urgent"] != "#c03030" || doc.UI.DataColors["type"]["10007"] != "#d07020" || doc.UI.DataColors["status"]["inprogress"] != "#7e5904" {
		t.Errorf("dataColors wrong: %+v", doc.UI.DataColors)
	}
	if doc.ConfigVersion == "" {
		t.Error("configVersion missing from config.json")
	}
	// The workspace mount gets the same block with prefixed bases.
	wraw, err := WebConfigBase(cfg, "/w/work")
	if err != nil {
		t.Fatal(err)
	}
	var wdoc struct {
		APIBase string `json:"apiBase"`
		UI      struct {
			Vars map[string]map[string]string `json:"vars"`
		} `json:"ui"`
	}
	if err := json.Unmarshal(wraw, &wdoc); err != nil {
		t.Fatal(err)
	}
	if wdoc.APIBase != "/w/work"+apiBase {
		t.Errorf("apiBase %q", wdoc.APIBase)
	}
	if wdoc.UI.Vars["light"]["--color-accent"] != "#7a4bd0" {
		t.Errorf("workspace config lost ui: %+v", wdoc.UI.Vars)
	}
}

func TestSettingsUIRoundtripAndRefusal(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	h := New(db, cfg)

	put := func(ui string) *httptest.ResponseRecorder {
		body := `{"projects":` + mustJSON(cfg.Projects) + `,"staleThresholdHours":72,"syncIntervalSec":0,"reconcileIntervalSec":0,"ui":` + ui + `}`
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(body)))
		return rec
	}

	// Valid ui persists, echoes back, and lands on disk.
	rec := put(`{"tokens":{"colors":{"accent":"#7a4bd0"}},"dataColors":{"label":{"urgent":"#c03030"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT ui → %d %s", rec.Code, rec.Body.String())
	}
	got := decode[struct {
		UI *config.UIConfig `json:"ui"`
	}](t, rec)
	if got.UI == nil || got.UI.Tokens.Colors["accent"] != "#7a4bd0" || got.UI.DataColors["label"]["urgent"] != "#c03030" {
		t.Fatalf("echo ui wrong: %+v", got.UI)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.UI == nil || saved.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Fatalf("disk ui wrong: %+v", saved.UI)
	}

	// A locked token is refused with the reason and does not clobber the file.
	rec = put(`{"tokens":{"colors":{"bg-base":"#000000"}}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("locked token → %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "locked") {
		t.Fatalf("refusal must say why: %s", rec.Body.String())
	}
	saved, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Fatalf("refused PUT clobbered stored ui: %+v", saved.UI)
	}
	// Display-name keys teach the right key kind.
	rec = put(`{"dataColors":{"status":{"In Progress":"#7e5904"}}}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "status_category") {
		t.Fatalf("status display name → %d %s", rec.Code, rec.Body.String())
	}
	// An older client that omits ui must not wipe it.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(`{"projects":[],"staleThresholdHours":72,"syncIntervalSec":0,"reconcileIntervalSec":0}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT without ui → %d %s", rec.Code, rec.Body.String())
	}
	saved, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.UI == nil || saved.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Fatalf("omitted ui key wiped overrides: %+v", saved.UI)
	}
}

// The poll doubles as the settings-change signal: with nothing pending it
// still answers 200 with configVersion (204→200, GDK-791).
func TestUIFocusAlwaysCarriesConfigVersion(t *testing.T) {
	cfg := uiTestConfig(t)
	h := New(nil, cfg)
	rec := get(t, h, apiBase+"ui-focus/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("idle poll → %d, want 200 (was 204)", rec.Code)
	}
	got := decode[struct {
		ConfigVersion string `json:"configVersion"`
	}](t, rec)
	if got.ConfigVersion == "" {
		t.Fatal("idle poll must carry configVersion")
	}
	// A write moves it — the client's refetch trigger.
	cfg2 := *cfg
	cfg2.UI = nil
	if err := cfg2.Save(); err != nil {
		t.Fatal(err)
	}
	rec = get(t, h, apiBase+"ui-focus/", nil)
	next := decode[struct {
		ConfigVersion string `json:"configVersion"`
	}](t, rec)
	if next.ConfigVersion == got.ConfigVersion {
		t.Fatalf("configVersion did not move after save: %q", next.ConfigVersion)
	}
}
