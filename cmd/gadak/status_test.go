package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func TestStatusWarnsWhenTokenExpiring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	cfg := &config.Config{
		TokenExpiresAt:    time.Now().UTC().Add(5*24*time.Hour + time.Hour).Format(config.TokenTimeFormat),
		TokenExpirySource: config.TokenExpirySourceAssumed,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "API token expires in 5 days") {
		t.Fatalf("missing warning line:\n%s", out)
	}
	if !strings.Contains(out, "assumed from the default lifetime") {
		t.Fatalf("missing assumed hedge:\n%s", out)
	}
	if !strings.Contains(out, "gadak init") {
		t.Fatalf("missing remedy:\n%s", out)
	}
}

func TestStatusJSONIncludesTokenExpiry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	cfg := &config.Config{
		TokenExpiresAt:    time.Now().UTC().Add(5*24*time.Hour + time.Hour).Format(config.TokenTimeFormat),
		TokenExpirySource: config.TokenExpirySourceUser,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var doc struct {
		TokenExpiry config.TokenExpiry `json:"token_expiry"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.TokenExpiry.State != config.TokenExpiryExpiring {
		t.Fatalf("token_expiry %+v", doc.TokenExpiry)
	}
	if doc.TokenExpiry.Source != config.TokenExpirySourceUser {
		t.Fatalf("source %q", doc.TokenExpiry.Source)
	}
	if doc.TokenExpiry.Message == "" {
		t.Fatal("json message empty")
	}
}

func TestStatusSurfacesConfigLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := captureBoth(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status must still succeed when config is unreadable: %v", err)
	}
	if !strings.Contains(stderr, "gadak: config:") {
		t.Fatalf("stderr must name the config problem, got %q", stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout json %q: %v", stdout, err)
	}
	if _, ok := doc["issues"]; !ok {
		t.Fatalf("mirror stats missing: %s", stdout)
	}
	ce, _ := doc["config_error"].(string)
	if ce == "" {
		t.Fatalf("json missing config_error: %s", stdout)
	}
	if !strings.Contains(ce, "config.json") {
		t.Fatalf("config_error must name the path, got %q", ce)
	}
}

func TestStatusJSONWikiPathStandaloneOn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{
		Kind:       config.KindStandalone,
		Confluence: &config.ConfluenceConfig{Spaces: []string{"LOC"}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var doc struct {
		Pages int `json:"pages"`
		Wiki  struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"wiki"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Wiki.Path != "on" {
		t.Fatalf("wiki.path = %q, want on; body %s", doc.Wiki.Path, out)
	}
	if doc.Wiki.Reason != "" {
		t.Fatalf("wiki.reason = %q, want empty when on", doc.Wiki.Reason)
	}
	if strings.Contains(out, "token") && strings.Contains(strings.ToLower(out), "secret") {
		t.Fatalf("status leaked a credential-shaped field: %s", out)
	}
}

func TestStatusJSONWikiPathSkippedWhenNotConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{
		Kind: config.KindStandalone,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var doc struct {
		Wiki struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"wiki"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Wiki.Path != "skipped" {
		t.Fatalf("wiki.path = %q, want skipped; body %s", doc.Wiki.Path, out)
	}
	if doc.Wiki.Reason != "sync: confluence is not configured" {
		t.Fatalf("wiki.reason = %q", doc.Wiki.Reason)
	}
}

func TestStatusJSONWikiPathConnectedOn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{
		Site:       "https://example.invalid",
		Email:      "user@example.invalid",
		Token:      "status-test-token",
		Confluence: &config.ConfluenceConfig{Spaces: []string{"ENG"}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if strings.Contains(out, "status-test-token") {
		t.Fatalf("token leaked in status --json: %s", out)
	}
	var doc struct {
		Wiki struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"wiki"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Wiki.Path != "on" {
		t.Fatalf("wiki.path = %q, want on; body %s", doc.Wiki.Path, out)
	}
}

// plantStaleSyncSchema writes a behind-the-times sync_state.schema_version
// without touching PRAGMA user_version. GDK-526: the column is only updated
// when a migration actually runs, so a later Open can leave it lagging.
func plantStaleSyncSchema(t *testing.T, path string, stale int, lastErr string, version int64) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	res, err := raw.Exec(`UPDATE sync_state SET schema_version = ?, last_error = ?, version = ?`, stale, lastErr, version)
	if err != nil {
		t.Fatal(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("plantStaleSyncSchema: no sync_state row to update")
	}
	var got int
	if err := raw.QueryRow(`SELECT schema_version FROM sync_state LIMIT 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != stale {
		t.Fatalf("plant did not stick: schema_version=%d want %d", got, stale)
	}
}

// GDK-526: status --json must report PRAGMA user_version, not the lagging
// sync_state.schema_version column. Other freshness fields still come from
// the row. doctor --json must agree.
func TestStatusJSONSchemaVersionMatchesLivePRAGMA(t *testing.T) {
	mirror(t, "https://example.invalid")
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const watermark = "2026-01-15T12:00:00.000Z"
	if err := db.RecordSync(ctx, "jira", store.SyncResult{Watermark: watermark}); err != nil {
		t.Fatal(err)
	}
	live := db.SchemaVersion()
	issues, err := db.TableCount(ctx, "issues")
	if err != nil {
		t.Fatal(err)
	}
	issueComments, err := db.IssueCommentCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pageComments, err := db.PageCommentCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	stale := live - 5
	if stale < 1 {
		t.Fatalf("live schema %d is too low to plant a stale value", live)
	}
	plantStaleSyncSchema(t, path, stale, "planted last_error", 77)

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	var st struct {
		SchemaVersion int    `json:"schema_version"`
		Watermark     string `json:"watermark"`
		LastError     string `json:"last_error"`
		Version       int64  `json:"version"`
		Issues        int    `json:"issues"`
		IssueComments int    `json:"issue_comments"`
		PageComments  int    `json:"page_comments"`
		SyncCount     int64  `json:"sync_count"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	if st.SchemaVersion != live {
		t.Fatalf("status --json schema_version = %d, want live PRAGMA %d (row planted at %d)", st.SchemaVersion, live, stale)
	}
	if st.Watermark != watermark {
		t.Errorf("watermark = %q, want %q", st.Watermark, watermark)
	}
	if st.LastError != "planted last_error" {
		t.Errorf("last_error = %q, want planted last_error", st.LastError)
	}
	if st.Version != 77 {
		t.Errorf("version = %d, want 77", st.Version)
	}
	if st.Issues != issues {
		t.Errorf("issues = %d, want %d", st.Issues, issues)
	}
	if st.IssueComments != issueComments {
		t.Errorf("issue_comments = %d, want %d", st.IssueComments, issueComments)
	}
	if st.PageComments != pageComments {
		t.Errorf("page_comments = %d, want %d", st.PageComments, pageComments)
	}
	if st.SyncCount < 1 {
		t.Errorf("sync_count = %d, want >= 1 from RecordSync", st.SyncCount)
	}

	docOut, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, docOut)
	}
	var doc struct {
		SchemaVersion *int `json:"schema_version"`
	}
	if err := json.Unmarshal([]byte(docOut), &doc); err != nil {
		t.Fatalf("decode doctor %q: %v", docOut, err)
	}
	if doc.SchemaVersion == nil || *doc.SchemaVersion != st.SchemaVersion {
		got := 0
		if doc.SchemaVersion != nil {
			got = *doc.SchemaVersion
		}
		t.Fatalf("doctor --json schema_version = %d, status = %d (must agree)", got, st.SchemaVersion)
	}

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	var col int
	if err := raw.QueryRow(`SELECT schema_version FROM sync_state LIMIT 1`).Scan(&col); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()
	if col != stale {
		t.Fatalf("status rewrote sync_state.schema_version to %d, want planted %d", col, stale)
	}
}

func TestDoctorReportsMigratedSinceLastSync(t *testing.T) {
	mirror(t, "https://example.invalid")
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	live := db.SchemaVersion()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	stale := live - 5
	if stale < 1 {
		t.Fatalf("live schema %d is too low to plant a stale value", live)
	}
	plantStaleSyncSchema(t, path, stale, "planted last_error", 77)

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	if !strings.Contains(human, "migrated since last sync") {
		t.Fatalf("doctor must name the schema lag:\n%s", human)
	}
	if !strings.Contains(human, "sync_state has "+strconv.Itoa(stale)) {
		t.Fatalf("doctor must show the lagging row value %d:\n%s", stale, human)
	}

	docOut, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, docOut)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(docOut), &doc); err != nil {
		t.Fatalf("decode doctor %q: %v", docOut, err)
	}
	note, _ := doc["schema_since_sync"].(string)
	if !strings.Contains(note, "migrated since last sync") || !strings.Contains(note, strconv.Itoa(stale)) {
		t.Fatalf("doctor --json schema_since_sync = %q", note)
	}
}

// statusJSONBaselineKeys are the --json fields that must survive GDK-522.
// Optional keys (last_error, pairing, update, …) are omitted on purpose.
// GDK-628: the mixed "comments" key became issue_comments + page_comments.
var statusJSONBaselineKeys = []string{
	"profile", "workspace", "workspace_source", "kind",
	"issues", "issue_comments", "page_comments", "pages", "schema_version",
	"api_usage", "frozen", "token_expiry", "wiki",
}

// GDK-628: the comments table holds issue and page comments alike, so the
// old single "comments" row was a mixed figure under an issue-sounding label
// — a number that disagreed with the settings runtime (IssueCommentCount) on
// any mirror with wiki comments. status must split it by meaning, reusing
// the settings panel's owner for the issue share, and must not keep the
// mixed key as an alias.
func TestStatusSplitsIssueAndPageComments(t *testing.T) {
	mirror(t, "https://example.invalid")
	t.Setenv("HOME", t.TempDir())

	// One page with one comment on top of mirror()'s one issue comment: the
	// smallest fixture where the mixed row (2) and either split row differ.
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x.atlassian.net/wiki"}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(context.Background(), []store.PageRecord{{
		Item: store.Item{
			ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
			Key: "100", Title: "Billing notes", BodyText: "billing",
			CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Page: store.Page{SpaceKey: "PROD", Version: 1, Status: "current", BodyADF: adf},
		Comments: []store.Comment{{
			ID: "confluence:c1", ExternalID: "c1", Author: "Lee",
			BodyADF: adf, BodyText: "ok", CreatedAt: "2026-07-02T00:00:00.000Z",
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	js, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, js)
	}
	doc := decodeStatusJSON(t, js)
	if n, _ := doc["issue_comments"].(float64); int(n) != 1 {
		t.Fatalf("issue_comments = %v, want 1 (SUM(issues.comment_count)); body %s", doc["issue_comments"], js)
	}
	if n, _ := doc["page_comments"].(float64); int(n) != 1 {
		t.Fatalf("page_comments = %v, want 1 (comments on kind=page items); body %s", doc["page_comments"], js)
	}
	if _, ok := doc["comments"]; ok {
		t.Fatalf("status --json still emits the mixed comments key: %s", js)
	}

	out, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	for _, k := range []string{"issue_comments", "page_comments"} {
		if !strings.Contains(out, k) {
			t.Fatalf("text output missing the %s row:\n%s", k, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "comments") {
			t.Fatalf("text output still prints the mixed comments row:\n%s", out)
		}
	}
}

func TestStatusJSONCustomFieldsMapped(t *testing.T) {
	cfg := mirror(t, "https://example.invalid")
	t.Setenv("HOME", t.TempDir())
	cfg.Fields = []config.FieldSpec{
		{Alias: "story_points", Label: "Story Points", IDs: []string{"customfield_1"}, Role: "plain"},
		{Alias: "severity", Label: "Severity", IDs: []string{"customfield_2"}, Role: "facet"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	doc := decodeStatusJSON(t, out)
	cf := statusCustomFields(t, doc)
	mapped, _ := cf["mapped"].(float64)
	if int(mapped) != 2 {
		t.Fatalf("custom_fields.mapped = %v, want 2; body %s", cf["mapped"], out)
	}
	if _, ok := cf["applied_at"]; ok {
		t.Fatalf("applied_at must be omitted when FieldsAppliedAt is empty: %s", out)
	}
	for _, k := range statusJSONBaselineKeys {
		if _, ok := doc[k]; !ok {
			t.Errorf("status --json lost %q", k)
		}
	}
}

func TestStatusJSONCustomFieldsAppliedAt(t *testing.T) {
	cfg := mirror(t, "https://example.invalid")
	t.Setenv("HOME", t.TempDir())
	const applied = "2026-08-21T12:00:00.000Z"
	cfg.Fields = []config.FieldSpec{
		{Alias: "story_points", Label: "Story Points", IDs: []string{"customfield_1"}, Role: "plain"},
	}
	cfg.FieldsAppliedAt = applied
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	cf := statusCustomFields(t, decodeStatusJSON(t, out))
	if got, _ := cf["applied_at"].(string); got != applied {
		t.Fatalf("custom_fields.applied_at = %q, want %q; body %s", got, applied, out)
	}
	mapped, _ := cf["mapped"].(float64)
	if int(mapped) != 1 {
		t.Fatalf("custom_fields.mapped = %v, want 1", cf["mapped"])
	}
}

func TestStatusJSONCustomFieldsUnmappedNoStderrNudge(t *testing.T) {
	// HasCustomFieldKeysInRaw parses every issues.raw document. status must
	// stay cheap, so the unmapped-raw hint lives on doctor only (GDK-522).
	cfg := mirror(t, "https://example.invalid")
	t.Setenv("HOME", t.TempDir())
	if len(cfg.Fields) != 0 {
		t.Fatalf("fixture Fields = %v, want empty", cfg.Fields)
	}

	_, stderr, err := captureBoth(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if strings.Contains(stderr, "fields --apply") {
		t.Fatalf("status must not stderr-nudge fields --apply (raw scan is not cheap): %q", stderr)
	}

	standaloneMirror(t)
	t.Setenv("HOME", t.TempDir())
	_, stderr, err = captureBoth(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("standalone status --json: %v", err)
	}
	if strings.Contains(stderr, "fields --apply") {
		t.Fatalf("standalone status must not nudge custom-field mapping: %q", stderr)
	}
}

func TestStatusReportsProjectScopeMismatch(t *testing.T) {
	// GDK-809: config listed DI, mirror has D1. status must name both
	// sides so "sync is stuck" is not the diagnosis.
	cfg := mirror(t, "https://example.invalid")
	cfg.Projects = []string{"DI"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "configured, not in the mirror") || !strings.Contains(out, "DI") {
		t.Fatalf("missing configured-not-mirrored line:\n%s", out)
	}
	if !strings.Contains(out, "in the mirror, not configured") || !strings.Contains(out, "NMB") {
		t.Fatalf("missing mirrored-not-configured line:\n%s", out)
	}
	js, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	doc := decodeStatusJSON(t, js)
	missing, _ := doc["projects_configured_not_in_mirror"].([]any)
	found := false
	for _, v := range missing {
		if v == "DI" {
			found = true
		}
	}
	if !found {
		t.Fatalf("json missing DI in projects_configured_not_in_mirror: %s", js)
	}
}

func decodeStatusJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	return doc
}

func statusCustomFields(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	cf, ok := doc["custom_fields"].(map[string]any)
	if !ok {
		t.Fatalf("custom_fields missing or not an object: %v", doc["custom_fields"])
	}
	return cf
}
