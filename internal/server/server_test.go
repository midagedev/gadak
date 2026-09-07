package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/selfupdate"
	"github.com/midagedev/gadak/internal/store"
)

// liveHandlers lets fixtureAt stop every Handler created against a DB before
// that DB is closed. Production never sets testRegisterHandler.
var (
	liveMu       sync.Mutex
	liveHandlers = map[*store.DB][]*Handler{}
)

func init() {
	testRegisterHandler = registerLive
}

func registerLive(h *Handler) {
	if h == nil || h.s == nil || h.s.db == nil {
		return
	}
	liveMu.Lock()
	liveHandlers[h.s.db] = append(liveHandlers[h.s.db], h)
	liveMu.Unlock()
}

func shutdownLive(t *testing.T, db *store.DB) {
	t.Helper()
	if db == nil {
		return
	}
	liveMu.Lock()
	hs := liveHandlers[db]
	delete(liveHandlers, db)
	liveMu.Unlock()
	for _, h := range hs {
		if err := h.Close(); err != nil {
			t.Errorf("Handler.Close: %v — a startSyncJob goroutine did not return within the shutdown bound (GDK-270)", err)
		}
	}
}

// testRequest is httptest.NewRequest with Host set for the loopback API guard.
// httptest defaults Host to "example.com", which browserGuard correctly
// rejects as a DNS-rebinding name; real clients send localhost or an IP.
func testRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Host = "127.0.0.1"
	return req
}

// quiesceFixtureDir empties the mirror directory, then names anything that
// comes back.
//
// The directory is a t.TempDir(), and Go removes it in a cleanup registered
// before ours — so it runs *after* ours (LIFO). Its RemoveAll lists the
// directory and then rmdirs it, and a file created in between makes the whole
// package fail with `TempDir RemoveAll cleanup: unlinkat …/002: directory not
// empty`, attributed to whichever test happened to own that directory. That is
// the least useful error shape available: it names no file and no writer, and on
// 2026-08-18 it landed on a pull request whose entire diff was one markdown file
// (GDK-270).
//
// mirror.db and its local.db sibling (ATTACHed on every connection by
// internal/store) are supposed to be there — closing a database does not delete
// it. Removing them here is not cleanup for its own sake: it leaves RemoveAll
// nothing to race on, and it turns "some file was in the way" into "this named
// file appeared after teardown began", reported by the fixture that owns the
// path instead of anonymously by the framework.
func quiesceFixtureDir(t *testing.T, dir string, db *store.DB) {
	t.Helper()
	if db != nil {
		stats := db.PoolStats()
		if stats.InUse > 0 {
			// A connection still checked out means something the test started
			// is still running. Close cannot reclaim it, and under WAL that
			// writer recreates journal files in this TempDir (GDK-270).
			t.Errorf("%d connection(s) still checked out after Close — something the test started is still running (GDK-270); stats=%+v", stats.InUse, stats)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Already gone, or unreadable: not this helper's business.
		return
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	back, err := os.ReadDir(dir)
	if err != nil || len(back) == 0 {
		return
	}
	names := make([]string, 0, len(back))
	for _, e := range back {
		names = append(names, e.Name())
	}
	t.Errorf("mirror dir got %v back after teardown removed everything — something in this package is still writing to a torn-down TempDir (GDK-270)", names)
}

// fixture builds a three-issue mirror and the configuration that turns the
// company-specific surfaces (member directory, group rules) back on.
func fixture(t *testing.T) (*store.DB, *config.Config) {
	t.Helper()
	db, cfg, _ := fixtureAt(t)
	return db, cfg
}

// fixtureAt also hands back the database path, which is how a plugin reaches the
// mirror: with SQL, from outside this process.
func fixtureAt(t *testing.T) (*store.DB, *config.Config, string) {
	t.Helper()
	// This fixture's Site is 127.0.0.1:1 (connection refused immediately).
	// Production jira.New retries 5 times with a 1s backoff — 15s of sleep
	// per refused read. Tests shrink the budget so a stray origin call does
	// not dominate the suite (GDK-608).
	shrinkJiraRetryBudget(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "mirror.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		// Stop writers first: a startSyncJob goroutine that outlives Close
		// holds a pool connection and recreates WAL files in dir (GDK-270).
		shutdownLive(t, db)
		db.Close()
		quiesceFixtureDir(t, dir, db)
	})
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Priorities: []string{"Highest", "High", "Medium"},
		Records: []store.IssueRecord{
			{
				Item: store.Item{
					ID: "jira:1001", SourceID: "jira", ExternalID: "1001", Key: "NMB-1",
					Title: "batch worker drops the last page", BodyText: "seen on staging",
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
					Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
					Priority: "High", Assignee: "김현철", AssigneeID: "acc-hc",
					AssigneeEmail: "hc@example.com",
					Reporter:      "박보고", ReporterID: "acc-rp", ReporterEmail: "rp@example.com",
					Labels: []string{"batch"}, Components: []string{"api"},
					DescriptionADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
					// Aliases from the configured field map; the response spreads them.
					Custom: map[string]any{"severity": "S2", "solution": "Fixed"},
				},
				Comments: []store.Comment{{
					ID: "jira:c-1", ExternalID: "c-1", Author: "김현철", AuthorID: "acc-hc",
					BodyADF:  json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
					BodyText: "hydra parity check failed", CreatedAt: "2026-07-02T00:00:00.000Z",
				}},
				Attachments: []store.Attachment{{
					ID: "jira:a-1", ExternalID: "10021", Filename: "trace.png",
					MimeType: "image/png", Size: 1234, CreatedAt: "2026-07-02T00:00:00.000Z",
				}},
				Changelog: []store.ChangeEntry{{
					ID: "jira:h-1", At: "2026-07-03T00:00:00.000Z", Author: "김현철",
					Field: "status", FromValue: "할 일", FromID: "1", ToValue: "진행 중", ToID: "3",
				}},
				Links: []store.Link{{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"}},
			},
			{
				Item: store.Item{
					ID: "jira:1002", SourceID: "jira", ExternalID: "1002", Key: "NMB-2",
					Title:     "cloud upload retries forever",
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-20T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "NMB", IssueType: "Bug", Status: "완료", StatusID: "10001",
					StatusCategory: "done", Priority: "Medium", Assignee: "이클라",
					AssigneeID: "acc-cl", AssigneeEmail: "cl@example.com",
					Labels: []string{"cloud"},
				},
			},
			{
				Item: store.Item{
					ID: "jira:2001", SourceID: "jira", ExternalID: "2001", Key: "NMA-9",
					Title:     "modeler crash on import",
					CreatedAt: "2026-07-05T00:00:00.000Z", UpdatedAt: "2026-07-06T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "NMA", IssueType: "Task", Status: "할 일", StatusID: "1",
					StatusCategory: "new",
				},
			},
		},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	cfg := &config.Config{
		// Dead loopback: HasCredential stays true so settings PUT still
		// covers the startSyncJob branch, but the job cannot leave the
		// box (GDK-304). 127.0.0.1:1 is connection-refused immediately.
		Site: "http://127.0.0.1:1", Email: "hc@example.com", Token: "secret-token",
		Projects: []string{"NMB", "NMA"},
		Members: []config.Member{{
			Email: "hc@example.com", Name: "김현철", DisplayName: "현철",
			Group: "batch", Department: "platform", JobRole: "lead",
			AvatarURL: "https://avatars/hc.png", JiraAccountID: "acc-hc",
		}},
		GroupRules: []config.GroupRule{
			{Group: "cloud", Labels: []string{"cloud"}},
			{Group: "batch", Projects: []string{"NMB"}, Labels: []string{"batch"}},
		},
		GroupLabels: map[string]string{"batch": "배치", "cloud": "클라우드"},
	}
	return db, cfg, path
}

// shrinkJiraRetryBudget is the GDK-608 test seam: one attempt, no sleep.
// Restored on cleanup so a later test in this package still sees production
// New() defaults if it constructs a client without this fixture.
func shrinkJiraRetryBudget(t *testing.T) {
	t.Helper()
	prevRetries, prevBackoff := jira.DefaultRetries, jira.DefaultBackoff
	jira.DefaultRetries, jira.DefaultBackoff = 1, 0
	t.Cleanup(func() {
		jira.DefaultRetries, jira.DefaultBackoff = prevRetries, prevBackoff
	})
}

// enrich writes one enrichment row the way a plugin does: raw SQL from another
// process, no Go write API involved.
func enrich(t *testing.T, path, key, kind, payload string) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("plugin open: %v", err)
	}
	defer sqlDB.Close()
	if _, err := sqlDB.Exec(`
		INSERT INTO enrichments (key, kind, payload, source, updated_at) VALUES (?,?,?,'test-plugin',?)
		ON CONFLICT(key, kind) DO UPDATE SET payload = excluded.payload`,
		key, kind, payload, store.Now()); err != nil {
		t.Fatalf("plugin write: %v", err)
	}
}

func get(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := testRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

// TestRoutesRegister is the cheapest guard there is against a whole class of
// outage: ServeMux panics at registration when two patterns overlap and neither
// is more specific, so a new route can take the server down on startup with
// nothing but a build that passed. Every other test builds a handler too, but
// this one says why it matters.
func TestRoutesRegister(t *testing.T) {
	New(nil, nil)
}

func TestBootstrapUpdateFields(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	prev := Version
	t.Cleanup(func() { Version = prev })
	Version = "0.3.0"

	// No cached release → fields omitted.
	body := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if body.LatestVersion != "" || body.ReleaseURL != "" || body.ReleaseNotes != "" {
		t.Fatalf("expected empty update fields, got latest=%q url=%q notes=%q", body.LatestVersion, body.ReleaseURL, body.ReleaseNotes)
	}

	// Newer release → version, url, and notes present.
	h.s.setUpdateInfo(selfupdate.Info{
		Latest: "0.3.1",
		URL:    "https://github.com/midagedev/gadak/releases/tag/v0.3.1",
		Notes:  "Fixed the flaky upload.",
	}, true)
	body = decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if body.LatestVersion != "0.3.1" {
		t.Fatalf("latest_version %q", body.LatestVersion)
	}
	if body.ReleaseURL == "" {
		t.Fatal("release_url empty")
	}
	if body.ReleaseNotes != "Fixed the flaky upload." {
		t.Fatalf("release_notes %q", body.ReleaseNotes)
	}

	// Newer release, empty body → version+url stay, notes omit.
	h.s.setUpdateInfo(selfupdate.Info{
		Latest: "0.3.1",
		URL:    "https://github.com/midagedev/gadak/releases/tag/v0.3.1",
	}, true)
	body = decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if body.LatestVersion != "0.3.1" || body.ReleaseNotes != "" {
		t.Fatalf("empty notes should omit, got latest=%q notes=%q", body.LatestVersion, body.ReleaseNotes)
	}

	// Same version → omit.
	h.s.setUpdateInfo(selfupdate.Info{
		Latest: "0.3.0",
		URL:    "https://github.com/midagedev/gadak/releases/tag/v0.3.0",
	}, true)
	body = decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if body.LatestVersion != "" || body.ReleaseURL != "" {
		t.Fatalf("same version should omit fields, got latest=%q url=%q", body.LatestVersion, body.ReleaseURL)
	}

	// Older release than running → omit.
	h.s.setUpdateInfo(selfupdate.Info{
		Latest: "0.2.0",
		URL:    "https://github.com/midagedev/gadak/releases/tag/v0.2.0",
	}, true)
	body = decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if body.LatestVersion != "" || body.ReleaseURL != "" {
		t.Fatalf("older release should omit fields, got latest=%q url=%q", body.LatestVersion, body.ReleaseURL)
	}
}

func TestDeltaUpdateFields(t *testing.T) {
	// GDK-214: delta is the 15s poll path. A newer release cached after
	// bootstrap must ride this document — bootstrap's ETag is sync version
	// and will 304, so this is the only live delivery.
	db, cfg := fixture(t)
	h := New(db, cfg)

	prev := Version
	t.Cleanup(func() { Version = prev })
	Version = "0.3.0"

	// Decode as a map so omitempty is visible as a missing key, not "".
	raw := func() map[string]any {
		t.Helper()
		rec := get(t, h, apiBase+"delta/", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return m
	}

	body := raw()
	if _, ok := body["latest_version"]; ok {
		t.Fatalf("expected empty update fields, got latest=%v url=%v", body["latest_version"], body["release_url"])
	}
	if _, ok := body["release_url"]; ok {
		t.Fatalf("expected omitted release_url, got %v", body["release_url"])
	}
	if _, ok := body["release_notes"]; ok {
		t.Fatalf("expected omitted release_notes, got %v", body["release_notes"])
	}

	h.s.setUpdateInfo(selfupdate.Info{
		Latest: "0.3.1",
		URL:    "https://github.com/midagedev/gadak/releases/tag/v0.3.1",
		Notes:  "Fixed the flaky upload.",
	}, true)
	body = raw()
	if body["latest_version"] != "0.3.1" {
		t.Fatalf("latest_version %v", body["latest_version"])
	}
	if body["release_url"] == nil || body["release_url"] == "" {
		t.Fatal("release_url empty")
	}
	if body["release_notes"] != "Fixed the flaky upload." {
		t.Fatalf("release_notes %v", body["release_notes"])
	}

	h.s.setUpdateInfo(selfupdate.Info{
		Latest: "0.3.0",
		URL:    "https://github.com/midagedev/gadak/releases/tag/v0.3.0",
	}, true)
	body = raw()
	if _, ok := body["latest_version"]; ok {
		t.Fatalf("same version should omit fields, got latest=%v url=%v", body["latest_version"], body["release_url"])
	}

	h.s.setUpdateInfo(selfupdate.Info{
		Latest: "0.2.0",
		URL:    "https://github.com/midagedev/gadak/releases/tag/v0.2.0",
	}, true)
	body = raw()
	if _, ok := body["latest_version"]; ok {
		t.Fatalf("older release should omit fields, got latest=%v url=%v", body["latest_version"], body["release_url"])
	}
}

func TestForceCheckBypassesCache(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v0.4.0",
			"html_url": "https://github.com/midagedev/gadak/releases/tag/v0.4.0",
		})
	}))
	t.Cleanup(srv.Close)
	prevAPI := selfupdate.APIBase
	selfupdate.APIBase = srv.URL
	t.Cleanup(func() { selfupdate.APIBase = prevAPI })

	db, cfg := fixture(t)
	h := New(db, cfg)
	prev := Version
	t.Cleanup(func() { Version = prev })
	Version = "0.3.0"

	dir := t.TempDir()
	fresh := selfupdate.Info{
		Latest:    "0.3.1",
		URL:       "https://example.invalid/old",
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "update-check.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Cached Check must not increment hits — otherwise the force-check
	// assertion would pass even if CheckNow did not bypass the file.
	if info, ok := selfupdate.Check(context.Background(), dir, "0.3.0", true); !ok || info.Latest != "0.3.1" {
		t.Fatalf("precondition: cache should serve 0.3.1, ok=%v latest=%q", ok, info.Latest)
	}
	if hits.Load() != 0 {
		t.Fatalf("precondition: cache must not hit network, hits=%d", hits.Load())
	}

	got := h.CheckNow(context.Background(), dir)
	if hits.Load() != 1 {
		t.Fatalf("force check hits=%d, want 1", hits.Load())
	}
	if got.Latest != "0.4.0" {
		t.Fatalf("latest %q", got.Latest)
	}
	if !got.Newer || got.Status != "newer" {
		t.Fatalf("status %+v", got)
	}

	var body map[string]any
	rec := get(t, h, apiBase+"delta/", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["latest_version"] != "0.4.0" {
		t.Fatalf("delta latest_version %v", body["latest_version"])
	}
}

func TestForceCheckDevSkipsNetwork(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	prevAPI := selfupdate.APIBase
	selfupdate.APIBase = srv.URL
	t.Cleanup(func() { selfupdate.APIBase = prevAPI })

	db, cfg := fixture(t)
	h := New(db, cfg)
	prev := Version
	t.Cleanup(func() { Version = prev })
	Version = "0.0.0-dev"

	got := h.CheckNow(context.Background(), t.TempDir())
	if got.Status != "dev" {
		t.Fatalf("status %q", got.Status)
	}
	if hits.Load() != 0 {
		t.Fatalf("dev force check must not hit network, hits=%d", hits.Load())
	}
}

func TestUpdateSnapshotEndpoint(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	prev := Version
	t.Cleanup(func() { Version = prev })
	Version = "0.3.0"

	rec := get(t, h, apiBase+"update/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	got := decode[UpdateStatus](t, rec)
	if got.Current != "0.3.0" {
		t.Fatalf("current %q", got.Current)
	}
	if got.Latest != "" {
		t.Fatalf("expected no latest, got %q", got.Latest)
	}

	h.s.setUpdateInfo(selfupdate.Info{
		Latest:    "0.3.1",
		URL:       "https://github.com/midagedev/gadak/releases/tag/v0.3.1",
		Notes:     "Fixed the flaky upload.",
		CheckedAt: "2026-08-18T00:00:00Z",
	}, true)
	got = decode[UpdateStatus](t, get(t, h, apiBase+"update/", nil))
	if got.Latest != "0.3.1" || !got.Newer {
		t.Fatalf("%+v", got)
	}
	if got.CheckedAt != "2026-08-18T00:00:00Z" {
		t.Fatalf("checked_at %q", got.CheckedAt)
	}
	if got.Notes != "Fixed the flaky upload." {
		t.Fatalf("release_notes %q", got.Notes)
	}
	if got.NotesLen != len("Fixed the flaky upload.") {
		t.Fatalf("release_notes_len %d", got.NotesLen)
	}
}

func TestBootstrapShapeAndETag(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	tag := rec.Header().Get("ETag")
	if !strings.HasPrefix(tag, `"sv-`) {
		t.Fatalf("ETag %q", tag)
	}

	body := decode[bootstrapResponse](t, rec)
	if body.ServerTime == "" || body.SyncVersion == 0 {
		t.Fatalf("server_time %q sync_version %d", body.ServerTime, body.SyncVersion)
	}
	if len(body.Issues) != 3 {
		t.Fatalf("issues %d", len(body.Issues))
	}
	if !strings.HasPrefix(body.MembersVersion, "sha256:") {
		t.Fatalf("members_version %q", body.MembersVersion)
	}
	if len(body.SyncHealth.Sources) != 1 || body.SyncHealth.Sources[0].Key != "jira" {
		t.Fatalf("sync_health %+v", body.SyncHealth)
	}

	// group rules: first match wins, so NMB-1 (labels batch) is batch and NMB-2
	// (labels cloud) is cloud even though both live in project NMB.
	groups := map[string]string{}
	for _, i := range body.Issues {
		groups[i.IssueKey] = deref(i.TeamGroup)
	}
	if groups["NMB-1"] != "batch" || groups["NMB-2"] != "cloud" {
		t.Fatalf("team_group %v", groups)
	}
	// No rule matches NMA-9 and it has no assignee, so the fallback yields nothing.
	if groups["NMA-9"] != "" {
		t.Fatalf("NMA-9 group %q", groups["NMA-9"])
	}

	// members: derived from the mirror, overridden by the configured directory.
	byEmail := map[string]member{}
	for _, m := range body.Members {
		byEmail[m.Email] = m
	}
	hc, ok := byEmail["hc@example.com"]
	if !ok {
		t.Fatalf("members %+v", body.Members)
	}
	if deref(hc.Group) != "batch" || deref(hc.DisplayName) != "현철" ||
		deref(hc.ProfileImage) != "https://avatars/hc.png" || deref(hc.JiraAccountID) != "acc-hc" {
		t.Fatalf("configured member not merged: %+v", hc)
	}
	if cl, ok := byEmail["cl@example.com"]; !ok || cl.Name != "이클라" || cl.Group != nil {
		t.Fatalf("derived member wrong: %+v ok=%v", cl, ok)
	}
	if rp, ok := byEmail["rp@example.com"]; !ok || rp.Name != "박보고" || deref(rp.JiraAccountID) != "acc-rp" {
		t.Fatalf("reporter member wrong: %+v ok=%v", rp, ok)
	}

	// 304 on the tag we just got, and on the client's own `"in-<version>"` form.
	for _, inm := range []string{tag, `"in-` + tag[len(`"sv-`):]} {
		rec := get(t, h, apiBase+"bootstrap/", map[string]string{"If-None-Match": inm})
		if rec.Code != http.StatusNotModified {
			t.Fatalf("If-None-Match %s → %d", inm, rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("304 carried a body: %s", rec.Body.String())
		}
	}
	if rec := get(t, h, apiBase+"bootstrap/", map[string]string{"If-None-Match": `"sv-999"`}); rec.Code != http.StatusOK {
		t.Fatalf("stale ETag → %d", rec.Code)
	}
}

func TestDeltaUpsertedAndDeleted(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	full := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))

	// The store's cursor bound is inclusive to the millisecond, so a cursor taken
	// in the same millisecond as the fixture writes legitimately re-sends them.
	// Wait one tick to ask the question this test is actually about: a later poll.
	time.Sleep(2 * time.Millisecond)
	quiet := store.Now()
	idle := decode[deltaResponse](t, get(t, h, apiBase+"delta/?since="+quiet, nil))
	if len(idle.Upserted) != 0 || len(idle.DeletedKeys) != 0 {
		t.Fatalf("idle poll moved: %+v", idle)
	}
	// members are omitted while the client's hash matches, and sent when it does not.
	if got := decode[deltaResponse](t, get(t, h, apiBase+"delta/?since="+quiet+"&mv="+full.MembersVersion, nil)); got.Members != nil {
		t.Fatalf("members resent for a matching mv: %+v", got.Members)
	}
	if idle.Members == nil {
		t.Fatal("members omitted without an mv")
	}
	if idle.MembersVersion != full.MembersVersion {
		t.Fatalf("members_version drifted: %q vs %q", idle.MembersVersion, full.MembersVersion)
	}

	cursor := store.Now()
	if _, err := db.DeleteItems(context.Background(), "jira", []string{"NMB-2"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Force: true,
		Records: []store.IssueRecord{{
			Item:  store.Item{ID: "jira:2001", SourceID: "jira", ExternalID: "2001", Key: "NMA-9", Title: "modeler crash on import (edited)"},
			Issue: store.Issue{ProjectKey: "NMA", Status: "할 일", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}

	got := decode[deltaResponse](t, get(t, h, apiBase+"delta/?since="+cursor, nil))
	if len(got.Upserted) != 1 || got.Upserted[0].IssueKey != "NMA-9" {
		t.Fatalf("upserted %+v", got.Upserted)
	}
	if len(got.DeletedKeys) != 1 || got.DeletedKeys[0] != "NMB-2" {
		t.Fatalf("deleted_keys %v", got.DeletedKeys)
	}
}

func TestDetailAssembly(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"NMB-1/detail/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	// The cut integrations stay in the shape as null / [].
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"development_opinion", "qa_context", "deploy"} {
		if string(raw[k]) != "null" {
			t.Fatalf("%s = %s, want null", k, raw[k])
		}
	}
	if string(raw["linked_prs"]) != "[]" {
		t.Fatalf("linked_prs = %s", raw["linked_prs"])
	}
	var detailKey, detailIssueKey string
	_ = json.Unmarshal(raw["issue_key"], &detailIssueKey)
	_ = json.Unmarshal(raw["key"], &detailKey)
	if detailIssueKey != "NMB-1" || detailKey != detailIssueKey {
		t.Errorf("detail key alias diverged: issue_key=%q key=%q", detailIssueKey, detailKey)
	}

	d := decode[detailResponse](t, rec)
	if len(d.Attachments) != 1 {
		t.Fatalf("attachments %+v", d.Attachments)
	}
	a := d.Attachments[0]
	if want := apiBase + "NMB-1/attachments/10021/content/"; a.ContentURL != want {
		t.Fatalf("content_url %q want %q", a.ContentURL, want)
	}
	if !a.IsImage || a.IsVideo {
		t.Fatalf("mime flags %+v", a)
	}
	if len(d.Comments) != 1 || d.Comments[0].CommentID != "c-1" {
		t.Fatalf("comments %+v", d.Comments)
	}
	// The comment author's email is resolved through the configured account id.
	if got := deref(d.Comments[0].AuthorEmail); got != "hc@example.com" {
		t.Fatalf("author_email %q", got)
	}
	if len(d.Comments[0].RawBody) == 0 {
		t.Fatal("raw_body dropped")
	}
	// The flattened body is the client's fallback when the ADF will not render.
	if d.Comments[0].Body != "hydra parity check failed" {
		t.Fatalf("body %q", d.Comments[0].Body)
	}
	if len(d.History) != 1 {
		t.Fatalf("history %+v", d.History)
	}
	// Categories come from the status ids, never from the localized names.
	if got := deref(d.History[0].FromCategory); got != "new" {
		t.Fatalf("from_category %q", got)
	}
	if got := deref(d.History[0].ToCategory); got != "inprogress" {
		t.Fatalf("to_category %q", got)
	}
	if len(d.LinkedIssues) != 1 || deref(d.LinkedIssues[0].Summary) != "cloud upload retries forever" ||
		deref(d.LinkedIssues[0].StatusCategory) != "done" {
		t.Fatalf("linked_issues %+v", d.LinkedIssues)
	}

	if rec := get(t, h, apiBase+"NOPE-1/detail/", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown key → %d", rec.Code)
	}
}

func TestSearchHitsCommentText(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	got := decode[struct {
		Keys  []string `json:"keys"`
		Total int      `json:"total"`
	}](t, get(t, h, apiBase+"search/?q=hydra&limit=10", nil))
	if got.Total != 1 || len(got.Keys) != 1 || got.Keys[0] != "NMB-1" {
		t.Fatalf("search %+v", got)
	}
	// Raw user input FTS5 cannot parse must not 500.
	if rec := get(t, h, apiBase+`search/?q=hydra+"unclosed`, nil); rec.Code != http.StatusOK {
		t.Fatalf("unparseable query → %d", rec.Code)
	}
}

func TestDeferredEndpointsAre404(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	// Feed is implemented (GET feed/ → 200). These remain deferred/cut.
	for _, path := range []string{
		apiBase + "notifications/config/", apiBase + "notifications/subscription/",
		apiBase + "presence-ticket/", apiBase + "ws/issues/",
		apiBase + "mentions/?email=hc@example.com", apiBase + "data-quality/",
		authBase + "login/", authBase + "logout/",
	} {
		rec := get(t, h, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s → %d, want 404", path, rec.Code)
		}
		if got := decode[map[string]string](t, rec)["error"]; got == "" {
			t.Fatalf("%s: 404 without an error code", path)
		}
	}
}

func TestAttachmentProxyNeedsCredential(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{})
	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", rec.Code)
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "credential_required" {
		t.Fatalf("error %q", got)
	}
}

func TestAttachmentProxyStreamsFromJira(t *testing.T) {
	var gotAuth, gotPath string
	mime := "image/png"
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		if mime == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", mime)
		_, _ = w.Write([]byte("PNGBYTES"))
	}))
	defer jira.Close()

	db, cfg := fixture(t)
	cfg.Site = jira.URL
	h := New(db, cfg)

	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "PNGBYTES" {
		t.Fatalf("proxy → %d %q", rec.Code, rec.Body.String())
	}
	if gotPath != "/rest/api/3/attachment/content/10021" {
		t.Fatalf("upstream path %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("upstream auth %q", gotAuth)
	}
	if rec.Header().Get("Content-Type") != "image/png" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers %v", rec.Header())
	}
	if got := rec.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("image forced to download: %q", got)
	}

	// SVG is an image that executes script, so it may never render inline.
	mime = "image/svg+xml"
	rec = get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("svg Content-Disposition %q", got)
	}

	// A rejected token has to reach the UI as credential_rejected, not as a
	// broken image.
	mime = ""
	rec = get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("upstream 401 → %d", rec.Code)
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "credential_rejected" {
		t.Fatalf("error %q", got)
	}
}

// I5: a roster row with account_id and no email must appear in members.
// addMember currently returns on empty email and drops them.
func TestMembersIncludeAccountIDOnlyRosterRows(t *testing.T) {
	db, cfg := fixture(t)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Force: true,
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:bot-1", SourceID: "jira", ExternalID: "bot-1", Key: "NMB-80",
				Title: "automation", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Task", Status: "할 일", StatusID: "1", StatusCategory: "new",
				Assignee: "Jira Automation", AssigneeID: "acc-bot", AssigneeEmail: "",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	h := New(db, cfg)
	body := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	var bot *member
	for i := range body.Members {
		if deref(body.Members[i].JiraAccountID) == "acc-bot" {
			bot = &body.Members[i]
			break
		}
	}
	if bot == nil {
		t.Fatalf("account-id-only assignee missing from members: %+v", body.Members)
	}
	if bot.Name != "Jira Automation" {
		t.Fatalf("account-id-only member %+v", bot)
	}

	// A later row for the same account that also has an email must merge, not
	// produce a second directory entry.
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Force: true,
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:bot-2", SourceID: "jira", ExternalID: "bot-2", Key: "NMB-81",
				Title: "same bot with email", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-02T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Task", Status: "할 일", StatusID: "1", StatusCategory: "new",
				Assignee: "Jira Automation", AssigneeID: "acc-bot", AssigneeEmail: "bot@example.invalid",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	body = decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	bots := 0
	var merged *member
	for i := range body.Members {
		if deref(body.Members[i].JiraAccountID) == "acc-bot" {
			bots++
			merged = &body.Members[i]
		}
	}
	if bots != 1 {
		t.Fatalf("acc-bot members = %d, want 1 (dedup id-only + email row): %+v", bots, body.Members)
	}
	if merged == nil || merged.Email != "bot@example.invalid" {
		t.Fatalf("merged bot member %+v", merged)
	}
}

func TestMe(t *testing.T) {
	db, cfg := fixture(t)
	got := decode[map[string]any](t, get(t, New(db, cfg), authBase+"me/", nil))
	if got["email"] != "hc@example.com" || got["account_id"] != "acc-hc" || got["name"] != "현철" {
		t.Fatalf("me %+v", got)
	}
	// No credential: 200 with a null identity, not 401 — the UI probes this on
	// every boot and a 4xx would land in the browser console.
	anon := decode[map[string]any](t, get(t, New(db, &config.Config{}), authBase+"me/", nil))
	if email, ok := anon["email"]; !ok || email != nil {
		t.Fatalf("uncredentialed me/ %+v", anon)
	}
}

func TestPersonalStateRoundtrip(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	req := testRequest(http.MethodPost, apiBase+"views/",
		strings.NewReader(`{"name":"내 배치","config":{"group":["batch"]}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST views/ → %d %s", rec.Code, rec.Body.String())
	}
	created := decode[savedView](t, rec)
	if created.ID == "" || deref(created.OwnerEmail) != "hc@example.com" {
		t.Fatalf("created view %+v", created)
	}

	list := decode[struct{ Views []savedView }](t, get(t, h, apiBase+"views/", nil))
	if len(list.Views) != 1 || string(list.Views[0].Config) != `{"group":["batch"]}` {
		t.Fatalf("views %+v", list.Views)
	}

	for _, tc := range []struct{ method, path string }{
		{http.MethodDelete, apiBase + "views/" + created.ID + "/"},
		{http.MethodPut, apiBase + "watches/NMB-1/"},
		{http.MethodPut, apiBase + "favorites/NMB-1/"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, testRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s → %d", tc.method, tc.path, rec.Code)
		}
	}
	if got := decode[struct{ Keys []string }](t, get(t, h, apiBase+"watches/", nil)); len(got.Keys) != 1 || got.Keys[0] != "NMB-1" {
		t.Fatalf("watches %+v", got.Keys)
	}
	if got := decode[struct{ Keys []string }](t, get(t, h, apiBase+"favorites/", nil)); len(got.Keys) != 1 || got.Keys[0] != "NMB-1" {
		t.Fatalf("favorites %+v", got.Keys)
	}
	if got := decode[struct{ Views []savedView }](t, get(t, h, apiBase+"views/", nil)); len(got.Views) != 0 {
		t.Fatalf("view survived deletion: %+v", got.Views)
	}
}

// TestFavoritesRoundtrip covers PUT → GET → DELETE → GET empty, and that the
// shared PUT pattern routes favorites/{key}/ and {key}/assignee/ correctly
// (ServeMux would panic at registration if they were separate patterns).
func TestFavoritesRoundtrip(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	put := httptest.NewRecorder()
	h.ServeHTTP(put, testRequest(http.MethodPut, apiBase+"favorites/NMB-1/", nil))
	if put.Code != http.StatusNoContent {
		t.Fatalf("PUT favorites/NMB-1/ → %d %s", put.Code, put.Body.String())
	}
	if got := decode[struct{ Keys []string }](t, get(t, h, apiBase+"favorites/", nil)); len(got.Keys) != 1 || got.Keys[0] != "NMB-1" {
		t.Fatalf("after PUT: favorites %+v", got.Keys)
	}

	del := httptest.NewRecorder()
	h.ServeHTTP(del, testRequest(http.MethodDelete, apiBase+"favorites/NMB-1/", nil))
	if del.Code != http.StatusNoContent {
		t.Fatalf("DELETE favorites/NMB-1/ → %d %s", del.Code, del.Body.String())
	}
	if got := decode[struct{ Keys []string }](t, get(t, h, apiBase+"favorites/", nil)); len(got.Keys) != 0 {
		t.Fatalf("after DELETE: favorites %+v", got.Keys)
	}
}

func TestSettingsRoundtripPreservesCredential(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	h := New(db, cfg)

	before := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if !before.HasCredential || before.Site != "http://127.0.0.1:1" {
		t.Fatalf("settings %+v", before)
	}
	if before.Features["qa"] || len(before.Features) != len(featureNames) {
		t.Fatalf("features %+v", before.Features)
	}

	before.GroupRules = []config.GroupRule{{Group: "platform", Projects: []string{"NMB"}}}
	before.Features = map[string]bool{"teamGroups": true, "bogus": true}
	before.StaleThresholdHours = 48
	body, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings/ → %d %s", rec.Code, rec.Body.String())
	}

	after := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if len(after.GroupRules) != 1 || after.GroupRules[0].Group != "platform" {
		t.Fatalf("rules not stored: %+v", after.GroupRules)
	}
	if !after.Features["teamGroups"] || after.StaleThresholdHours != 48 {
		t.Fatalf("after %+v", after)
	}
	if _, ok := after.Features["bogus"]; ok {
		t.Fatal("unknown feature flag accepted")
	}

	// The credential block survives a settings write, on disk and in the handler.
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.Token != "secret-token" || saved.Email != "hc@example.com" {
		t.Fatalf("credential lost: %+v", saved)
	}
	if saved.GroupRules[0].Group != "platform" {
		t.Fatalf("not persisted: %+v", saved.GroupRules)
	}

	// New rules take effect immediately: the cached projection is invalidated.
	boot := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	for _, i := range boot.Issues {
		if i.ProjectKey == "NMB" && deref(i.TeamGroup) != "platform" {
			t.Fatalf("%s team_group %q, want platform", i.IssueKey, deref(i.TeamGroup))
		}
	}
}

func TestWebConfigHidesCredential(t *testing.T) {
	_, cfg := fixture(t)
	doc, err := WebConfig(cfg)
	if err != nil {
		t.Fatalf("WebConfig: %v", err)
	}
	for _, secret := range []string{"secret-token", "token", "hc@example.com", "email"} {
		if strings.Contains(string(doc), secret) {
			t.Fatalf("config.json leaked %q: %s", secret, doc)
		}
	}
	var got webConfigDoc
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.APIBase != apiBase || got.AuthBase != authBase {
		t.Fatalf("bases %+v", got)
	}
	if got.JiraBaseURL != "http://127.0.0.1:1" || got.GroupLabels["batch"] != "배치" {
		t.Fatalf("doc %+v", got)
	}
	// An unset threshold must not serialize as 0 — that would mark every issue stale.
	if got.StaleThresholdHours != defaultStaleHours {
		t.Fatalf("staleThresholdHours %d", got.StaleThresholdHours)
	}
	if len(got.Features) != len(featureNames) || got.Features["qa"] {
		t.Fatalf("features %+v", got.Features)
	}
}

func TestIssueLiteFieldNames(t *testing.T) {
	db, cfg := fixture(t)
	body := decode[struct {
		Issues []map[string]json.RawMessage `json:"issues"`
	}](t, get(t, New(db, cfg), apiBase+"bootstrap/", nil))
	rows := map[string]map[string]json.RawMessage{}
	for _, row := range body.Issues {
		var key string
		_ = json.Unmarshal(row["issue_key"], &key)
		rows[key] = row
	}
	// The client stores these rows verbatim in IndexedDB, so the names are a
	// contract (contracts/api.md, "IssueLite").
	for _, field := range []string{
		"issue_key", "key", "summary", "project_key", "issue_type", "status", "status_id",
		"status_category", "priority", "priority_rank", "assignee", "assignee_id", "assignee_email",
		"reporter", "reporter_id", "reporter_email", "labels", "components", "fix_versions", "epic_key",
		"parent_key", "hierarchy_level",
		"created_at", "updated_at", "status_changed_at", "resolved_at", "reopen_count",
		"comment_count", "team_group",
	} {
		if _, ok := rows["NMB-1"][field]; !ok {
			t.Errorf("issue row is missing %q", field)
		}
	}
	var issueKey, keyAlias string
	_ = json.Unmarshal(rows["NMB-1"]["issue_key"], &issueKey)
	_ = json.Unmarshal(rows["NMB-1"]["key"], &keyAlias)
	if issueKey != "NMB-1" || keyAlias != issueKey {
		t.Errorf("key alias diverged: issue_key=%q key=%q", issueKey, keyAlias)
	}
	// Configured aliases arrive as top-level fields, which is where the client's
	// filters and columns read them — never as a nested `custom` object.
	if got := string(rows["NMB-1"]["severity"]); got != `"S2"` {
		t.Errorf("severity = %s", got)
	}
	if got := string(rows["NMB-1"]["solution"]); got != `"Fixed"` {
		t.Errorf("solution = %s", got)
	}
	if _, nested := rows["NMB-1"]["custom"]; nested {
		t.Error("custom leaked as a nested object")
	}
	// An issue with no aliases still carries the stored fields.
	if _, ok := rows["NMA-9"]["issue_key"]; !ok {
		t.Errorf("row without aliases lost its fields: %v", rows["NMA-9"])
	}
}

// TestIssueLiteHierarchyLevelOnBootstrap is GDK-329: the REST IssueLite
// carries issues.hierarchy_level as stored, including a sub-task −1.
func TestIssueLiteHierarchyLevelOnBootstrap(t *testing.T) {
	db, cfg := fixture(t)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1099", SourceID: "jira", ExternalID: "1099", Key: "NMB-99",
				Title: "child of NMB-1", CreatedAt: "2026-07-01T00:00:00.000Z",
				UpdatedAt: "2026-08-02T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Sub-task", IssueTypeID: "10005",
				Status: "할 일", StatusID: "1", StatusCategory: "new",
				ParentKey: "NMB-1", HierarchyLevel: -1,
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	body := decode[struct {
		Issues []map[string]json.RawMessage `json:"issues"`
	}](t, get(t, New(db, cfg), apiBase+"bootstrap/", nil))
	var found bool
	for _, row := range body.Issues {
		var key string
		_ = json.Unmarshal(row["issue_key"], &key)
		if key != "NMB-99" {
			continue
		}
		found = true
		t.Logf("NMB-99 IssueLite JSON fields hierarchy_level=%s parent_key=%s", row["hierarchy_level"], row["parent_key"])
		var level int
		if err := json.Unmarshal(row["hierarchy_level"], &level); err != nil {
			t.Fatalf("hierarchy_level %s: %v", row["hierarchy_level"], err)
		}
		if level != -1 {
			t.Errorf("NMB-99 hierarchy_level=%d want -1", level)
		}
		var parent *string
		if err := json.Unmarshal(row["parent_key"], &parent); err != nil {
			t.Fatalf("parent_key %s: %v", row["parent_key"], err)
		}
		if parent == nil || *parent != "NMB-1" {
			t.Errorf("NMB-99 parent_key=%v want NMB-1", parent)
		}
	}
	if !found {
		t.Fatal("NMB-99 missing from bootstrap")
	}
}

func TestGroupFallbackUsesAssigneeAccountID(t *testing.T) {
	v := derivedView{
		groupsEnabled:  true,
		groupByAccount: map[string]string{"acc-hidden": "platform"},
		groupByEmail:   map[string]string{"visible@example.com": "legacy"},
	}
	if got := deref(v.group(store.IssueLite{AssigneeID: nilIfEmpty("acc-hidden")})); got != "platform" {
		t.Fatalf("account-id group = %q", got)
	}
	if got := deref(v.group(store.IssueLite{AssigneeEmail: nilIfEmpty("visible@example.com")})); got != "legacy" {
		t.Fatalf("email fallback group = %q", got)
	}
}

func TestEnrichmentsMerge(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	// Wrapped payloads: one kind feeds both the list row and the detail panel.
	enrich(t, path, "NMB-1", "deploy",
		`{"status":{"state":"prod","merged_prs":2,"total_prs":2},"detail":{"state":"prod","releases":[{"tag":"v1.2.3"}]}}`)
	enrich(t, path, "NMB-1", "qa",
		`{"impact":{"qa_impact_state":"blocking","qa_impact_label":"차단","qa_runs":[{"key":"R-1","label":"run"}],"qa_suites":[]},"context":{"state":"blocking","state_label":"차단","runs":[],"suites":[]}}`)
	enrich(t, path, "NMB-1", "prs", `[{"number":7,"title":"fix the drop","url":"https://x/pr/7","state":"merged"}]`)
	enrich(t, path, "NMB-1", "opinion", `"재현 조건이 좁다"`)
	// The bare shape data-model.md documents still works, unwrapped.
	enrich(t, path, "NMB-2", "deploy", `{"state":"merged","merged_prs":1,"total_prs":1}`)
	// A plugin writing garbage may not corrupt the document.
	enrich(t, path, "NMA-9", "deploy", `{oops`)
	h := New(db, cfg)

	rows := map[string]map[string]json.RawMessage{}
	for _, row := range decode[struct {
		Issues []map[string]json.RawMessage `json:"issues"`
	}](t, get(t, h, apiBase+"bootstrap/", nil)).Issues {
		var key string
		_ = json.Unmarshal(row["issue_key"], &key)
		rows[key] = row
	}
	if got := string(rows["NMB-1"]["deploy_status"]); !strings.Contains(got, `"state":"prod"`) {
		t.Errorf("deploy_status = %s", got)
	}
	if got := string(rows["NMB-1"]["qa_impact_state"]); got != `"blocking"` {
		t.Errorf("qa_impact_state = %s", got)
	}
	if got := string(rows["NMB-1"]["qa_runs"]); got != `[{"key":"R-1","label":"run"}]` {
		t.Errorf("qa_runs = %s", got)
	}
	if got := string(rows["NMB-2"]["deploy_status"]); !strings.Contains(got, `"state":"merged"`) {
		t.Errorf("unwrapped payload dropped: %s", got)
	}
	if _, ok := rows["NMA-9"]["deploy_status"]; ok {
		t.Error("invalid payload reached the response")
	}
	// The row without an enrichment keeps its own fields.
	if _, ok := rows["NMA-9"]["issue_key"]; !ok {
		t.Error("row lost its fields")
	}

	d := decode[map[string]json.RawMessage](t, get(t, h, apiBase+"NMB-1/detail/", nil))
	if got := string(d["deploy"]); !strings.Contains(got, `"v1.2.3"`) {
		t.Errorf("deploy = %s", got)
	}
	if got := string(d["qa_context"]); !strings.Contains(got, `"state_label":"차단"`) {
		t.Errorf("qa_context = %s", got)
	}
	if got := string(d["linked_prs"]); !strings.Contains(got, `"number":7`) {
		t.Errorf("linked_prs = %s", got)
	}
	if got := string(d["development_opinion"]); got != `"재현 조건이 좁다"` {
		t.Errorf("development_opinion = %s", got)
	}
	// An unwrapped deploy payload serves the detail panel too.
	if got := string(decode[map[string]json.RawMessage](t, get(t, h, apiBase+"NMB-2/detail/", nil))["deploy"]); !strings.Contains(got, `"merged"`) {
		t.Errorf("NMB-2 deploy = %s", got)
	}
	// An issue nobody enriched keeps the null / [] the client guards on.
	nma := decode[map[string]json.RawMessage](t, get(t, h, apiBase+"NMA-9/detail/", nil))
	if string(nma["qa_context"]) != "null" || string(nma["linked_prs"]) != "[]" {
		t.Errorf("unenriched detail: %s %s", nma["qa_context"], nma["linked_prs"])
	}
}

func TestEnrichmentCannotShadowMirroredFields(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	enrich(t, path, "NMB-1", "qa", `{"impact":{"status":"HACKED","summary":"HACKED","qa_impact_state":"verified"}}`)
	rows := decode[struct {
		Issues []map[string]json.RawMessage `json:"issues"`
	}](t, get(t, New(db, cfg), apiBase+"bootstrap/", nil)).Issues
	for _, row := range rows {
		var key string
		_ = json.Unmarshal(row["issue_key"], &key)
		if key != "NMB-1" {
			continue
		}
		// The stored fields are serialized last, so they are what JSON.parse keeps.
		var status, summary string
		_ = json.Unmarshal(row["status"], &status)
		_ = json.Unmarshal(row["summary"], &summary)
		if status != "진행 중" || !strings.HasPrefix(summary, "batch worker") {
			t.Fatalf("plugin shadowed a mirrored field: status=%q summary=%q", status, summary)
		}
		if got := string(row["qa_impact_state"]); got != `"verified"` {
			t.Fatalf("qa_impact_state = %s", got)
		}
	}
}

func TestSyncHealth(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	if got := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil)).SyncHealth; got.Overall != "warning" ||
		got.Sources[0].Status != "missing" {
		t.Fatalf("never-synced mirror: %+v", got)
	}
	if err := db.RecordSync(context.Background(), sourceID, store.SyncResult{Watermark: "2026-08-04T00:00:00.000Z"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil)).SyncHealth; got.Overall != "healthy" ||
		got.Sources[0].Message != "ok" || got.Sources[0].SyncedAt == nil {
		t.Fatalf("after a good run: %+v", got)
	}
	if err := db.RecordSync(context.Background(), sourceID, store.SyncResult{Err: errors.New("401 from jira")}); err != nil {
		t.Fatalf("record: %v", err)
	}
	got := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil)).SyncHealth
	if got.Overall != "failed" || got.Sources[0].Status != "failed" ||
		!strings.Contains(got.Sources[0].Message, "401") {
		t.Fatalf("after a failed run: %+v", got)
	}

	// Staleness is measured from the last successful run, with a wide margin.
	s := &server{}
	s.cfg.Store(&config.Config{})
	old := "2020-01-01T00:00:00.000Z"
	now := store.Now()
	if !s.stale(&old) || s.stale(&now) || s.stale(nil) {
		t.Fatal("staleness window wrong")
	}
}

func TestSyncHealthTokenExpiry(t *testing.T) {
	db, cfg := fixture(t)
	if err := db.RecordSync(context.Background(), sourceID, store.SyncResult{Watermark: "2026-08-04T00:00:00.000Z"}); err != nil {
		t.Fatal(err)
	}
	cfg.TokenExpiresAt = time.Now().UTC().Add(5*24*time.Hour + time.Hour).Format(config.TokenTimeFormat)
	cfg.TokenExpirySource = config.TokenExpirySourceAssumed
	got := decode[bootstrapResponse](t, get(t, New(db, cfg), apiBase+"bootstrap/", nil)).SyncHealth
	if got.Overall != "healthy" {
		t.Fatalf("expiring token must not flip overall (that would read as a delayed mirror): %+v", got)
	}
	if got.TokenExpiry.State != config.TokenExpiryExpiring || got.TokenExpiry.Message == "" {
		t.Fatalf("token_expiry %+v", got.TokenExpiry)
	}
	if !strings.Contains(got.TokenExpiry.Message, "assumed from the default lifetime") {
		t.Fatalf("assumed hedge missing: %q", got.TokenExpiry.Message)
	}
	if got.TokenExpiry.DaysLeft == nil || *got.TokenExpiry.DaysLeft != 5 {
		t.Fatalf("days_left %v", got.TokenExpiry.DaysLeft)
	}
}

// A groupQuery is a SELECT when it is saved, and that is all anyone checked.
// The tables and columns it names can stop existing later — a renamed field,
// a hand-edited config — and the failure surfaces on the read path. Taking
// bootstrap down there strands the person outside the settings dialog that
// would let them fix it, so classification is what gets lost, not the app.
func TestBootstrapSurvivesABrokenGroupQuery(t *testing.T) {
	db, cfg := fixture(t)
	cfg.GroupQuery = "SELECT key, 'x' FROM no_such_table"
	h := New(db, cfg)

	rec := get(t, h, apiBase+"bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("a broken groupQuery must not take bootstrap down: status %d", rec.Code)
	}
	if body := decode[bootstrapResponse](t, rec); len(body.Issues) == 0 {
		t.Fatal("issues went missing with the broken query")
	}
}

func TestHandlerShutdownCancelsSyncJob(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	if !h.s.startSyncJob(cfg, true) {
		t.Fatal("startSyncJob refused")
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := db.PoolStats().InUse; n != 0 {
		t.Fatalf("%d connection(s) still checked out after Handler.Close — background sync did not return (GDK-270)", n)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestSnapshotSyncMatchesProgressEndpoint(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	httpDoc := decode[progressResponse](t, get(t, h, apiBase+"sync/progress/", nil))
	snap := h.snapshotSync()
	if snap != httpDoc {
		t.Fatalf("snapshotSync %+v != GET sync/progress/ %+v", snap, httpDoc)
	}
}
