package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

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
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(store.Batch{
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
					Reporter:      "박보고", ReporterEmail: "rp@example.com",
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
		Site: "https://x.atlassian.net", Email: "hc@example.com", Token: "secret-token",
		Projects: []string{"NMB", "NMA"},
		Members: []config.Member{{
			Email: "hc@example.com", Name: "김현철", DisplayName: "현철",
			Group: "batch", Department: "D1", JobRole: "lead",
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
	req := httptest.NewRequest(http.MethodGet, path, nil)
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
		groups[i.IssueKey] = deref(i.D1Group)
	}
	if groups["NMB-1"] != "batch" || groups["NMB-2"] != "cloud" {
		t.Fatalf("d1_group %v", groups)
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
	if _, err := db.DeleteItems("jira", []string{"NMB-2"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.UpsertIssues(store.Batch{
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
	for _, path := range []string{
		apiBase + "feed/", apiBase + "feed/read/",
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

	// A rejected token has to reach the UI as credential_required, not as a
	// broken image.
	mime = ""
	rec = get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("upstream 401 → %d", rec.Code)
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "credential_required" {
		t.Fatalf("error %q", got)
	}
}

func TestMe(t *testing.T) {
	db, cfg := fixture(t)
	got := decode[map[string]any](t, get(t, New(db, cfg), authBase+"me/", nil))
	if got["email"] != "hc@example.com" || got["name"] != "현철" {
		t.Fatalf("me %+v", got)
	}
	rec := get(t, New(db, &config.Config{}), authBase+"me/", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("uncredentialed me/ → %d", rec.Code)
	}
}

func TestPersonalStateRoundtrip(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodPost, apiBase+"views/",
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
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s → %d", tc.method, tc.path, rec.Code)
		}
	}
	if got := decode[struct{ Keys []string }](t, get(t, h, apiBase+"watches/", nil)); len(got.Keys) != 1 || got.Keys[0] != "NMB-1" {
		t.Fatalf("watches %+v", got.Keys)
	}
	if got := decode[struct{ Views []savedView }](t, get(t, h, apiBase+"views/", nil)); len(got.Views) != 0 {
		t.Fatalf("view survived deletion: %+v", got.Views)
	}
}

func TestSettingsRoundtripPreservesCredential(t *testing.T) {
	t.Setenv("SCRY_HOME", t.TempDir())
	db, cfg := fixture(t)
	h := New(db, cfg)

	before := decode[settingsDoc](t, get(t, h, apiBase+"settings/", nil))
	if !before.HasCredential || before.Site != "https://x.atlassian.net" {
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
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, apiBase+"settings/", strings.NewReader(string(body))))
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
		if i.ProjectKey == "NMB" && deref(i.D1Group) != "platform" {
			t.Fatalf("%s d1_group %q, want platform", i.IssueKey, deref(i.D1Group))
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
	if got.JiraBaseURL != "https://x.atlassian.net" || got.GroupLabels["batch"] != "배치" {
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
		"issue_key", "summary", "project_key", "issue_type", "status", "status_id",
		"status_category", "priority", "priority_rank", "assignee", "assignee_email",
		"reporter", "reporter_email", "labels", "components", "fix_versions", "epic_key",
		"created_at", "updated_at", "status_changed_at", "resolved_at", "reopen_count",
		"comment_count", "d1_group",
	} {
		if _, ok := rows["NMB-1"][field]; !ok {
			t.Errorf("issue row is missing %q", field)
		}
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
	if err := db.RecordSync(sourceID, store.SyncResult{Watermark: "2026-08-04T00:00:00.000Z"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil)).SyncHealth; got.Overall != "healthy" ||
		got.Sources[0].Message != "정상" || got.Sources[0].SyncedAt == nil {
		t.Fatalf("after a good run: %+v", got)
	}
	if err := db.RecordSync(sourceID, store.SyncResult{Err: errors.New("401 from jira")}); err != nil {
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
