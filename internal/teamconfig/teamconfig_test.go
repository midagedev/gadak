package teamconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// TestExportWhitelistCoversAllConfigFields forces every new Config field to be
// classified as either team-shareable (exportableConfigFields) or deliberately
// private (neverExportConfigFields).
//
// Why reflection + union equality, not a blacklist test:
// If we only asserted "Site/Email/Token must not appear", a future field such as
// "PersonalWebhookURL" or another token would export automatically until someone
// noticed. Whitelist + this coverage test means adding a field to Config fails
// CI until a developer answers "is this team consensus or personal?" once.
func TestExportWhitelistCoversAllConfigFields(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(config.Config{})
	all := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		all[f.Name] = true
	}

	exportSet := make(map[string]bool)
	for _, n := range exportableConfigFields {
		if exportSet[n] {
			t.Errorf("exportableConfigFields: duplicate %q", n)
		}
		exportSet[n] = true
		if !all[n] {
			t.Errorf("exportableConfigFields has %q which is not a config.Config field", n)
		}
	}
	neverSet := make(map[string]bool)
	for _, n := range neverExportConfigFields {
		if neverSet[n] {
			t.Errorf("neverExportConfigFields: duplicate %q", n)
		}
		neverSet[n] = true
		if !all[n] {
			t.Errorf("neverExportConfigFields has %q which is not a config.Config field", n)
		}
		if exportSet[n] {
			t.Errorf("%q is in both exportable and never-export lists", n)
		}
	}
	for name := range all {
		if !exportSet[name] && !neverSet[name] {
			t.Errorf("config.Config field %q is not classified: add it to exportableConfigFields (team-shareable) or neverExportConfigFields (private / per-machine)", name)
		}
	}
}

func sampleConfig() *config.Config {
	return &config.Config{
		Site:            "https://evil.example.atlassian.net",
		Email:           "secret-user@example.com",
		Token:           "ATATT" + strings.Repeat("x", 30),
		AccountID:       "acc-secret-99",
		TokenOwner:      "Secret Owner",
		TokenVerifiedAt: "2026-01-01T00:00:00Z",
		Projects:        []string{"NMB", "NMA"},
		FieldMap: map[string]string{
			"storyPoints": "customfield_10016",
		},
		BodyFields:     []string{"customfield_10001"},
		EditableFields: map[string]string{"storyPoints": "customfield_10016"},
		Members: []config.Member{
			{Email: "dana@example.com", Name: "Dana"},
		},
		GroupRules: []config.GroupRule{
			{Group: "platform", Projects: []string{"NMB"}},
		},
		GroupLabels: map[string]string{"platform": "Platform"},
		GroupColors: map[string]string{"platform": "#336699"},
		ProductByGroup: map[string]config.Product{
			"platform": {Key: "plat", Label: "Platform"},
		},
		Features:            map[string]bool{"feed": true},
		QaDashboardURL:      "https://qa.example.com",
		StaleThresholdHours: 48,
		// Must never export:
		SyncIntervalSec:      30,
		ReconcileIntervalSec: 600,
		AttachmentCacheMB:    128,
		Notify:               boolPtr(false),
	}
}

func boolPtr(b bool) *bool { return &b }

func TestExportOmitsCredentialsAsStrings(t *testing.T) {
	t.Parallel()
	cfg := sampleConfig()
	doc := BuildDocument(cfg, nil, ExportOptions{
		Now: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
	})
	raw, err := MarshalDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, forbidden := range []string{
		cfg.Site,
		cfg.Email,
		cfg.Token,
		cfg.AccountID,
		cfg.TokenOwner,
		cfg.TokenVerifiedAt,
		`"site"`,
		`"email"`,
		`"token"`,
		`"account_id"`,
		`"tokenOwner"`,
		`"tokenVerifiedAt"`,
		`"syncIntervalSec"`,
		`"reconcileIntervalSec"`,
		`"notify"`,
		`"attachmentCacheMB"`,
	} {
		if strings.Contains(s, forbidden) {
			t.Errorf("export JSON must not contain %q\n%s", forbidden, s)
		}
	}
	// Team material is present.
	for _, want := range []string{`"fieldMap"`, `"groupRules"`, `"projects"`, "NMB", "customfield_10016"} {
		if !strings.Contains(s, want) {
			t.Errorf("export missing %q\n%s", want, s)
		}
	}
}

func TestMembersDefaultExcluded(t *testing.T) {
	t.Parallel()
	cfg := sampleConfig()
	// Default export: no members / no emails.
	raw, err := MarshalDocument(BuildDocument(cfg, nil, ExportOptions{
		Now: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "dana@example.com") {
		t.Errorf("default export must not include member emails:\n%s", raw)
	}
	if strings.Contains(string(raw), `"members"`) {
		t.Errorf("default export must not include members key:\n%s", raw)
	}

	// WithMembers path includes emails (credential scan must not block emails).
	raw2, err := MarshalDocument(BuildDocument(cfg, nil, ExportOptions{
		WithMembers: true,
		Now:         time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw2), "dana@example.com") {
		t.Errorf("--with-members export should include emails:\n%s", raw2)
	}
}

func TestMergeRules(t *testing.T) {
	t.Parallel()
	existing := &config.Config{
		FieldMap: map[string]string{"old": "customfield_1"},
		Projects: []string{"KEEP"},
	}
	incoming := Document{
		Version: CurrentFormat,
		Settings: TeamSettings{
			FieldMap:   map[string]string{"new": "customfield_2"},
			Projects:   []string{"IN"},
			GroupRules: []config.GroupRule{{Group: "g", Projects: []string{"X"}}},
		},
		Views: []ExportView{
			{Name: "Mine", Config: json.RawMessage(`{"a":1}`)},
			{Name: "New view", Config: json.RawMessage(`{"b":2}`)},
		},
	}
	existingViews := []store.SavedView{
		{ID: "id1", Name: "Mine", Config: json.RawMessage(`{"a":0}`)},
	}

	// Default: skip non-empty settings and same-named views; add missing.
	plan := BuildPlan(existing, existingViews, incoming, ImportOptions{})
	assertSettingAction(t, plan, "fieldMap", SettingSkip)
	assertSettingAction(t, plan, "projects", SettingSkip)
	assertSettingAction(t, plan, "groupRules", SettingAdd)
	assertViewAction(t, plan, "Mine", ViewSkip)
	assertViewAction(t, plan, "New view", ViewAdd)

	// Overwrite: replace conflicts.
	planO := BuildPlan(existing, existingViews, incoming, ImportOptions{Overwrite: true})
	assertSettingAction(t, planO, "fieldMap", SettingReplace)
	assertSettingAction(t, planO, "projects", SettingReplace)
	assertViewAction(t, planO, "Mine", ViewReplace)
	assertViewAction(t, planO, "New view", ViewAdd)

	// Apply default merge and ensure existing fieldMap untouched, groupRules filled.
	db := openTestDB(t)
	// DB starts with Mine so skip/add behaviour matches the plan inputs.
	if err := db.PutSavedView(store.SavedView{ID: "id1", Name: "Mine", Config: json.RawMessage(`{"a":0}`)}); err != nil {
		t.Fatal(err)
	}
	cfg := *existing
	cfg.FieldMap = copyStringMap(existing.FieldMap)
	cfg.Projects = copyStrings(existing.Projects)
	if err := ApplyPlan(&cfg, db, plan); err != nil {
		t.Fatal(err)
	}
	if cfg.FieldMap["old"] != "customfield_1" || len(cfg.FieldMap) != 1 {
		t.Errorf("fieldMap should be unchanged, got %v", cfg.FieldMap)
	}
	if len(cfg.GroupRules) != 1 || cfg.GroupRules[0].Group != "g" {
		t.Errorf("groupRules should be added, got %v", cfg.GroupRules)
	}
	views, err := db.SavedViews()
	if err != nil {
		t.Fatal(err)
	}
	// Mine skipped (still original config), New view added once.
	if len(views) != 2 {
		t.Fatalf("views after default merge: %+v", views)
	}
	byName := map[string]store.SavedView{}
	for _, v := range views {
		byName[v.Name] = v
	}
	if string(byName["Mine"].Config) != `{"a":0}` {
		t.Errorf("Mine should be unchanged without overwrite, got %s", byName["Mine"].Config)
	}
	if byName["New view"].Name == "" {
		t.Fatal("New view missing")
	}

	// Overwrite plan against the same DB: Mine replaced, New view skipped (already exists).
	cfg2 := *existing
	cfg2.FieldMap = map[string]string{"old": "customfield_1"}
	viewsNow, _ := db.SavedViews()
	planO2 := BuildPlan(&cfg2, viewsNow, incoming, ImportOptions{Overwrite: true})
	assertViewAction(t, planO2, "Mine", ViewReplace)
	assertViewAction(t, planO2, "New view", ViewReplace) // exists now → replace under overwrite
	if err := ApplyPlan(&cfg2, db, planO2); err != nil {
		t.Fatal(err)
	}
	if cfg2.FieldMap["new"] != "customfield_2" {
		t.Errorf("overwrite should replace fieldMap, got %v", cfg2.FieldMap)
	}
	views, _ = db.SavedViews()
	var mine store.SavedView
	for _, v := range views {
		if v.Name == "Mine" {
			mine = v
		}
	}
	if mine.ID != "id1" {
		t.Errorf("overwrite must keep id, got %q", mine.ID)
	}
	if !jsonEqual(mine.Config, []byte(`{"a":1}`)) {
		t.Errorf("overwrite view config: got %s", mine.Config)
	}
	names := map[string]int{}
	for _, v := range views {
		names[v.Name]++
	}
	if names["Mine"] != 1 || names["New view"] != 1 {
		t.Errorf("duplicate views: %v", names)
	}
}

func assertSettingAction(t *testing.T, p Plan, key string, want SettingAction) {
	t.Helper()
	for _, s := range p.Settings {
		if s.Key == key {
			if s.Action != want {
				t.Errorf("settings %s: want %s, got %s", key, want, s.Action)
			}
			return
		}
	}
	t.Errorf("settings %s: missing from plan", key)
}

func assertViewAction(t *testing.T, p Plan, name string, want ViewAction) {
	t.Helper()
	for _, v := range p.Views {
		if v.Name == name {
			if v.Action != want {
				t.Errorf("view %s: want %s, got %s", name, want, v.Action)
			}
			return
		}
	}
	t.Errorf("view %s: missing from plan", name)
}

func TestDryRunNoChanges(t *testing.T) {
	// Integration-style: write config file + views, build plan, do not Apply when dry-run.
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	config.SetProfile("")

	cfg := &config.Config{
		Site:     "https://keep.example.atlassian.net",
		Email:    "keep@example.com",
		Token:    "keep-token-not-atlassian-shape",
		Projects: []string{"OLD"},
		FieldMap: map[string]string{"x": "customfield_1"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(home, "scry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PutSavedView(store.SavedView{
		ID: "v1", Name: "Existing", Config: json.RawMessage(`{"k":1}`),
	}); err != nil {
		t.Fatal(err)
	}

	incoming, err := MarshalDocument(BuildDocument(sampleConfig(), []store.SavedView{
		{Name: "Other", Config: json.RawMessage(`{}`)},
	}, ExportOptions{Now: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)}))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ParseDocument(incoming)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	views, err := db.SavedViews()
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildPlan(loaded, views, doc, ImportOptions{DryRun: true})
	// Dry-run path: never call ApplyPlan or Save.
	_ = plan

	after, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Site != cfg.Site || after.Email != cfg.Email || after.Token != cfg.Token {
		t.Fatal("dry-run must not touch credentials")
	}
	if after.FieldMap["x"] != "customfield_1" || len(after.Projects) != 1 || after.Projects[0] != "OLD" {
		t.Fatalf("dry-run changed settings: %+v", after)
	}
	viewsAfter, _ := db.SavedViews()
	if len(viewsAfter) != 1 || viewsAfter[0].Name != "Existing" {
		t.Fatalf("dry-run changed views: %+v", viewsAfter)
	}
	// Confirm plan would have done something if applied.
	if len(plan.Settings) == 0 && len(plan.Views) == 0 {
		t.Fatal("expected non-empty plan for dry-run smoke")
	}
}

func TestImportRejectsCredentialKeys(t *testing.T) {
	t.Parallel()
	cases := []string{
		`{"scry_team_config":1,"settings":{"site":"https://x.atlassian.net","fieldMap":{}}}`,
		`{"scry_team_config":1,"settings":{"token":"ATATT` + strings.Repeat("a", 30) + `"}}`,
		`{"scry_team_config":1,"email":"x@y.com","settings":{}}`,
		`{"scry_team_config":1,"settings":{"account_id":"acc1"}}`,
	}
	for _, raw := range cases {
		_, err := ParseDocument([]byte(raw))
		if err == nil {
			t.Errorf("expected rejection for %s", raw)
			continue
		}
		if !strings.Contains(err.Error(), "credentials") && !strings.Contains(err.Error(), "personal identity") {
			t.Errorf("error should mention credentials, got: %v", err)
		}
	}
}

func TestRoundTripExportImport(t *testing.T) {
	// Export from profile A → import into empty profile B.
	base := t.TempDir()
	homeA := filepath.Join(base, "a")
	homeB := filepath.Join(base, "b")
	if err := os.MkdirAll(homeA, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeB, 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SCRY_HOME", homeA)
	config.SetProfile("")
	src := sampleConfig()
	// Use a non-ATATT token so sampleConfig token never appears if something leaks
	// via members path; sampleConfig already has ATATT but export omits it.
	if err := src.Save(); err != nil {
		t.Fatal(err)
	}
	dbA, err := store.Open(filepath.Join(homeA, "scry.db"))
	if err != nil {
		t.Fatal(err)
	}
	viewsIn := []store.SavedView{
		{ID: "va", Name: "Reopened", Config: json.RawMessage(`{"statusCategories":["done"]}`)},
		{ID: "vb", Name: "Mine", Config: json.RawMessage(`{"assignee":"me"}`)},
	}
	for _, v := range viewsIn {
		if err := dbA.PutSavedView(v); err != nil {
			t.Fatal(err)
		}
	}
	storedA, _ := dbA.SavedViews()
	doc := BuildDocument(src, storedA, ExportOptions{
		Now: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
	})
	raw, err := MarshalDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	dbA.Close()

	// Import into B (empty team settings, has its own credentials).
	t.Setenv("SCRY_HOME", homeB)
	config.SetProfile("")
	dst := &config.Config{
		Site:  "https://other.example.atlassian.net",
		Email: "other@example.com",
		Token: "other-token-plain",
	}
	if err := dst.Save(); err != nil {
		t.Fatal(err)
	}
	dbB, err := store.Open(filepath.Join(homeB, "scry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()

	parsed, err := ParseDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	existing, _ := dbB.SavedViews()
	plan := BuildPlan(loaded, existing, parsed, ImportOptions{})
	if err := ApplyPlan(loaded, dbB, plan); err != nil {
		t.Fatal(err)
	}
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}

	// Credentials preserved.
	if loaded.Site != dst.Site || loaded.Email != dst.Email || loaded.Token != dst.Token {
		t.Fatalf("credentials not preserved: site=%q email=%q token=%q", loaded.Site, loaded.Email, loaded.Token)
	}
	// Team settings match source whitelist.
	if !reflect.DeepEqual(loaded.Projects, src.Projects) {
		t.Errorf("projects: got %v want %v", loaded.Projects, src.Projects)
	}
	if !reflect.DeepEqual(loaded.FieldMap, src.FieldMap) {
		t.Errorf("fieldMap: got %v want %v", loaded.FieldMap, src.FieldMap)
	}
	if !reflect.DeepEqual(loaded.GroupRules, src.GroupRules) {
		t.Errorf("groupRules: got %v want %v", loaded.GroupRules, src.GroupRules)
	}
	if loaded.QaDashboardURL != src.QaDashboardURL {
		t.Errorf("qaDashboardUrl: got %q", loaded.QaDashboardURL)
	}
	if loaded.StaleThresholdHours != src.StaleThresholdHours {
		t.Errorf("staleThresholdHours: got %d", loaded.StaleThresholdHours)
	}
	// Members not in default export → not imported.
	if len(loaded.Members) != 0 {
		t.Errorf("members should be empty without --with-members, got %v", loaded.Members)
	}

	viewsB, err := dbB.SavedViews()
	if err != nil {
		t.Fatal(err)
	}
	if len(viewsB) != 2 {
		t.Fatalf("want 2 views, got %d", len(viewsB))
	}
	byName := map[string]store.SavedView{}
	for _, v := range viewsB {
		byName[v.Name] = v
		if v.ID == "va" || v.ID == "vb" {
			t.Errorf("imported view should get a new id, got %q for %q", v.ID, v.Name)
		}
	}
	if !jsonEqual(byName["Reopened"].Config, []byte(`{"statusCategories":["done"]}`)) {
		t.Errorf("Reopened config: %s", byName["Reopened"].Config)
	}
	if !jsonEqual(byName["Mine"].Config, []byte(`{"assignee":"me"}`)) {
		t.Errorf("Mine config: %s", byName["Mine"].Config)
	}
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func TestScanBlocksTokenInFieldMap(t *testing.T) {
	t.Parallel()
	doc := BuildDocument(&config.Config{
		FieldMap: map[string]string{
			"x": "ATATT" + strings.Repeat("Z", 30),
		},
	}, nil, ExportOptions{Now: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)})
	_, err := MarshalDocument(doc)
	if err == nil {
		t.Fatal("expected credential scan to refuse export")
	}
	if !strings.Contains(err.Error(), "credential-shaped") {
		t.Errorf("got: %v", err)
	}
}

func TestUnsupportedVersion(t *testing.T) {
	t.Parallel()
	_, err := ParseDocument([]byte(`{"scry_team_config":99,"settings":{},"views":[]}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("want unsupported version error, got %v", err)
	}
}

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
