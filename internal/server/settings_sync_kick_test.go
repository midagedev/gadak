package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
	gadakSync "github.com/midagedev/gadak/internal/sync"
)

// putSettingsJSON issues PUT settings/ with the given body map.
func putSettingsJSON(t *testing.T, h http.Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(b))))
	return rec
}

// TestPutSettingsScopeChangeKicksFullSync: changing projects (or confluence
// scope) with a credential must ask for a full resync.
func TestPutSettingsScopeChangeKicksFullSync(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	// Seed disk so Load/Save round-trips stay isolated.
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := New(db, cfg)

	var mu sync.Mutex
	var kicks []bool
	h.s.syncKick = func(_ *config.Config, full bool) bool {
		mu.Lock()
		defer mu.Unlock()
		kicks = append(kicks, full)
		return true
	}

	rec := putSettingsJSON(t, h, map[string]any{
		"projects":            []string{"NMB", "NMA", "OPS"},
		"staleThresholdHours": 72,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT → %d %s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(kicks) != 1 || !kicks[0] {
		t.Fatalf("syncKick calls full=%v, want one full=true", kicks)
	}
}

// TestPutSettingsNonScopeNoKick: interval / stale-threshold alone must not resync.
func TestPutSettingsNonScopeNoKick(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
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
		"projects":             cfg.Projects,
		"staleThresholdHours":  48,
		"syncIntervalSec":      30,
		"reconcileIntervalSec": 3600,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT → %d %s", rec.Code, rec.Body.String())
	}
	if kicks != 0 {
		t.Fatalf("syncKick called %d times, want 0", kicks)
	}
}

// TestPutSettingsScopeChangeNoCredentialNoKick: without a token, scope edits
// must not start a sync (nothing to authenticate with).
func TestPutSettingsScopeChangeNoCredentialNoKick(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	cfg.Email, cfg.Token = "", ""
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
		"projects":            []string{"ONLY"},
		"staleThresholdHours": 72,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT → %d %s", rec.Code, rec.Body.String())
	}
	if kicks != 0 {
		t.Fatalf("syncKick called %d times without credential, want 0", kicks)
	}
}

// TestSyncRunsSourceFilter: ?source= selects jira vs confluence history;
// missing source stays jira (compat); garbage is 400.
func TestSyncRunsSourceFilter(t *testing.T) {
	db, cfg := fixture(t)
	// Seed one run per source.
	if err := db.AppendSyncRun(context.Background(), sourceID, store.SyncRun{
		Kind: "full", StartedAt: "2026-08-01T00:00:00Z", FinishedAt: "2026-08-01T00:01:00Z",
		Fetched: 3, Changed: 3,
	}); err != nil {
		t.Fatalf("append jira: %v", err)
	}
	if err := db.AppendSyncRun(context.Background(), gadakSync.ConfluenceSourceID, store.SyncRun{
		Kind: "full", StartedAt: "2026-08-01T00:02:00Z", FinishedAt: "2026-08-01T00:03:00Z",
		Fetched: 9, Changed: 9,
	}); err != nil {
		t.Fatalf("append confluence: %v", err)
	}
	h := New(db, cfg)

	type runsDoc struct {
		Runs   []store.SyncRun `json:"runs"`
		Source string          `json:"source"`
	}

	// Default (no query) → jira, with source echo.
	def := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/", nil))
	if def.Source != "jira" {
		t.Fatalf("default source %q, want jira", def.Source)
	}
	if len(def.Runs) != 1 || def.Runs[0].Fetched != 3 {
		t.Fatalf("default runs %+v", def.Runs)
	}

	// Explicit jira same as default.
	jira := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/?source=jira", nil))
	if jira.Source != "jira" || len(jira.Runs) != 1 || jira.Runs[0].Fetched != 3 {
		t.Fatalf("jira query %+v", jira)
	}

	// Confluence.
	conf := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/?source=confluence", nil))
	if conf.Source != "confluence" {
		t.Fatalf("confluence source %q", conf.Source)
	}
	if len(conf.Runs) != 1 || conf.Runs[0].Fetched != 9 {
		t.Fatalf("confluence runs %+v", conf.Runs)
	}

	// Invalid.
	bad := get(t, h, apiBase+"sync/runs/?source=nope", nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid source status %d, want 400; body %s", bad.Code, bad.Body.String())
	}
	if !strings.Contains(bad.Body.String(), "invalid_source") {
		t.Fatalf("body %s", bad.Body.String())
	}
}

// TestSyncRunsLastCheckedAt: GET sync/runs/ carries sources.synced_at (the
// same origin sync_health reads) as last_checked_at, without inventing a
// no-op row. GDK-486 — a quiet tick refreshes the chip and this field together.
func TestSyncRunsLastCheckedAt(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	type runsDoc struct {
		Runs          []store.SyncRun `json:"runs"`
		Source        string          `json:"source"`
		LastCheckedAt *string         `json:"last_checked_at"`
	}

	empty := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/", nil))
	if empty.LastCheckedAt != nil {
		t.Fatalf("never-synced last_checked_at = %q, want omitted", *empty.LastCheckedAt)
	}
	if len(empty.Runs) != 0 {
		t.Fatalf("runs = %+v, want empty", empty.Runs)
	}

	// No-op tick: RecordSync stamps sources.synced_at and does not append a run.
	if err := db.RecordSync(context.Background(), sourceID, store.SyncResult{Watermark: "2026-08-04T00:00:00.000Z"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	boot := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if len(boot.SyncHealth.Sources) == 0 || boot.SyncHealth.Sources[0].SyncedAt == nil {
		t.Fatalf("bootstrap synced_at missing after RecordSync: %+v", boot.SyncHealth)
	}
	want := *boot.SyncHealth.Sources[0].SyncedAt

	got := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/", nil))
	if got.LastCheckedAt == nil {
		t.Fatal("last_checked_at omitted after RecordSync")
	}
	if *got.LastCheckedAt != want {
		t.Fatalf("last_checked_at %q != health synced_at %q", *got.LastCheckedAt, want)
	}
	if len(got.Runs) != 0 {
		t.Fatalf("no-op tick must not invent a run: %+v", got.Runs)
	}

	// A recorded run's finished_at is a different fact. last_checked_at stays
	// the health origin, not the newest run timestamp.
	runAt := "2020-01-01T00:00:00Z"
	if err := db.AppendSyncRun(context.Background(), sourceID, store.SyncRun{
		Kind: "incremental", StartedAt: runAt, FinishedAt: runAt,
		Fetched: 1, Changed: 1,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	withRun := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/", nil))
	if withRun.LastCheckedAt == nil || *withRun.LastCheckedAt != want {
		t.Fatalf("last_checked_at after a run %+v, want %q", withRun.LastCheckedAt, want)
	}
	if *withRun.LastCheckedAt == runAt {
		t.Fatal("last_checked_at must not be the run timestamp")
	}
	if len(withRun.Runs) != 1 {
		t.Fatalf("runs %+v, want the one we appended", withRun.Runs)
	}

	// Confluence is the same field, scoped to ?source=.
	if err := db.UpsertSource(context.Background(), store.Source{
		ID: gadakSync.ConfluenceSourceID, Kind: "confluence", BaseURL: "https://x.atlassian.net/wiki",
	}); err != nil {
		t.Fatalf("confluence source: %v", err)
	}
	if err := db.RecordSync(context.Background(), gadakSync.ConfluenceSourceID, store.SyncResult{Watermark: "2026-08-05T00:00:00.000Z"}); err != nil {
		t.Fatalf("confluence record: %v", err)
	}
	boot2 := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	var confAt *string
	for i := range boot2.SyncHealth.Sources {
		if boot2.SyncHealth.Sources[i].Key == "confluence" {
			confAt = boot2.SyncHealth.Sources[i].SyncedAt
		}
	}
	if confAt == nil {
		t.Fatal("confluence health synced_at missing")
	}
	conf := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/?source=confluence", nil))
	if conf.LastCheckedAt == nil || *conf.LastCheckedAt != *confAt {
		t.Fatalf("confluence last_checked_at %+v vs health %q", conf.LastCheckedAt, *confAt)
	}
	jira := decode[runsDoc](t, get(t, h, apiBase+"sync/runs/?source=jira", nil))
	if jira.LastCheckedAt == nil || *jira.LastCheckedAt != want {
		t.Fatalf("jira last_checked_at %+v, want %q", jira.LastCheckedAt, want)
	}
}
