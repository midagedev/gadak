package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/store"

	_ "modernc.org/sqlite" // the tests read the mirror over SQL to check columns the read API does not expose
)

// ── fake Jira ─────────────────────────────────────────────────────────────────

// fakeSite serves the handful of endpoints the connector calls. Everything it
// returns is fixture JSON, so the mapping under test sees real payload shapes.
type fakeSite struct {
	t          *testing.T
	lang       string
	issues     []json.RawMessage
	pageSize   int
	failOffset int // offset of the sync page that answers 500; -1 for none
	changelog  map[string]string
	comments   map[string]string
	// countFail, when true, makes approximate-count answer 500 so Run must
	// continue without a progress denominator.
	countFail bool
	// lastCountJQL is the JQL last posted to approximate-count (tests assert it).
	lastCountJQL string

	mu         sync.Mutex
	syncJQL    string
	syncFields []string
	keyOnlyRun int
	// allJQLs records every search/jql body (sync + reconcile) for empty-scope tests.
	allJQLs []string
	// fieldCatalog, when set, is returned from GET /rest/api/3/field (discovery).
	fieldCatalog []map[string]any
	// fieldHits is GET /rest/api/3/field count (sprint discovery cache tests).
	fieldHits int
	// filtersJSON, when set, is GET /filter/my. Default is an empty list so
	// importFilters does not fail existing issue-sync tests.
	filtersJSON []byte
	// authStatus, when non-zero, is returned for every request. Watch tests use
	// 401 (ErrAuth, loop must stop) and 500 (transport, loop must retry).
	authStatus int
	// hits is every request this site has seen, including authStatus short-circuits.
	hits int
	// versions is GET /rest/api/3/project/{key}/versions. Missing key → `[]`
	// so existing issue-sync tests do not fail on the catalog pass (GDK-532).
	versions map[string]string
	// versionsStatus, when non-zero, is returned for every versions GET.
	versionsStatus int
	versionHits    int
}

func newSite(t *testing.T, lang string) *fakeSite {
	return &fakeSite{t: t, lang: lang, issues: fixtures(lang), pageSize: 2, failOffset: -1,
		changelog: map[string]string{}, comments: map[string]string{}}
}

func (f *fakeSite) start() *jira.Client {
	srv := httptest.NewServer(f)
	f.t.Cleanup(srv.Close)
	c := jira.New(srv.URL, "someone@example.com", "secret-token")
	c.Retries, c.Backoff = 1, 0 // no sleeping in tests
	return c
}

func (f *fakeSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits++
	if f.authStatus != 0 {
		http.Error(w, `{"errorMessages":["Client must be authenticated"]}`, f.authStatus)
		return
	}
	switch {
	case r.URL.Path == "/rest/api/3/status":
		w.Write(statusesJSON(f.lang))
	case r.URL.Path == "/rest/api/3/priority":
		w.Write([]byte(`[{"id":"1","name":"Highest"},{"id":"2","name":"High"},{"id":"3","name":"Medium"},{"id":"4","name":"Low"}]`))
	case r.URL.Path == "/rest/api/3/field":
		f.fieldHits++
		cat := f.fieldCatalog
		if cat == nil {
			cat = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(cat)
	case r.URL.Path == "/rest/api/3/search/approximate-count":
		f.count(w, r)
	case r.URL.Path == "/rest/api/3/search/jql":
		f.search(w, r)
	case strings.HasSuffix(r.URL.Path, "/changelog"):
		f.child(w, f.changelog, r.URL.Path, "/changelog")
	case strings.HasSuffix(r.URL.Path, "/comment"):
		f.child(w, f.comments, r.URL.Path, "/comment")
	case r.URL.Path == "/rest/api/3/filter/my":
		if f.filtersJSON != nil {
			w.Write(f.filtersJSON)
			return
		}
		w.Write([]byte(`[]`))
	case strings.HasPrefix(r.URL.Path, "/rest/api/3/project/") && strings.HasSuffix(r.URL.Path, "/versions"):
		f.versionHits++
		if f.versionsStatus != 0 {
			http.Error(w, `{"errorMessages":["versions boom"]}`, f.versionsStatus)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/rest/api/3/project/")
		key := strings.TrimSuffix(rest, "/versions")
		if unesc, err := url.PathUnescape(key); err == nil {
			key = unesc
		}
		if f.versions != nil {
			if body, ok := f.versions[key]; ok {
				w.Write([]byte(body))
				return
			}
		}
		w.Write([]byte(`[]`))
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.Error(w, "no", http.StatusNotFound)
	}
}

func (f *fakeSite) count(w http.ResponseWriter, r *http.Request) {
	var body struct {
		JQL string `json:"jql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Fatalf("bad count body: %v", err)
	}
	f.lastCountJQL = body.JQL
	if f.countFail {
		http.Error(w, `{"errorMessages":["count boom"]}`, http.StatusInternalServerError)
		return
	}
	// Approximate count = fixture size; good enough for progress-denominator tests.
	_ = json.NewEncoder(w).Encode(map[string]int{"count": len(f.issues)})
}

func (f *fakeSite) search(w http.ResponseWriter, r *http.Request) {
	var body struct {
		JQL           string   `json:"jql"`
		Fields        []string `json:"fields"`
		Expand        string   `json:"expand"`
		NextPageToken string   `json:"nextPageToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Fatalf("bad search body: %v", err)
	}
	f.allJQLs = append(f.allJQLs, body.JQL)
	offset, _ := strconv.Atoi(body.NextPageToken)
	keyOnly := body.Expand == "" // the reconcile pass asks for no changelog
	if keyOnly {
		f.keyOnlyRun++
	} else {
		f.syncJQL, f.syncFields = body.JQL, body.Fields
		if offset == f.failOffset {
			http.Error(w, `{"errorMessages":["boom"]}`, http.StatusInternalServerError)
			return
		}
	}

	end := min(offset+f.pageSize, len(f.issues))
	page := map[string]any{"issues": f.issues[min(offset, len(f.issues)):end]}
	if end < len(f.issues) {
		page["nextPageToken"] = strconv.Itoa(end)
	} else {
		page["isLast"] = true
	}
	json.NewEncoder(w).Encode(page)
}

func (f *fakeSite) child(w http.ResponseWriter, from map[string]string, path, suffix string) {
	key := strings.TrimSuffix(path, suffix)
	key = key[strings.LastIndex(key, "/")+1:]
	payload, ok := from[key]
	if !ok {
		f.t.Errorf("unexpected child fetch for %s%s", key, suffix)
		http.Error(w, "no", http.StatusNotFound)
		return
	}
	w.Write([]byte(payload))
}

// ── fixtures ──────────────────────────────────────────────────────────────────

// statusesJSON is the site status list. The same three ids carry different
// display names per language, which is the whole point: no logic may key on them.
func statusesJSON(lang string) []byte {
	out := []map[string]any{}
	for _, id := range []string{"1", "3", "5"} {
		out = append(out, statusObj(id, lang))
	}
	b, _ := json.Marshal(out)
	return b
}

func statusObj(id, lang string) map[string]any {
	names := map[string]string{
		"1en": "To Do", "3en": "In Progress", "5en": "Done",
		"1ko": "할 일", "3ko": "진행 중", "5ko": "완료",
	}
	cats := map[string]string{"1": "new", "3": "indeterminate", "5": "done"}
	return map[string]any{
		"id": id, "name": names[id+lang],
		"statusCategory": map[string]any{"key": cats[id]},
	}
}

func adfDoc(text string) map[string]any {
	return map[string]any{"type": "doc", "version": 1, "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}},
	}}
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixtures is three issues over two pages: one rich, one bare, one resolved.
func fixtures(lang string) []json.RawMessage {
	user := func(id, name string) map[string]any {
		return map[string]any{"accountId": id, "displayName": name, "emailAddress": name + "@example.com"}
	}
	nmb1 := map[string]any{
		"id": "10001", "key": "NMB-1",
		"fields": map[string]any{
			"summary":     "Editor crashes on save",
			"description": adfDoc("the flangewidget panics when saving"),
			"environment": adfDoc("macOS 15, build 4210"),
			"project":     map[string]any{"key": "NMB"},
			"issuetype":   map[string]any{"id": "10004", "name": "Bug"},
			"status":      statusObj("3", lang),
			"priority":    map[string]any{"id": "2", "name": "High"},
			"assignee":    user("acc-dana", "Dana"),
			"reporter":    user("acc-sam", "Sam"),
			"creator":     user("acc-sam", "Sam"),
			"parent":      map[string]any{"key": "NMB-100"},
			"labels":      []string{"regression", "editor"},
			"components":  []any{map[string]any{"name": "Core"}},
			"fixVersions": []any{map[string]any{"id": "10012", "name": "2026.8"}},
			"versions":    []any{map[string]any{"name": "2026.7"}},
			"duedate":     "2026-08-20",
			"created":     "2026-07-01T10:00:00.000+0900",
			"updated":     "2026-08-02T10:00:00.000+0900",
			"comment": map[string]any{"total": 2, "comments": []any{
				map[string]any{"id": "9001", "author": user("acc-dana", "Dana"),
					"body":    adfDoc("cannot reproduce with commentonlytoken enabled"),
					"created": "2026-07-02T10:00:00.000+0900", "updated": "2026-07-02T10:00:00.000+0900"},
				map[string]any{"id": "9002", "author": user("acc-sam", "Sam"),
					"body": adfDoc("still broken"), "created": "2026-07-03T10:00:00.000+0900"},
			}},
			"attachment": []any{map[string]any{"id": "7001", "filename": "crash.log",
				"mimeType": "text/plain", "size": 2048, "author": user("acc-sam", "Sam"),
				"created": "2026-07-02T10:00:00.000+0900"}},
			"issuelinks": []any{map[string]any{
				"type":         map[string]any{"name": "Relates"},
				"outwardIssue": map[string]any{"key": "NMB-2"}}},
			"customfield_10050": map[string]any{"value": "Sev1"},
			"customfield_10101": adfDoc("steps: open the reproseed sample"),
		},
		// Resolved, then reopened: derived fields must see both.
		"changelog": map[string]any{"total": 3, "histories": []any{
			history("h1", "2026-07-10T10:00:00.000+0900", "status", "1", "5", lang),
			history("h2", "2026-07-20T10:00:00.000+0900", "status", "5", "3", lang),
			map[string]any{"id": "h3", "created": "2026-07-21T10:00:00.000+0900",
				"author": user("acc-sam", "Sam"),
				"items": []any{map[string]any{"field": "Assignee", "fieldId": "assignee",
					"from": "", "fromString": "", "to": "acc-dana", "toString": "Dana"}}},
		}},
	}
	nmb2 := map[string]any{
		"id": "10002", "key": "NMB-2",
		"fields": map[string]any{
			"summary":   "Add keyboard shortcut",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10002", "name": "Task"},
			"status":    statusObj("1", lang),
			"reporter":  user("acc-sam", "Sam"),
			"created":   "2026-07-05T10:00:00.000+0900",
			"updated":   "2026-08-03T10:00:00.000+0900",
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	}
	nmb3 := map[string]any{
		"id": "10003", "key": "NMB-3",
		"fields": map[string]any{
			"summary":    "Crash on empty project",
			"project":    map[string]any{"key": "NMB"},
			"issuetype":  map[string]any{"id": "10004", "name": "Bug"},
			"status":     statusObj("5", lang),
			"priority":   map[string]any{"id": "1", "name": "Highest"},
			"resolution": map[string]any{"id": "10000", "name": "Done"},
			"reporter":   user("acc-dana", "Dana"),
			"created":    "2026-07-06T10:00:00.000+0900",
			"updated":    "2026-08-04T18:15:00.000+0900",
		},
		"changelog": map[string]any{"total": 1, "histories": []any{
			history("h9", "2026-08-04T18:15:00.000+0900", "status", "1", "5", lang),
		}},
	}
	out := []json.RawMessage{}
	for _, m := range []map[string]any{nmb1, nmb2, nmb3} {
		b, _ := json.Marshal(m)
		out = append(out, b)
	}
	return out
}

// history is one changelog entry. The display values are localized, the ids are
// not, which is what every derived rule keys on.
func history(id, at, field, fromID, toID, lang string) map[string]any {
	from, to := statusObj(fromID, lang), statusObj(toID, lang)
	return map[string]any{
		"id": id, "created": at,
		"author": map[string]any{"accountId": "acc-sam", "displayName": "Sam"},
		"items": []any{map[string]any{
			"field": "status", "fieldId": "status",
			"from": fromID, "fromString": from["name"],
			"to": toID, "toString": to["name"],
		}},
	}
}

// ── harness ───────────────────────────────────────────────────────────────────

func testConfig() *config.Config {
	return &config.Config{
		Site: "https://example.atlassian.net", Email: "someone@example.com", Token: "secret-token",
		Projects:   []string{"NMB"},
		FieldMap:   map[string]string{"severity": "customfield_10050"},
		BodyFields: []string{"customfield_10101"},
	}
}

func lite(t *testing.T, db *mirror, key string) store.IssueLite {
	t.Helper()
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lites {
		if l.IssueKey == key {
			return l
		}
	}
	t.Fatalf("%s is not in the mirror", key)
	return store.IssueLite{}
}

// mirror is the store plus the path, because a few assertions read columns the
// read API does not expose — data-model.md promises SQL access anyway.
type mirror struct {
	*store.DB
	path string
}

func newMirror(t *testing.T) *mirror {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &mirror{DB: db, path: path}
}

func (m *mirror) column(t *testing.T, table, col, key string) string {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+m.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var v *string
	if err := conn.QueryRow(`SELECT `+col+` FROM `+table+` WHERE key = ?`, key).Scan(&v); err != nil {
		t.Fatalf("%s.%s for %s: %v", table, col, key, err)
	}
	if v == nil {
		return ""
	}
	return *v
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestFullSyncMapsEverything(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	cfg := testConfig()

	res, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 3 || res.Changed != 3 || !res.Full {
		t.Fatalf("result = %+v", res)
	}
	lites, err := db.IssueLites(context.Background())
	if err != nil || len(lites) != 3 {
		t.Fatalf("mirror holds %d issues, err = %v", len(lites), err)
	}
	// Two pages of two, so pagination actually happened.
	if !strings.Contains(site.syncJQL, `project = "NMB" ORDER BY updated DESC`) {
		t.Errorf("full sync JQL = %q", site.syncJQL)
	}
	fields := strings.Join(site.syncFields, ",")
	for _, want := range []string{"summary", "customfield_10050", "customfield_10101"} {
		if !strings.Contains(fields, want) {
			t.Errorf("field list is missing %q: %s", want, fields)
		}
	}

	one := lite(t, db, "NMB-1")
	if one.Summary != "Editor crashes on save" || one.StatusCategory != "inprogress" {
		t.Errorf("NMB-1 = %+v", one)
	}
	if one.PriorityRank != 2 {
		t.Errorf("priority_rank = %d, want 2 (High is second in the site list)", one.PriorityRank)
	}
	if one.PriorityID != "2" {
		t.Errorf("priority_id = %q, want 2 (fixture High id)", one.PriorityID)
	}
	if got := db.column(t, "issues", "priority_id", "NMB-1"); got != "2" {
		t.Errorf("issues.priority_id = %q, want 2", got)
	}
	if one.ReopenCount != 1 || one.ReopenedAt == nil || *one.ReopenedAt != "2026-07-20T01:00:00.000Z" {
		t.Errorf("reopen = %d at %v", one.ReopenCount, one.ReopenedAt)
	}
	if one.ResolvedAt != nil {
		t.Errorf("resolved_at must be nil once reopened, got %v", *one.ResolvedAt)
	}
	if one.StatusChangedAt == nil || *one.StatusChangedAt != "2026-07-20T01:00:00.000Z" {
		t.Errorf("status_changed_at = %v", one.StatusChangedAt)
	}
	if one.CommentCount != 2 {
		t.Errorf("comment_count = %d", one.CommentCount)
	}
	// Parent is the direct link; epic_key is derived only when the parent chain
	// reaches a hierarchyLevel==1 issue in the mirror (NMB-100 is not mirrored).
	if one.ParentKey == nil || *one.ParentKey != "NMB-100" {
		t.Errorf("parent_key = %v", one.ParentKey)
	}
	if one.EpicKey != nil {
		t.Errorf("epic_key = %v, want nil without mirrored epic ancestor", one.EpicKey)
	}
	if strings.Join(one.Labels, ",") != "regression,editor" || strings.Join(one.FixVersions, ",") != "2026.8" {
		t.Errorf("labels = %v, fixVersions = %v", one.Labels, one.FixVersions)
	}
	if got := db.column(t, "issues", "fix_version_ids", "NMB-1"); got != `["10012"]` {
		t.Errorf("fix_version_ids = %q, want [\"10012\"] (same order as names)", got)
	}
	if one.AssigneeEmail == nil || *one.AssigneeEmail != "Dana@example.com" {
		t.Errorf("assignee_email = %v", one.AssigneeEmail)
	}
	// The reporter's email is what the client's reporter filters key on, so it has
	// to be mapped like the assignee's rather than left to the account id.
	if one.ReporterEmail == nil || *one.ReporterEmail != "Sam@example.com" {
		t.Errorf("reporter_email = %v", one.ReporterEmail)
	}
	if one.UpdatedAt == nil || *one.UpdatedAt != "2026-08-02T01:00:00.000Z" {
		t.Errorf("updated_at should be normalized to UTC, got %v", one.UpdatedAt)
	}

	three := lite(t, db, "NMB-3")
	if three.StatusCategory != "done" || three.ResolvedAt == nil {
		t.Errorf("NMB-3 = %+v", three)
	}
	if three.ResolutionID != "10000" {
		t.Errorf("NMB-3 resolution_id = %q, want 10000 (fixture Done id)", three.ResolutionID)
	}
	if got := db.column(t, "issues", "resolution_id", "NMB-3"); got != "10000" {
		t.Errorf("issues.resolution_id = %q, want 10000", got)
	}

	// Configured custom field lands in issues.custom under its alias.
	if got := db.column(t, "issues", "custom", "NMB-1"); !strings.Contains(got, `"severity"`) || !strings.Contains(got, "Sev1") {
		t.Errorf("custom = %s", got)
	}
	if got := db.column(t, "issues", "environment_text", "NMB-1"); got != "macOS 15, build 4210" {
		t.Errorf("environment_text = %q", got)
	}
	if got := db.column(t, "items", "url", "NMB-1"); !strings.HasSuffix(got, "/browse/NMB-1") {
		t.Errorf("url = %q", got)
	}

	detail, err := db.Detail(context.Background(), "NMB-1")
	if err != nil || detail == nil {
		t.Fatalf("detail err = %v", err)
	}
	if len(detail.Comments) != 2 || len(detail.Attachments) != 1 || len(detail.History) != 3 || len(detail.LinkedIssues) != 1 {
		t.Errorf("detail: %d comments, %d attachments, %d history, %d links",
			len(detail.Comments), len(detail.Attachments), len(detail.History), len(detail.LinkedIssues))
	}
	if detail.LinkedIssues[0].Key != "NMB-2" || detail.LinkedIssues[0].Direction != "outward" {
		t.Errorf("link = %+v", detail.LinkedIssues[0])
	}
	if len(detail.DescriptionADF) == 0 {
		t.Error("description_adf must be stored raw for the UI to render")
	}

	// body_text carries the description, the configured body field and comment
	// text into FTS.
	for _, term := range []string{"flangewidget", "reproseed", "commentonlytoken"} {
		hits, err := db.Search(context.Background(), term, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits.Keys) != 1 || hits.Keys[0] != "NMB-1" {
			t.Errorf("search %q = %v", term, hits.Keys)
		}
	}

	state, _ := db.SyncState(context.Background(), SourceID)
	if state.Watermark != "2026-08-04T18:15:00.000+0900" {
		t.Errorf("watermark = %q, want the newest updated seen", state.Watermark)
	}
	if state.LastError != nil {
		t.Errorf("last_error = %v", *state.LastError)
	}
}

// TestFixVersionIDsIngest is FAIL-first for GDK-532: the issue payload already
// carries NamedID{id,name} on fixVersions, and the mirror must store both the
// name array (fix_versions, the 0.x recipe key) and the same-order id array.
func TestFixVersionIDsIngest(t *testing.T) {
	site := newSite(t, "en")
	site.issues = []json.RawMessage{raw(t, map[string]any{
		"id": "10001", "key": "NMB-1",
		"fields": map[string]any{
			"summary":   "version ids must survive ingest",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10004", "name": "Bug"},
			"status":    statusObj("3", "en"),
			"created":   "2026-07-01T10:00:00.000Z",
			"updated":   "2026-08-02T10:00:00.000Z",
			"fixVersions": []any{
				map[string]any{"id": "10012", "name": "v2.5"},
				map[string]any{"id": "10013", "name": "v2.6"},
			},
		},
	})}
	db := newMirror(t)
	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}
	if got := db.column(t, "issues", "fix_versions", "NMB-1"); got != `["v2.5","v2.6"]` {
		t.Errorf("fix_versions = %q, want names in payload order", got)
	}
	if got := db.column(t, "issues", "fix_version_ids", "NMB-1"); got != `["10012","10013"]` {
		t.Errorf("fix_version_ids = %q, want ids in the same order as names", got)
	}
}

func TestVersionCatalogUpsertAndPrune(t *testing.T) {
	site := newSite(t, "en")
	site.versions = map[string]string{
		"NMB": `[{"id":"10012","name":"v2.5","released":true,"archived":false,"releaseDate":"2026-08-20"},{"id":"10013","name":"v2.6","released":false,"archived":true}]`,
	}
	db := newMirror(t)
	client := site.start()
	cfg := testConfig()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if n := versionCount(t, db); n != 2 {
		t.Errorf("versions after re-full = %d, want 2 (no duplicates)", n)
	}
	site.versions["NMB"] = `[{"id":"10012","name":"v2.5","released":true,"archived":false,"releaseDate":"2026-08-20"}]`
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if n := versionCount(t, db); n != 1 {
		t.Errorf("versions after prune = %d, want 1", n)
	}
	if id := versionIDs(t, db); id != "10012" {
		t.Errorf("kept ids = %q, want 10012 (10013 left the catalog)", id)
	}
}

func TestVersionCatalogGetFailureContinues(t *testing.T) {
	site := newSite(t, "en")
	site.versionsStatus = http.StatusInternalServerError
	db := newMirror(t)
	var logs []string
	res, err := Run(context.Background(), testConfig(), db.DB, Options{
		Full: true, Client: site.start(),
		Log: func(s string) { logs = append(logs, s) },
	})
	if err != nil {
		t.Fatalf("catalog GET failure must not fail the issue pass: %v", err)
	}
	if res.Changed == 0 {
		t.Error("issue pass wrote nothing; catalog failure aborted ingest")
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "versions:") || !strings.Contains(joined, "skipped") {
		t.Errorf("want a versions skipped warning, logs = %v", logs)
	}
	if n := versionCount(t, db); n != 0 {
		t.Errorf("versions rows = %d, want 0 after a failed catalog GET", n)
	}
}

func TestVersionCatalogSkippedOnIncremental(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	client := site.start()
	cfg := testConfig()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	hits := site.versionHits
	if hits == 0 {
		t.Fatal("full sync did not fetch the version catalog")
	}
	if _, err := Run(context.Background(), cfg, db.DB, Options{Client: client}); err != nil {
		t.Fatal(err)
	}
	if site.versionHits != hits {
		t.Errorf("incremental tick fetched versions (%d → %d); catalog is full/reconcile only", hits, site.versionHits)
	}
}

func versionCount(t *testing.T, db *mirror) int {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM versions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func versionIDs(t *testing.T, db *mirror) string {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var ids []string
	rows, err := conn.Query(`SELECT id FROM versions ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return strings.Join(ids, ",")
}

func TestImportFiltersCompilesJQLIntoSourceQueries(t *testing.T) {
	site := newSite(t, "en")
	site.filtersJSON = []byte(`[
		{"id":"10000","name":"Open in NMB","jql":"project = NMB AND statusCategory = \"In Progress\"","favourite":true,
		 "owner":{"displayName":"Dana"}},
		{"id":"10001","name":"Sprint board","jql":"project = NMB AND sprint in openSprints()","favourite":false,
		 "owner":{"displayName":"Dana"}}
	]`)
	db := newMirror(t)
	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}
	got, err := db.SourceQueries(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("source_queries %d", len(got))
	}
	// Starred first.
	if got[0].Name != "Open in NMB" || !got[0].Favourite {
		t.Fatalf("first %+v", got[0])
	}
	var cfg struct {
		Filters struct {
			JiraProject    []string `json:"jira_project"`
			StatusCategory []string `json:"status_category"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(got[0].Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Filters.JiraProject) != 1 || cfg.Filters.JiraProject[0] != "NMB" {
		t.Fatalf("compiled project %+v", cfg.Filters.JiraProject)
	}
	if len(cfg.Filters.StatusCategory) != 1 || cfg.Filters.StatusCategory[0] != "inprogress" {
		t.Fatalf("compiled category %+v", cfg.Filters.StatusCategory)
	}
	// GDK-518: sprint in openSprints() is in the subset; project still applied.
	if got[1].Name != "Sprint board" {
		t.Fatalf("second %q", got[1].Name)
	}
	joined := strings.Join(got[1].Applied, " ")
	if !strings.Contains(joined, "project") || !strings.Contains(joined, "sprint") {
		t.Fatalf("applied %v, want project and sprint", got[1].Applied)
	}
	for _, u := range got[1].Unsupported {
		if strings.Contains(strings.ToLower(u), "sprint") {
			t.Fatalf("sprint still unsupported: %v", got[1].Unsupported)
		}
	}
}

func TestIncrementalRerunIsANoOp(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	cfg := testConfig()
	client := site.start()

	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	before, _ := db.SyncState(context.Background(), SourceID)

	res, err := Run(context.Background(), cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Error("a mirror with a watermark must sync incrementally")
	}
	if res.Fetched != 3 || res.Changed != 0 {
		t.Errorf("re-run changed %d of %d fetched rows, want 0", res.Changed, res.Fetched)
	}
	after, _ := db.SyncState(context.Background(), SourceID)
	if after.Version != before.Version {
		t.Errorf("version moved %d -> %d on an unchanged re-run; the ETag would break", before.Version, after.Version)
	}
	if after.Watermark != before.Watermark {
		t.Errorf("watermark moved %q -> %q", before.Watermark, after.Watermark)
	}
	// The overlap window is two minutes back, expressed in the offset Jira
	// itself stamps on `updated` (JQL reads a bare timestamp in account time).
	if !strings.Contains(site.syncJQL, `updated >= "2026/08/04 18:13"`) {
		t.Errorf("incremental JQL = %q", site.syncJQL)
	}
	if !strings.Contains(site.syncJQL, `project in ("NMB")`) || !strings.Contains(site.syncJQL, "ORDER BY updated ASC") {
		t.Errorf("incremental JQL = %q", site.syncJQL)
	}
}

func TestFailedPageKeepsMirrorAndWatermark(t *testing.T) {
	site := newSite(t, "en")
	site.failOffset = 2 // page one commits, page two dies
	db := newMirror(t)
	cfg := testConfig()
	client := site.start()

	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err == nil {
		t.Fatal("expected the injected 500 to fail the run")
	}
	lites, _ := db.IssueLites(context.Background())
	if len(lites) != 2 {
		t.Errorf("committed pages must survive a later failure, mirror holds %d", len(lites))
	}
	state, _ := db.SyncState(context.Background(), SourceID)
	if state.Watermark != "" {
		t.Errorf("watermark advanced to %q on a failed run", state.Watermark)
	}
	if state.LastError == nil || !strings.Contains(*state.LastError, "500") {
		t.Errorf("last_error = %v", state.LastError)
	}

	// Recovery: no watermark means the next run is a full one and it completes.
	site.failOffset = -1
	res, err := Run(context.Background(), cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Full {
		t.Error("a mirror without a watermark must resync fully")
	}
	state, _ = db.SyncState(context.Background(), SourceID)
	if state.LastError != nil || state.Watermark == "" {
		t.Errorf("state after recovery = %+v", state)
	}
	if lites, _ = db.IssueLites(context.Background()); len(lites) != 3 {
		t.Errorf("mirror holds %d issues after recovery", len(lites))
	}
}

func TestReconcileDeletesVanishedKeys(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	cfg := testConfig()
	client := site.start()

	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if site.keyOnlyRun == 0 {
		t.Error("a full sync must run the reconcile pass")
	}

	site.issues = site.issues[:1] // NMB-2 and NMB-3 left scope upstream
	res, err := Run(context.Background(), cfg, db.DB, Options{Reconcile: true, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 2 {
		t.Fatalf("deleted %d, want 2", res.Deleted)
	}
	lites, _ := db.IssueLites(context.Background())
	if len(lites) != 1 || lites[0].IssueKey != "NMB-1" {
		t.Errorf("mirror = %+v", lites)
	}
	gone, err := db.DeletedKeysSince(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(gone) // same deleted_at, so the order within one pass is arbitrary
	if strings.Join(gone, ",") != "NMB-2,NMB-3" {
		t.Errorf("tombstones = %v", gone)
	}

	// An upstream that answers with nothing is a scoping failure, not proof that
	// every issue was deleted.
	site.issues = nil
	if _, err := Run(context.Background(), cfg, db.DB, Options{Reconcile: true, Client: client}); err == nil {
		t.Fatal("expected reconcile to refuse an empty upstream")
	}
	if lites, _ = db.IssueLites(context.Background()); len(lites) != 1 {
		t.Errorf("mirror emptied by an empty upstream: %d rows left", len(lites))
	}
}

func TestDerivedFieldsIgnoreDisplayLanguage(t *testing.T) {
	type snapshot struct {
		category, status                                        string
		reopen                                                  int
		reopenedAt, resolvedAt, statusChangedAt, assigneeChange string
	}
	take := func(lang string) map[string]snapshot {
		site := newSite(t, lang)
		db := newMirror(t)
		if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: site.start()}); err != nil {
			t.Fatal(err)
		}
		out := map[string]snapshot{}
		lites, _ := db.IssueLites(context.Background())
		for _, l := range lites {
			s := snapshot{category: l.StatusCategory, status: l.Status, reopen: l.ReopenCount}
			if l.ReopenedAt != nil {
				s.reopenedAt = *l.ReopenedAt
			}
			if l.ResolvedAt != nil {
				s.resolvedAt = *l.ResolvedAt
			}
			if l.StatusChangedAt != nil {
				s.statusChangedAt = *l.StatusChangedAt
			}
			s.assigneeChange = db.column(t, "issues", "assignee_changed_at", l.IssueKey)
			out[l.IssueKey] = s
		}
		return out
	}
	en, ko := take("en"), take("ko")
	if en["NMB-1"].status == ko["NMB-1"].status {
		t.Fatalf("fixture is not localized: both runs report status %q", en["NMB-1"].status)
	}
	for key, want := range en {
		got := ko[key]
		want.status, got.status = "", "" // the display name is the one thing allowed to differ
		if got != want {
			t.Errorf("%s: korean run derived %+v, english run %+v", key, got, want)
		}
	}
	if en["NMB-1"].reopen != 1 || en["NMB-1"].assigneeChange == "" {
		t.Errorf("NMB-1 derived %+v", en["NMB-1"])
	}
}

func TestTruncatedChildrenArePaged(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	// NMB-2 claims more history and more comments than it inlines, so sync has to
	// fetch both from the dedicated endpoints.
	site.issues[1] = raw(t, map[string]any{
		"id": "10002", "key": "NMB-2",
		"fields": map[string]any{
			"summary": "Long history", "project": map[string]any{"key": "NMB"},
			"status": statusObj("1", "en"), "created": "2026-07-05T10:00:00.000+0900",
			"updated": "2026-08-03T10:00:00.000+0900",
			"comment": map[string]any{"total": 3, "maxResults": 1, "comments": []any{
				map[string]any{"id": "9101", "body": adfDoc("inlined only")},
			}},
		},
		"changelog": map[string]any{"total": 4, "maxResults": 1, "histories": []any{
			history("h20", "2026-07-01T10:00:00.000+0900", "status", "1", "3", "en"),
		}},
	})
	site.changelog["NMB-2"] = string(raw(t, map[string]any{"total": 4, "isLast": true, "values": []any{
		history("h20", "2026-07-01T10:00:00.000+0900", "status", "1", "3", "en"),
		history("h21", "2026-07-02T10:00:00.000+0900", "status", "3", "5", "en"),
		history("h22", "2026-07-03T10:00:00.000+0900", "status", "5", "1", "en"),
		history("h23", "2026-07-04T10:00:00.000+0900", "status", "1", "3", "en"),
	}}))
	site.comments["NMB-2"] = string(raw(t, map[string]any{"total": 3, "comments": []any{
		map[string]any{"id": "9101", "body": adfDoc("first")},
		map[string]any{"id": "9102", "body": adfDoc("second")},
		map[string]any{"id": "9103", "body": adfDoc("third")},
	}}))

	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}
	two := lite(t, db, "NMB-2")
	if two.CommentCount != 3 {
		t.Errorf("comment_count = %d, want 3 from the paged fetch", two.CommentCount)
	}
	// The reopen (5 -> 1) exists only in the paged changelog.
	if two.ReopenCount != 1 || two.ReopenedAt == nil {
		t.Errorf("reopen = %d at %v, want 1 from the paged changelog", two.ReopenCount, two.ReopenedAt)
	}
	detail, _ := db.Detail(context.Background(), "NMB-2")
	if len(detail.History) != 4 {
		t.Errorf("history rows = %d, want 4", len(detail.History))
	}
}

func TestWatchRunsImmediatelyAndStopsWithContext(t *testing.T) {
	site := newSite(t, "en")
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 1

	ctx, cancel := context.WithCancel(context.Background())
	passes := make(chan struct{}, 4)
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{Full: true, Client: client, Log: func(line string) {
			if strings.HasPrefix(line, "done:") {
				select {
				case passes <- struct{}{}:
				default:
				}
			}
		}})
	}()

	<-passes // the first pass must not wait for a tick
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
	if lites, _ := db.IssueLites(context.Background()); len(lites) != 3 {
		t.Errorf("watch mirrored %d issues", len(lites))
	}
}

func TestRunRejectsMissingConfig(t *testing.T) {
	db := newMirror(t)
	if _, err := Run(context.Background(), &config.Config{}, db.DB, Options{}); err == nil {
		t.Error("expected a missing credential to fail before any request")
	}
}

// TestEmptyProjectsJQL proves empty cfg.Projects never emits `project in ()`
// and builds the three documented unscoped clauses.
func TestEmptyProjectsJQL(t *testing.T) {
	// String contracts (no network).
	if got := fullJQL(""); got != "ORDER BY updated DESC" {
		t.Errorf("full empty = %q", got)
	}
	if got := fullJQL("AAA"); got != `project = "AAA" ORDER BY updated DESC` {
		t.Errorf("full one = %q", got)
	}
	incEmpty := incrementalJQL(nil, "2026-08-05T14:54:00.000+0000")
	if incEmpty != `updated >= "2026/08/05 14:52" ORDER BY updated ASC` {
		t.Errorf("incremental empty = %q", incEmpty)
	}
	if strings.Contains(incEmpty, "project") {
		t.Errorf("incremental empty must not mention project: %q", incEmpty)
	}
	incOne := incrementalJQL([]string{"AAA"}, "2026-08-05T14:54:00.000+0000")
	if !strings.Contains(incOne, `project in ("AAA")`) || strings.Contains(incOne, "project in ()") {
		t.Errorf("incremental one = %q", incOne)
	}
	if got := reconcileJQL(nil); got != "ORDER BY created ASC" {
		t.Errorf("reconcile empty = %q", got)
	}
	if got := reconcileJQL([]string{"AAA", "BBB"}); got != `project in ("AAA", "BBB") ORDER BY created ASC` {
		t.Errorf("reconcile multi = %q", got)
	}
	if strings.Contains(reconcileJQL(nil), "project in ()") || strings.Contains(reconcileJQL([]string{}), "project in ()") {
		t.Error("reconcile empty must not emit project in ()")
	}

	// End-to-end: empty projects full sync + reconcile hit the unscoped JQLs.
	site := newSite(t, "en")
	db := newMirror(t)
	cfg := testConfig()
	cfg.Projects = nil
	res, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 3 {
		t.Fatalf("fetched %d, want 3", res.Fetched)
	}
	if site.syncJQL != "ORDER BY updated DESC" {
		t.Errorf("full search JQL = %q", site.syncJQL)
	}
	var sawReconcile bool
	for _, j := range site.allJQLs {
		if strings.Contains(j, "project in ()") {
			t.Errorf("empty project clause leaked: %q", j)
		}
		if j == "ORDER BY created ASC" {
			sawReconcile = true
		}
	}
	if !sawReconcile {
		t.Errorf("reconcile JQL missing; all = %v", site.allJQLs)
	}

	// Incremental with empty projects after a watermark is set.
	site2 := newSite(t, "en")
	client := site2.start()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Client: client}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(site2.syncJQL, "updated >=") || !strings.HasSuffix(site2.syncJQL, "ORDER BY updated ASC") {
		t.Errorf("incremental empty JQL = %q", site2.syncJQL)
	}
	if strings.Contains(site2.syncJQL, "project") {
		t.Errorf("incremental empty must drop project filter: %q", site2.syncJQL)
	}
}

func TestFormatCount(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		999:     "999",
		1000:    "1,000",
		6543:    "6,543",
		1234567: "1,234,567",
	}
	for n, want := range cases {
		if got := formatCount(n); got != want {
			t.Errorf("formatCount(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestCountFailureStillSyncs: approximate-count 500 must not fail the run, and
// page progress lines must omit the denominator.
func TestCountFailureStillSyncs(t *testing.T) {
	site := newSite(t, "en")
	site.countFail = true
	db := newMirror(t)
	var logs []string
	res, err := Run(context.Background(), testConfig(), db.DB, Options{
		Full:   true,
		Client: site.start(),
		Log:    func(s string) { logs = append(logs, s) },
	})
	if err != nil {
		t.Fatalf("Run failed when Count failed: %v", err)
	}
	if res.Fetched != 3 {
		t.Fatalf("fetched %d, want 3", res.Fetched)
	}
	joined := strings.Join(logs, "\n")
	if strings.Contains(joined, "about ") {
		t.Errorf("start line must omit about-count when Count fails:\n%s", joined)
	}
	if strings.Contains(joined, " / ") {
		t.Errorf("page lines must omit denominator when Count fails:\n%s", joined)
	}
	// Page lines look like "  N issues" (two leading spaces).
	sawBare := false
	for _, line := range logs {
		if strings.HasPrefix(line, "  ") && strings.HasSuffix(line, " issues") && !strings.Contains(line, "/") {
			sawBare = true
			break
		}
	}
	if !sawBare {
		t.Errorf("expected bare progress lines; logs:\n%s", joined)
	}
	if !strings.HasPrefix(logs[0], "full sync: NMB") {
		t.Errorf("first log = %q", logs[0])
	}
	// done line still present with duration.
	last := logs[len(logs)-1]
	if !strings.HasPrefix(last, "done: ") || !strings.Contains(last, " in ") {
		t.Errorf("done line = %q", last)
	}
}

func TestRunFlushesAPIUsage(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	c := site.start()
	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: c}); err != nil {
		t.Fatal(err)
	}
	days, err := db.APIUsage(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 {
		t.Fatalf("api_usage rows = %d, want 1", len(days))
	}
	if days[0].Requests < 1 {
		t.Errorf("requests = %d, want at least the status/priority/search calls", days[0].Requests)
	}
	// Client counters were taken by the flush.
	if u := c.Usage(); u.Requests != 0 {
		t.Errorf("client still holds %d requests after flush", u.Requests)
	}
}

func TestFlushAPIUsageFailureDoesNotPropagate(t *testing.T) {
	// Closed DB makes AddAPIUsage fail; flush must log and return without panic
	// so a broken instrumentation path cannot fail the sync that produced traffic.
	db := newMirror(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"1","name":"Highest"}]`))
	}))
	t.Cleanup(srv.Close)
	c := jira.New(srv.URL, "a@b.c", "tok")
	c.Retries, c.Backoff = 1, 0
	if _, err := c.Priorities(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Usage().Requests < 1 {
		t.Fatal("setup: expected at least one counted request")
	}
	_ = db.Close()
	var logs []string
	FlushAPIUsage(context.Background(), db.DB, c, func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	if len(logs) != 1 || !strings.Contains(logs[0], "api usage flush") {
		t.Fatalf("expected one flush log line, got %v", logs)
	}
	// Counters were still taken (we do not want to re-flush forever on a bad DB).
	if c.Usage().Requests != 0 {
		t.Errorf("TakeUsage should still zero the client: %+v", c.Usage())
	}
}

// TestSyncRunKindString is the pure label contract for SyncRun.Kind — same
// strings the pre-unification Jira defer produced (full|incremental, optional
// +reconcile). Kept as a unit so a future skeleton edit fails loudly.
func TestSyncRunKindString(t *testing.T) {
	cases := []struct {
		full, reconcile bool
		want            string
	}{
		{false, false, "incremental"},
		{true, false, "full"},
		{false, true, "incremental+reconcile"},
		{true, true, "full+reconcile"},
	}
	for _, tc := range cases {
		if got := syncRunKind(tc.full, tc.reconcile); got != tc.want {
			t.Errorf("syncRunKind(%v, %v) = %q, want %q", tc.full, tc.reconcile, got, tc.want)
		}
	}
}

// TestJiraSyncRunKinds proves the live Jira Run path still stamps the same
// SyncRun.Kind values as before the runSource unification:
//
//	full            → "full+reconcile"  (full always runs reconcile)
//	incremental     → "incremental"     (when something changed so the run is kept)
//	+Reconcile flag → "incremental+reconcile"
func TestJiraSyncRunKinds(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	cfg := testConfig()
	client := site.start()

	// 1. Full → full+reconcile
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	runs, err := db.SyncRuns(context.Background(), SourceID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) < 1 {
		t.Fatal("expected at least one SyncRun after full")
	}
	if runs[0].Kind != "full+reconcile" {
		t.Fatalf("full kind = %q, want full+reconcile", runs[0].Kind)
	}
	t.Logf("jira full SyncRun.Kind = %q", runs[0].Kind)

	// 2. Incremental that mutates a mirrored issue → incremental
	// Bump NMB-1's updated so the incremental search window picks it up as a change.
	raw := []byte(site.issues[0])
	// Fixture issues are JSON objects; patch "updated" to a later stamp.
	patched := strings.Replace(string(raw),
		`"updated":"2026-08-02T10:00:00.000+0900"`,
		`"updated":"2026-08-05T12:00:00.000+0900"`, 1)
	if patched == string(raw) {
		// Fallback: try the common en fixture stamp if the first pattern missed.
		patched = strings.Replace(string(raw),
			`"updated": "2026-08-02T10:00:00.000+0900"`,
			`"updated": "2026-08-05T12:00:00.000+0900"`, 1)
	}
	site.issues[0] = json.RawMessage(patched)

	res, err := Run(context.Background(), cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Fatal("expected incremental, got full")
	}
	runs, err = db.SyncRuns(context.Background(), SourceID, 5)
	if err != nil {
		t.Fatal(err)
	}
	// Newest first; want an incremental entry (may still be full+reconcile only
	// if changed=0 and the run was skipped — fail loudly if so).
	var sawInc bool
	for _, r := range runs {
		t.Logf("jira SyncRun.Kind after incremental = %q (changed path)", r.Kind)
		if r.Kind == "incremental" {
			sawInc = true
			break
		}
	}
	if !sawInc {
		// Incremental with changes should always record. Dump for diagnosis.
		t.Fatalf("no incremental SyncRun; runs=%v res.Changed=%d", kindsOf(runs), res.Changed)
	}

	// 3. Reconcile-flagged incremental that deletes → incremental+reconcile
	site.issues = site.issues[:1]
	res, err = Run(context.Background(), cfg, db.DB, Options{Reconcile: true, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted < 1 && res.Changed < 1 {
		t.Fatalf("reconcile run left no mark (deleted=%d changed=%d); kind would be skipped", res.Deleted, res.Changed)
	}
	runs, err = db.SyncRuns(context.Background(), SourceID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].Kind != "incremental+reconcile" {
		t.Fatalf("reconcile kind = %q, want incremental+reconcile (runs=%v)", runs[0].Kind, kindsOf(runs))
	}
	t.Logf("jira reconcile SyncRun.Kind = %q", runs[0].Kind)
}

func kindsOf(runs []store.SyncRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.Kind
	}
	return out
}

// TestKoreanChangelogWithoutFieldIDRecordsStatus is FAIL-first for C10: a
// Korean-account changelog item with no fieldId must still store field=status
// so Derive sees the reopen.
func TestKoreanChangelogWithoutFieldIDRecordsStatus(t *testing.T) {
	issue := map[string]any{
		"id": "20001", "key": "NMB-KO",
		"fields": map[string]any{
			"summary":   "korean changelog",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10004", "name": "버그"},
			"status":    statusObj("1", "ko"),
			"reporter":  map[string]any{"accountId": "acc-sam", "displayName": "Sam", "emailAddress": "sam@example.com"},
			"created":   "2026-07-01T10:00:00.000+0900",
			"updated":   "2026-08-04T10:00:00.000+0900",
		},
		"changelog": map[string]any{"total": 2, "histories": []any{
			map[string]any{
				"id": "hk1", "created": "2026-07-10T10:00:00.000+0900",
				"author": map[string]any{"accountId": "acc-sam", "displayName": "Sam"},
				"items": []any{map[string]any{
					"field": "상태", "from": "1", "fromString": "할 일",
					"to": "5", "toString": "완료",
				}},
			},
			map[string]any{
				"id": "hk2", "created": "2026-08-01T10:00:00.000+0900",
				"author": map[string]any{"accountId": "acc-sam", "displayName": "Sam"},
				"items": []any{map[string]any{
					"field": "상태", "from": "5", "fromString": "완료",
					"to": "1", "toString": "할 일",
				}},
			},
		}},
	}
	b, err := json.Marshal(issue)
	if err != nil {
		t.Fatal(err)
	}
	site := &fakeSite{t: t, lang: "ko", issues: []json.RawMessage{b}, pageSize: 10, failOffset: -1,
		changelog: map[string]string{}, comments: map[string]string{}}
	db := newMirror(t)
	cfg := testConfig()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}

	var field string
	if err := db.raw(t).QueryRow(`SELECT field FROM changelog WHERE id = 'jira:hk2:0'`).Scan(&field); err != nil {
		t.Fatalf("changelog row: %v", err)
	}
	if field != "status" {
		t.Errorf("C10: stored field = %q, want status (localized 상태 must not be stored)", field)
	}
	one := lite(t, db, "NMB-KO")
	if one.ReopenCount != 1 {
		t.Errorf("C10: reopen_count = %d, want 1 (Derive missed 상태)", one.ReopenCount)
	}
}

// TestLocalizedResolutionKeysOnID is FAIL-first for GDK-520: a Korean-account
// resolution name (`완료`) must be findable by the stable id, and the English
// display name `Done` must match nothing.
func TestLocalizedResolutionKeysOnID(t *testing.T) {
	issue := map[string]any{
		"id": "20002", "key": "NMB-RES",
		"fields": map[string]any{
			"summary":    "localized resolution",
			"project":    map[string]any{"key": "NMB"},
			"issuetype":  map[string]any{"id": "10004", "name": "버그"},
			"status":     statusObj("5", "ko"),
			"resolution": map[string]any{"id": "10000", "name": "완료"},
			"reporter":   map[string]any{"accountId": "acc-sam", "displayName": "Sam", "emailAddress": "sam@example.com"},
			"created":    "2026-07-01T10:00:00.000+0900",
			"updated":    "2026-08-04T10:00:00.000+0900",
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	}
	b, err := json.Marshal(issue)
	if err != nil {
		t.Fatal(err)
	}
	site := &fakeSite{t: t, lang: "ko", issues: []json.RawMessage{b}, pageSize: 10, failOffset: -1,
		changelog: map[string]string{}, comments: map[string]string{}}
	db := newMirror(t)
	cfg := testConfig()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}

	conn := db.raw(t)
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM issues_full WHERE resolution_id = '10000'`).Scan(&n); err != nil {
		t.Fatalf("WHERE resolution_id = '10000': %v", err)
	}
	if n != 1 {
		t.Errorf("resolution_id = 10000 matched %d rows, want 1", n)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM issues_full WHERE resolution = 'Done'`).Scan(&n); err != nil {
		t.Fatalf("WHERE resolution = 'Done': %v", err)
	}
	if n != 0 {
		t.Errorf("resolution = 'Done' matched %d rows, want 0 (name is localized to 완료)", n)
	}
	if got := db.column(t, "issues", "resolution", "NMB-RES"); got != "완료" {
		t.Errorf("resolution display name = %q, want 완료", got)
	}
}

// GDK-485: last_error for a paired unreachable origin must start with the
// folded pairing sentence, not the atlhttp "GET /rest/api/3/status: …" prefix.
func TestPairedUnreachableLastErrorIsFolded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	if err := pairing.SaveRemote(home, pairing.Remote{Endpoint: url, Token: "pair-token", Label: "laptop"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.Retries, c.Backoff = 1, 0

	db := newMirror(t)
	_, runErr := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: c})
	if runErr == nil {
		t.Fatal("sync against a closed home serve must fail")
	}
	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == nil || *st.LastError == "" {
		t.Fatalf("last_error empty after paired unreachable (run err %v)", runErr)
	}
	got := *st.LastError
	if strings.HasPrefix(got, "GET ") || strings.HasPrefix(got, "POST ") {
		t.Fatalf("last_error kept the REST prefix: %q", got)
	}
	if !strings.HasPrefix(got, "pairing:") {
		t.Fatalf("last_error = %q, want a pairing: sentence first", got)
	}
	if !strings.Contains(got, "cannot reach the home serve") {
		t.Fatalf("last_error = %q, want the unreachable sentence", got)
	}
}

func TestChangelogFieldIDPrefersStableID(t *testing.T) {
	got := changelogField(jira.HistoryItem{Field: "상태", FieldID: "status"})
	if got != "status" {
		t.Errorf("fieldId present: got %q", got)
	}
	got = changelogField(jira.HistoryItem{Field: "상태"})
	if got != "status" {
		t.Errorf("korean fallback: got %q, want status", got)
	}
	got = changelogField(jira.HistoryItem{Field: "Status"})
	if got != "status" {
		t.Errorf("english fallback: got %q, want status", got)
	}
	got = changelogField(jira.HistoryItem{Field: "커스텀", FieldID: "customfield_9"})
	if got != "customfield_9" {
		t.Errorf("custom fieldId: got %q", got)
	}
}

// TestCommentVisibilityAndJsdPublicMapped is FAIL-first for GDK-511: a Jira
// comment carrying visibility.role and jsdPublic:false must land in the three
// comments columns; a comment that omitted both keys stores ”/”/NULL.
func TestCommentVisibilityAndJsdPublicMapped(t *testing.T) {
	author := map[string]any{"accountId": "acc-dana", "displayName": "Dana", "emailAddress": "dana@example.com"}
	issue := map[string]any{
		"id": "51001", "key": "NMB-511",
		"fields": map[string]any{
			"summary":   "comment visibility",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10004", "name": "Bug"},
			"status":    statusObj("1", "en"),
			"created":   "2026-07-01T10:00:00.000+0900",
			"updated":   "2026-08-04T10:00:00.000+0900",
			"comment": map[string]any{"total": 2, "comments": []any{
				map[string]any{
					"id": "c-vis", "author": author,
					"body":    adfDoc("restricted internal"),
					"created": "2026-07-02T10:00:00.000+0900", "updated": "2026-07-02T10:00:00.000+0900",
					"visibility": map[string]any{"type": "role", "value": "Administrators"},
					"jsdPublic":  false,
				},
				map[string]any{
					"id": "c-pub", "author": author,
					"body":    adfDoc("open comment"),
					"created": "2026-07-03T10:00:00.000+0900", "updated": "2026-07-03T10:00:00.000+0900",
				},
			}},
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	}
	b, err := json.Marshal(issue)
	if err != nil {
		t.Fatal(err)
	}
	site := &fakeSite{t: t, lang: "en", issues: []json.RawMessage{b}, pageSize: 10, failOffset: -1,
		changelog: map[string]string{}, comments: map[string]string{}}
	db := newMirror(t)
	cfg := testConfig()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}

	conn := db.raw(t)
	var visType, visValue string
	var jsd sql.NullInt64
	if err := conn.QueryRow(`
		SELECT visibility_type, visibility_value, jsd_public
		FROM comments WHERE external_id = 'c-vis'`).Scan(&visType, &visValue, &jsd); err != nil {
		t.Fatalf("restricted comment columns: %v", err)
	}
	if visType != "role" || visValue != "Administrators" {
		t.Errorf("restricted visibility = %q/%q, want role/Administrators", visType, visValue)
	}
	if !jsd.Valid || jsd.Int64 != 0 {
		t.Errorf("restricted jsd_public = valid=%v val=%d, want 0", jsd.Valid, jsd.Int64)
	}

	visType, visValue = "x", "x"
	jsd = sql.NullInt64{Valid: true, Int64: 1}
	if err := conn.QueryRow(`
		SELECT visibility_type, visibility_value, jsd_public
		FROM comments WHERE external_id = 'c-pub'`).Scan(&visType, &visValue, &jsd); err != nil {
		t.Fatalf("open comment columns: %v", err)
	}
	if visType != "" || visValue != "" {
		t.Errorf("open visibility = %q/%q, want empty", visType, visValue)
	}
	if jsd.Valid {
		t.Errorf("open jsd_public valid=%v val=%d, want NULL", jsd.Valid, jsd.Int64)
	}
}

func TestScopeLabelStandaloneOmitsAccount(t *testing.T) {
	// GDK-464
	connected := scopeLabel(&config.Config{})
	if connected != "every project this account can see" {
		t.Fatalf("connected empty projects = %q", connected)
	}
	st := scopeLabel(&config.Config{Kind: config.KindStandalone, DefaultProject: origin.DefaultProjectKey})
	if strings.Contains(st, "this account") {
		t.Fatalf("standalone still mentions an account: %q", st)
	}
	if st != origin.DefaultProjectKey {
		t.Fatalf("standalone empty projects = %q, want %s", st, origin.DefaultProjectKey)
	}
	named := scopeLabel(&config.Config{Kind: config.KindStandalone, Projects: []string{"IDEA", "STD"}})
	if named != "IDEA, STD" {
		t.Fatalf("standalone with projects = %q", named)
	}
}
