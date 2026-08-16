package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func TestSettingsFieldSpecsAndUsageReadOnly(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Fields = []config.FieldSpec{
		{Alias: "severity", Label: "Severity Level", IDs: []string{"customfield_10", "customfield_20"}, Role: "facet", Kind: "option", Auto: true},
	}
	if err := db.ReplaceFieldUsage(context.Background(), []store.FieldUsageRow{
		{ProjectKey: "NMB", Alias: "severity", Filled: 2, Total: 3},
	}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	got := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if len(got.FieldSpecsOut) != 1 || got.FieldSpecsOut[0].Alias != "severity" {
		t.Fatalf("fieldSpecs = %+v", got.FieldSpecsOut)
	}
	if got.FieldUsage["NMB"]["severity"] != 2 {
		t.Fatalf("fieldUsage = %+v", got.FieldUsage)
	}

	// PUT without Fields must preserve cfg.Fields (copy of live config).
	// settingsDoc has no Fields write path; PUT copies other keys but keeps Fields.
	put := settingsDoc{
		Projects:            cfg.Projects,
		StaleThresholdHours: 72,
		FieldMap:            map[string]string{"noise": "customfield_1"},
	}
	body, _ := json.Marshal(put)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT → %d %s", rec.Code, rec.Body.String())
	}
	// Live config still has Fields
	if len(cfg.Fields) != 1 {
		// New(db, cfg) stores pointer; handlePut copies next := *s.config()
		// and Save — Fields on next came from live config copy.
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Fields) != 1 || saved.Fields[0].Alias != "severity" {
		t.Fatalf("PUT wiped Fields: %+v", saved.Fields)
	}
	// PUT still writes fieldMap, but LoadFor migrates leftover keys. Fields
	// is already set, so FieldSpecs sole-truth drops the map (noise is not
	// unioned in). GDK-149: this file is outside the migration whitelist.
	if len(saved.FieldMap) != 0 {
		t.Fatalf("Load must migrate leftover fieldMap, still %v", saved.FieldMap)
	}
}

func TestEditMetaFromFieldSpecs(t *testing.T) {
	f, h, cfg := writable(t)
	// Clear legacy allowlist; use specs with Kind instead.
	cfg.EditableFields = nil
	cfg.Fields = []config.FieldSpec{
		{Alias: "solution", Label: "Solution", IDs: []string{"customfield_999", "customfield_10092"}, Role: "facet", Kind: "option", Auto: true},
		{Alias: "tags", Label: "Tags", IDs: []string{"customfield_multi"}, Role: "facet", Kind: "multi_option", Auto: true},
	}
	// editmeta from fake already has customfield_10092 as option; add multi_option.
	// fakeJira editMeta is a fixed JSON string — extend it.
	f.editMeta = `{
		"customfield_10092": {"schema":{"type":"option"},"allowedValues":[{"id":"10160","value":"Fixed"}]},
		"fixVersions": {"schema":{"type":"array","items":"version"},"allowedValues":[{"id":"v1","name":"1.2.0"}]},
		"customfield_20000": {"schema":{"type":"user"}},
		"customfield_multi": {"schema":{"type":"array","items":"option"},"allowedValues":[{"id":"1","value":"A"},{"id":"2","value":"B"}]}
	}`

	got := decode[struct {
		Fields map[string]struct {
			Kind     string              `json:"kind"`
			Editable bool                `json:"editable"`
			Options  []map[string]string `json:"options"`
		} `json:"fields"`
	}](t, get(t, h, apiBase+"NMB-1/editmeta/", nil))

	if got.Fields["solution"].Kind != "option" {
		t.Fatalf("solution via specs: %+v", got.Fields["solution"])
	}
	if got.Fields["tags"].Kind != "multi_option" {
		t.Fatalf("multi_option kind: %+v", got.Fields["tags"])
	}

	// multi_option write payload
	rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"tags","value":["1","2"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("multi_option edit → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_multi":[{"id":"1"},{"id":"2"}]}}` {
		t.Fatalf("multi_option body %s", got)
	}

	// Resolve first present id: customfield_999 absent, uses 10092
	rec = send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"solution","value":"10160"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("solution edit → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_10092":{"id":"10160"}}}` {
		t.Fatalf("resolved id body %s", got)
	}
}
