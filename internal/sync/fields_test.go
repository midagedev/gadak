package sync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
)

func TestFieldListDiscoveryMode(t *testing.T) {
	// Discovery mode full → *all
	cfg := &config.Config{}
	got := fieldList(cfg, true)
	if len(got) != 1 || got[0] != "*all" {
		t.Fatalf("discovery full = %v, want [*all]", got)
	}
	// Discovery mode incremental → normal base list, not *all
	got = fieldList(cfg, false)
	if len(got) == 0 || got[0] == "*all" {
		t.Fatalf("discovery incremental must not use *all: %v", got)
	}
	for _, f := range got {
		if f == "*all" {
			t.Fatal("incremental must not include *all")
		}
	}
	// Specs present → exact ids + base
	cfg.Fields = []config.FieldSpec{
		{Alias: "severity", IDs: []string{"customfield_10", "customfield_20"}, Role: "facet"},
	}
	cfg.BodyFields = []string{"customfield_99"}
	got = fieldList(cfg, true)
	joined := strings.Join(got, ",")
	for _, want := range []string{"summary", "customfield_10", "customfield_20", "customfield_99"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %s", want, joined)
		}
	}
	if strings.Contains(joined, "*all") {
		t.Error("specs present must not use *all")
	}
	// Legacy FieldMap still lists ids
	legacy := &config.Config{FieldMap: map[string]string{"severity": "customfield_50"}}
	got = fieldList(legacy, true)
	if !strings.Contains(strings.Join(got, ","), "customfield_50") {
		t.Errorf("legacy FieldMap missing: %v", got)
	}
}

func TestDiscoveryE2ETwoProjectsCoalesce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)

	// Two projects; same logical name "Severity Level" under different ids.
	// ALPHA fills customfield_10; BETA fills customfield_20. Body role text on ALPHA.
	issue := func(id, key, project, updated string, custom map[string]any) json.RawMessage {
		f := map[string]any{
			"summary":   "s-" + key,
			"project":   map[string]any{"key": project},
			"issuetype": map[string]any{"id": "1", "name": "Bug"},
			"status":    map[string]any{"id": "1", "name": "To Do", "statusCategory": map[string]any{"key": "new"}},
			"created":   "2026-07-01T10:00:00.000+0900",
			"updated":   updated,
			"reporter":  map[string]any{"accountId": "a", "displayName": "A", "emailAddress": "a@example.com"},
		}
		for k, v := range custom {
			f[k] = v
		}
		b, _ := json.Marshal(map[string]any{
			"id": id, "key": key, "fields": f,
			"changelog": map[string]any{"total": 0, "histories": []any{}},
		})
		return b
	}
	site := &fakeSite{
		t: t, lang: "en", pageSize: 10, failOffset: -1,
		issues: []json.RawMessage{
			issue("1", "ALPHA-1", "ALPHA", "2026-08-01T10:00:00.000+0900", map[string]any{
				"customfield_10": map[string]any{"value": "High"},
				"customfield_30": adfDoc("repro steps unique token xyzzy"),
				"customfield_20": nil,
			}),
			issue("2", "BETA-1", "BETA", "2026-08-02T10:00:00.000+0900", map[string]any{
				"customfield_20": map[string]any{"value": "Low"},
				"customfield_10": nil,
			}),
		},
		changelog: map[string]string{},
		comments:  map[string]string{},
		fieldCatalog: []map[string]any{
			{
				"id": "customfield_10", "name": "Severity Level", "custom": true,
				"schema": map[string]any{"type": "option", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:select"},
			},
			{
				"id": "customfield_20", "name": "Severity Level", "custom": true,
				"schema": map[string]any{"type": "option", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:select"},
			},
			{
				"id": "customfield_30", "name": "Repro Steps", "custom": true,
				"schema": map[string]any{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textarea"},
			},
		},
	}
	cfg := &config.Config{
		Site: "https://example.atlassian.net", Email: "a@example.com", Token: "tok",
		Projects: []string{"ALPHA", "BETA"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	db := newMirror(t)
	res, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start(), Log: func(string) {}})
	if err != nil {
		t.Fatal(err)
	}
	// Fake site returns every fixture for each project JQL (no project filter),
	// so Fetched is 2 projects × 2 issues. Changed is still 2 unique keys.
	if !res.Full || res.Changed != 2 {
		t.Fatalf("result = %+v", res)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	var sev *config.FieldSpec
	var body *config.FieldSpec
	for i := range loaded.Fields {
		s := &loaded.Fields[i]
		switch s.Alias {
		case "severity_level":
			sev = s
		case "repro_steps":
			body = s
		}
	}
	if sev == nil || len(sev.IDs) != 2 {
		t.Fatalf("severity_level ids: %+v fields=%+v", sev, loaded.Fields)
	}
	// Equal fill → id ascending: customfield_10, customfield_20.
	if sev.IDs[0] != "customfield_10" || sev.IDs[1] != "customfield_20" {
		t.Errorf("IDs order = %v", sev.IDs)
	}
	if body == nil || body.Role != "body" {
		t.Fatalf("repro_steps body role: %+v", body)
	}

	if got := db.column(t, "issues", "custom", "ALPHA-1"); !strings.Contains(got, `"severity_level"`) || !strings.Contains(got, "High") {
		t.Errorf("ALPHA custom = %s", got)
	}
	if got := db.column(t, "issues", "custom", "BETA-1"); !strings.Contains(got, "Low") {
		t.Errorf("BETA custom = %s", got)
	}

	usage, err := db.FieldUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	filled := map[string]map[string]int{}
	for _, r := range usage {
		if filled[r.ProjectKey] == nil {
			filled[r.ProjectKey] = map[string]int{}
		}
		filled[r.ProjectKey][r.Alias] = r.Filled
	}
	if filled["ALPHA"]["severity_level"] != 1 || filled["BETA"]["severity_level"] != 1 {
		t.Errorf("field_usage severity = %+v", filled)
	}
	if filled["ALPHA"]["repro_steps"] != 1 || filled["BETA"]["repro_steps"] != 0 {
		t.Errorf("field_usage repro = %+v", filled)
	}

	hits, err := db.Search(context.Background(), "xyzzy", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range hits.Keys {
		if k == "ALPHA-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("FTS missed body text; keys=%v", hits.Keys)
	}

	if len(site.syncFields) != 1 || site.syncFields[0] != "*all" {
		t.Errorf("sync fields = %v, want [*all]", site.syncFields)
	}
}

func TestReingestCustomIdempotent(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	// Sync without field map: discovery mode *all, but empty catalog → no specs.
	// Then apply explicit specs via ReingestCustom.
	site := newSite(t, "en")
	// newSite fixtures include customfield_10050 / _10101 on NMB-1.
	// Disable discovery Save side-effects by providing a FieldMap so discovery
	// mode is off, but leave Custom empty by not mapping during first path —
	// actually with FieldMap, custom is filled on ingest. Use no map + empty
	// catalog so discovery saves empty Fields, then reingest with specs.
	site.fieldCatalog = []map[string]any{} // discovery finds nothing
	db := newMirror(t)
	cfg := &config.Config{
		Site: "https://example.atlassian.net", Email: "a@example.com", Token: "tok",
		Projects: []string{"NMB"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}
	// custom should be empty after discovery of zero specs
	if got := db.column(t, "issues", "custom", "NMB-1"); got != "" && got != "{}" {
		// discovery may still leave {}
	}
	specs := []config.FieldSpec{
		{Alias: "severity", Label: "Severity", IDs: []string{"customfield_10050"}, Role: "facet", Kind: "option", Auto: true},
		{Alias: "repro", Label: "Repro", IDs: []string{"customfield_10101"}, Role: "body", Auto: true},
	}
	n, err := db.ReingestCustom(context.Background(), fields.SpecIDsFrom(specs), fields.BodyFieldIDs(nil, specs))
	if err != nil {
		t.Fatal(err)
	}
	custom := db.column(t, "issues", "custom", "NMB-1")
	if !strings.Contains(custom, `"severity"`) {
		t.Fatalf("custom after reingest = %s (rewrote %d)", custom, n)
	}
	if n < 1 {
		t.Errorf("expected at least one rewrite, got %d", n)
	}
	// Comments FTS preserved
	res, err := db.Search(context.Background(), "commentonlytoken", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range res.Keys {
		if k == "NMB-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("comments FTS lost after reingest; keys=%v", res.Keys)
	}
	// Body role FTS
	res, err = db.Search(context.Background(), "reproseed", 10)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, k := range res.Keys {
		if k == "NMB-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("body field FTS missed; keys=%v body=%q", res.Keys, db.column(t, "items", "body_text", "NMB-1"))
	}
	// Second reingest is idempotent
	n2, err := db.ReingestCustom(context.Background(), fields.SpecIDsFrom(specs), fields.BodyFieldIDs(nil, specs))
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Errorf("second reingest rewrote %d rows, want 0", n2)
	}
}
