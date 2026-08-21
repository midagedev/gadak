package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// fakeJira is the smallest Jira Cloud that drives the CLI's write paths, and it
// records what was sent so a test can assert on the request rather than only on
// the printed line. The issue it hands back on the re-read is deliberately
// *different* from the seeded row (status 완료), which is how a test proves the
// write refreshed the mirror instead of reprinting what was already there.
type createdIssue struct {
	key, id, project, summary, typeID, typeName string
	labels                                      []string
}

type fakeJira struct {
	*httptest.Server
	t      *testing.T
	calls  []string
	bodies map[string]string

	// create support (additive; existing write tests never hit these paths)
	created          map[string]createdIssue
	createBodies     []string // every POST /issue body, in order (bodies map keeps only the last)
	lastCreatedKey   string
	nextCreateN      int
	failNthCreate    int  // 1-based; 0 = never fail
	skipCreateReread bool // POST /issue succeeds but /search/jql ignores the new key
	rereadStatus     int  // when non-zero, POST /search/jql fails with this status

	// attach support (additive; existing write tests never hit these paths)
	uploads       []recordedUpload
	failNthAttach int // 1-based; 0 = never fail

	// lang selects localized catalog names the way internal/sync/sync_test.go
	// statusesJSON does. Priority ids stay stable; names follow the account
	// language. The sync fake's /priority list is still English-only — this
	// CLI fake does not inherit that gap.
	lang string

	// transitionsJSON overrides GET /transitions. Empty keeps the default
	// Korean-named Start work / Close pair the existing write tests rely on.
	transitionsJSON string

	// searchUsers, when set, answers GET /user/search for the query string.
	// Default (nil) is the Marco Reyes hit TestAssignResolvesEmailAndUnassigns
	// uses. searchQueries records every query, including cache-miss origin calls.
	searchUsers   func(query string) string
	searchQueries []string

	// editMeta is the fields object GET /issue/{key}/editmeta answers inside
	// {"fields": …}. Empty is {}. Matches the server fake's shape so CLI
	// --editmeta tests drive the same Jira document web handleEditMeta parses.
	editMeta string

	// versionsJSON overrides GET /project/{key}/versions. Empty keeps the
	// default catalog TestEditFixVersion* relies on (id 10012 = v2.5).
	versionsJSON string

	// linkTypesJSON overrides GET /issueLinkType. Empty keeps the Blocks
	// catalog TestLink* relies on (id 10000, outward "blocks").
	linkTypesJSON string
}

// recordedUpload is one multipart POST /issue/{key}/attachments the fake saw.
type recordedUpload struct {
	Key      string
	Filename string
	Content  string
	Token    string
}

func newFakeJira(t *testing.T) *fakeJira {
	f := &fakeJira{t: t, bodies: map[string]string{}, created: map[string]createdIssue{}, nextCreateN: 42}
	f.Server = httptest.NewServer(http.HandlerFunc(f.route))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeJira) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
	tag := r.Method + " " + path
	f.calls = append(f.calls, tag)
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if len(body) > 0 {
		f.bodies[tag] = string(body)
	}
	if r.Header.Get("Authorization") == "" {
		f.t.Errorf("%s: no Authorization header", tag)
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case path == "/status":
		_, _ = w.Write([]byte(`[{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}},
			{"id":"10001","name":"완료","statusCategory":{"key":"done"}}]`))
	case path == "/priority":
		_, _ = w.Write(f.prioritiesJSON())
	case path == "/resolution":
		_, _ = w.Write([]byte(`[{"id":"10000","name":"Done"},{"id":"10002","name":"Won't Do"}]`))
	case path == "/search/jql":
		if f.rereadStatus != 0 {
			w.WriteHeader(f.rereadStatus)
			return
		}
		jql := ""
		if raw := f.bodies[tag]; raw != "" {
			var body struct {
				JQL string `json:"jql"`
			}
			_ = json.Unmarshal([]byte(raw), &body)
			jql = body.JQL
		}
		for key, ci := range f.created {
			if strings.Contains(jql, `"`+key+`"`) {
				f.writeCreatedSearch(w, ci)
				return
			}
		}
		_, _ = w.Write([]byte(`{"issues":[{"id":"1001","key":"NMB-1","fields":{
			"summary":"batch worker drops the last page",
			"status":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},
			"project":{"key":"NMB"},"issuetype":{"id":"10004","name":"Bug"},
			"assignee":{"accountId":"acc-hc","displayName":"Dana Whitfield"},
			"created":"2026-07-01T00:00:00.000+0900","updated":"2026-08-04T12:00:00.000+0900"
		}}],"isLast":true}`))
	case strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet:
		if f.transitionsJSON != "" {
			_, _ = w.Write([]byte(f.transitionsJSON))
			return
		}
		_, _ = w.Write([]byte(`{"transitions":[
			{"id":"21","name":"Start work","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}},
			{"id":"31","name":"Close","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}}]}`))
	case strings.HasSuffix(path, "/comment") && r.Method == http.MethodPost:
		_, _ = w.Write([]byte(`{"id":"c-99","author":{"displayName":"Dana Whitfield"},
			"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"checked"}]}]},
			"created":"2026-08-04T12:00:00.000+0900"}`))
	case path == "/user/search":
		q := r.URL.Query().Get("query")
		f.searchQueries = append(f.searchQueries, q)
		if f.searchUsers != nil {
			body := f.searchUsers(q)
			if body == "" {
				body = "[]"
			}
			_, _ = w.Write([]byte(body))
			return
		}
		_, _ = w.Write([]byte(`[{"accountId":"acc-mr","displayName":"Marco Reyes","emailAddress":"marco@example.com","active":true}]`))
	case path == "/issue" && r.Method == http.MethodPost:
		f.handleCreateIssue(w)
	case strings.HasSuffix(path, "/attachments") && r.Method == http.MethodPost:
		f.handleAttach(w, r, path, body)
	case path == "/issue/createmeta":
		_, _ = w.Write([]byte(`{"projects":[
			{"key":"NMB","name":"Numbers","issuetypes":[
				{"id":"10001","name":"Task"},
				{"id":"10002","name":"작업"},
				{"id":"10004","name":"Bug"}]},
			{"key":"GDK","name":"Gadak","issuetypes":[
				{"id":"10001","name":"Task"}]}
		]}`))
	case strings.HasSuffix(path, "/editmeta"):
		raw := f.editMeta
		if raw == "" {
			raw = "{}"
		}
		_, _ = w.Write([]byte(`{"fields":` + raw + `}`))
	case strings.HasPrefix(path, "/project/") && strings.HasSuffix(path, "/versions") && r.Method == http.MethodGet:
		raw := f.versionsJSON
		if raw == "" {
			raw = `[{"id":"10012","name":"v2.5","released":true,"archived":false},{"id":"10013","name":"v2.6","released":false,"archived":false}]`
		}
		_, _ = w.Write([]byte(raw))
	case path == "/issueLinkType" && r.Method == http.MethodGet:
		raw := f.linkTypesJSON
		if raw == "" {
			raw = `{"issueLinkTypes":[{"id":"10000","name":"Blocks","outward":"blocks","inward":"is blocked by"}]}`
		}
		_, _ = w.Write([]byte(raw))
	case path == "/issueLink" && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusCreated)
	default:
		// transitions POST and assignee PUT answer 204, like Jira.
		w.WriteHeader(http.StatusNoContent)
	}
}

func (f *fakeJira) handleCreateIssue(w http.ResponseWriter) {
	body := f.bodies["POST /issue"]
	f.createBodies = append(f.createBodies, body)
	if f.failNthCreate > 0 && len(f.createBodies) == f.failNthCreate {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorMessages":["forced create failure"]}`))
		return
	}
	var req struct {
		Fields struct {
			Project struct {
				Key string `json:"key"`
			} `json:"project"`
			IssueType struct {
				ID string `json:"id"`
			} `json:"issuetype"`
			Summary string   `json:"summary"`
			Labels  []string `json:"labels"`
		} `json:"fields"`
	}
	_ = json.Unmarshal([]byte(body), &req)
	project := req.Fields.Project.Key
	if project == "" {
		project = "NMB"
	}
	n := f.nextCreateN
	if n == 0 {
		n = 42
	}
	f.nextCreateN = n + 1
	key := project + "-" + strconv.Itoa(n)
	id := "90" + strconv.Itoa(n)
	typeName := fakeTypeName(req.Fields.IssueType.ID)
	f.lastCreatedKey = key
	if !f.skipCreateReread {
		f.created[key] = createdIssue{
			key: key, id: id, project: project, summary: req.Fields.Summary,
			typeID: req.Fields.IssueType.ID, typeName: typeName, labels: req.Fields.Labels,
		}
	}
	_, _ = w.Write([]byte(`{"id":"` + id + `","key":"` + key + `"}`))
}

func (f *fakeJira) handleAttach(w http.ResponseWriter, r *http.Request, path string, body []byte) {
	key := strings.TrimPrefix(strings.TrimSuffix(path, "/attachments"), "/issue/")
	rec := recordedUpload{Key: key, Token: r.Header.Get("X-Atlassian-Token")}
	if _, params, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil {
		mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		if part, err := mr.NextPart(); err == nil {
			rec.Filename = part.FileName()
			b, _ := io.ReadAll(part)
			rec.Content = string(b)
			_ = part.Close()
		}
	}
	f.uploads = append(f.uploads, rec)
	if f.failNthAttach > 0 && len(f.uploads) == f.failNthAttach {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errorMessages":["forced attach failure"]}`))
		return
	}
	id := strconv.Itoa(20000 + len(f.uploads))
	filename := rec.Filename
	if filename == "" {
		filename = "file"
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{
		{"id": id, "filename": filename, "mimeType": "application/octet-stream", "size": len(rec.Content)},
	})
}

func fakeTypeName(id string) string {
	switch id {
	case "10002":
		return "작업"
	case "10004":
		return "Bug"
	default:
		return "Task"
	}
}

// prioritiesJSON is GET /priority. Same three ids as the English catalog;
// lang=="ko" uses Jira Cloud's Korean default names so create/edit resolve
// by the name the account actually sees.
func (f *fakeJira) prioritiesJSON() []byte {
	if f != nil && f.lang == "ko" {
		return []byte(`[{"id":"1","name":"가장 높음"},{"id":"2","name":"높음"},{"id":"3","name":"보통"}]`)
	}
	return []byte(`[{"id":"1","name":"Highest"},{"id":"2","name":"High"},{"id":"3","name":"Medium"}]`)
}

func (f *fakeJira) writeCreatedSearch(w http.ResponseWriter, ci createdIssue) {
	hit := map[string]any{
		"id":  ci.id,
		"key": ci.key,
		"fields": map[string]any{
			"summary": ci.summary,
			"status": map[string]any{
				"id": "10001", "name": "완료",
				"statusCategory": map[string]string{"key": "done"},
			},
			"project":   map[string]string{"key": ci.project},
			"issuetype": map[string]string{"id": ci.typeID, "name": ci.typeName},
			"assignee":  map[string]string{"accountId": "acc-hc", "displayName": "Dana Whitfield"},
			"created":   "2026-07-01T00:00:00.000+0900",
			"updated":   "2026-08-04T12:00:00.000+0900",
		},
	}
	if len(ci.labels) > 0 {
		hit["fields"].(map[string]any)["labels"] = ci.labels
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{hit}, "isLast": true})
}

func (f *fakeJira) called(tag string) bool {
	for _, c := range f.calls {
		if c == tag {
			return true
		}
	}
	return false
}

// mirror puts a one-issue mirror and a config in a throwaway GADAK_HOME, so the
// commands run exactly as they do from a shell: they find everything through
// config.Dir().
func mirror(t *testing.T, site string) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://nimbus.example.com"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"3": "inprogress", "10001": "done"},
		Priorities: []string{"Highest", "High", "Medium"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1001", SourceID: "jira", Kind: "issue", ExternalID: "1001", Key: "NMB-1",
				Title: "batch worker drops the last page", BodyText: "the idempotency key is dropped on retry",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
				Priority: "High", Assignee: "Dana Whitfield", AssigneeID: "acc-hc",
				Labels:         []string{"batch"},
				DescriptionADF: json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"retry drops the key"}]}]}`),
			},
			Comments: []store.Comment{{
				ID: "jira:c-1", ExternalID: "c-1", Author: "Marco Reyes",
				BodyText: "reproduced against the sandbox gateway", CreatedAt: "2026-07-02T00:00:00.000Z",
			}},
			Attachments: []store.Attachment{{
				ID: "jira:a-1", ExternalID: "10021", Filename: "trace.har",
				MimeType: "application/json", Size: 4096, CreatedAt: "2026-07-02T00:00:00.000Z",
			}},
			Changelog: []store.ChangeEntry{{
				ID: "jira:h-1", At: "2026-07-03T00:00:00.000Z", Author: "Dana Whitfield",
				Field: "status", FromValue: "Backlog", FromID: "1", ToValue: "진행 중", ToID: "3",
			}},
			Links: []store.Link{{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"}},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	cfg := &config.Config{Site: site, Email: "agent@example.com", Token: "token", Projects: []string{"NMB"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg
}

// standaloneMirror is mirror("") plus KindStandalone. The no-site open
// tests are about an already-inited local workspace, not an empty
// connected config (GDK-454: HasCredential is true here, so open does
// not print ErrNotConfigured).
func standaloneMirror(t *testing.T) *config.Config {
	t.Helper()
	cfg := mirror(t, "")
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// capture runs a command with stdout redirected, which is the only way to assert
// on what a CLI actually printed.
func capture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	cmdErr := fn()
	os.Stdout = saved
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out), cmdErr
}

func TestIssuePrintsBodyTextWhenADFEmpty(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "linear:lin-desc", SourceID: "linear", Kind: "issue", ExternalID: "lin-desc",
				Key: "FIX-DESC", Title: "linear body", BodyText: "## Overview\n\nmarkdown body",
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "FIX", StatusCategory: "new", Status: "Todo"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error { return cmdIssue([]string{"FIX-DESC"}) })
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !strings.Contains(out, "markdown body") {
		t.Fatalf("issue output missing linear markdown:\n%s", out)
	}
	if strings.Contains(out, `"type":"doc"`) {
		t.Fatal("must not stuff markdown into ADF")
	}
}

func TestOpenFallsBackToJiraBrowseWhenItemURLEmpty(t *testing.T) {
	mirror(t, "https://jira.example.com")
	var got string
	saved := startIssueOpen
	startIssueOpen = func(u string) error { got = u; return nil }
	t.Cleanup(func() { startIssueOpen = saved })
	out, err := capture(t, func() error { return cmdOpen([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	want := "https://jira.example.com/browse/NMB-1"
	if got != want {
		t.Fatalf("opened %q, want Jira browse fallback %q", got, want)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("stdout %q, want the URL", out)
	}
}

func TestOpenPrefersStoredItemURL(t *testing.T) {
	mirror(t, "https://jira.example.com")
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
	linURL := "https://linear.app/example/issue/FIX-OPEN/x"
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "linear:lin-open", SourceID: "linear", Kind: "issue", ExternalID: "lin-open",
				Key: "FIX-OPEN", Title: "open me", URL: linURL,
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "FIX", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var got string
	saved := startIssueOpen
	startIssueOpen = func(u string) error { got = u; return nil }
	t.Cleanup(func() { startIssueOpen = saved })
	out, err := capture(t, func() error { return cmdOpen([]string{"FIX-OPEN"}) })
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != linURL {
		t.Fatalf("opened %q, want stored items.url %q", got, linURL)
	}
	if !strings.Contains(out, linURL) {
		t.Fatalf("stdout %q", out)
	}
}

// stubIssueOpen captures `gadak open`'s browser seam and isolates serve
// discovery so a live listener on this machine cannot flip the standalone
// cases (the no-site path now asks serveFocusURL).
func stubIssueOpen(t *testing.T) *string {
	t.Helper()
	var got string
	savedOpen, savedDiscover := startIssueOpen, discoverServes
	startIssueOpen = func(u string) error { got = u; return nil }
	discoverServes = func() []serveHit { return nil }
	t.Cleanup(func() {
		startIssueOpen = savedOpen
		discoverServes = savedDiscover
	})
	return &got
}

func seedIssueURL(t *testing.T, key, itemURL string) {
	t.Helper()
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:" + key, SourceID: "jira", Kind: "issue", ExternalID: key,
				Key: key, Title: key, URL: itemURL,
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: strings.Split(key, "-")[0], StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func seedNamedIssue(t *testing.T, key, title string) {
	t.Helper()
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:" + key, SourceID: "jira", Kind: "issue", ExternalID: key,
				Key: key, Title: title,
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: strings.Split(key, "-")[0], StatusCategory: "new", Status: "Todo"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenStandaloneRelativeURLDoesNotSucceed(t *testing.T) {
	standaloneMirror(t)
	seedIssueURL(t, "STD-1", "/browse/STD-1")
	got := stubIssueOpen(t)

	_, err := capture(t, func() error { return cmdOpen([]string{"STD-1"}) })
	if err == nil {
		t.Fatalf("open of relative items.url succeeded (opened %q)", *got)
	}
	msg := err.Error()
	if strings.Contains(msg, "gadak init") {
		t.Fatalf("prescribed re-init on an already-inited workspace: %s", msg)
	}
	if !strings.Contains(msg, "no Jira site to browse") || !strings.Contains(msg, "gadak views open") {
		t.Fatalf("want no-site browse advice, got %q", msg)
	}
	if *got != "" {
		t.Fatalf("opened %q; relative /browse/KEY must not go to the OS opener", *got)
	}
}

func TestOpenStandaloneMissingKeyIsNotInitAdvice(t *testing.T) {
	standaloneMirror(t)
	got := stubIssueOpen(t)

	_, err := capture(t, func() error { return cmdOpen([]string{"NOPE-1"}) })
	if err == nil {
		t.Fatalf("missing key succeeded (opened %q)", *got)
	}
	msg := err.Error()
	if !strings.Contains(msg, "NOPE-1 is not in the mirror — check the key, or run `gadak sync`") {
		t.Fatalf("want the issue-verb missing-key wording, got %q", msg)
	}
	if strings.Contains(msg, "gadak init") {
		t.Fatalf("prescribed re-init for a typo: %s", msg)
	}
}

func TestOpenStandaloneKnownKeyWithoutSite(t *testing.T) {
	standaloneMirror(t)
	got := stubIssueOpen(t)

	_, err := capture(t, func() error { return cmdOpen([]string{"NMB-1"}) })
	if err == nil {
		t.Fatalf("no-site open succeeded (opened %q)", *got)
	}
	msg := err.Error()
	if strings.Contains(msg, "gadak init") {
		t.Fatalf("prescribed re-init: %s", msg)
	}
	if !strings.Contains(msg, "no Jira site to browse") {
		t.Fatalf("want no-site browse advice, got %q", msg)
	}
}

func TestOpenMissingKeyWithSiteStillBrowses(t *testing.T) {
	// `open` is the escape hatch to Jira: the mirror may lag a key that
	// exists on the site, so a configured site still gets the browse URL.
	// Only the no-site workspaces (standalone/paired) refuse a missing key.
	mirror(t, "https://jira.example.com")
	got := stubIssueOpen(t)

	if _, err := capture(t, func() error { return cmdOpen([]string{"NOPE-1"}) }); err != nil {
		t.Fatalf("missing key with a site must fall back to browse: %v", err)
	}
	if *got != "https://jira.example.com/browse/NOPE-1" {
		t.Fatalf("opened %q, want the site browse URL", *got)
	}
}

func TestOpenStandaloneUsesLiveServe(t *testing.T) {
	standaloneMirror(t)
	got := stubIssueOpen(t)
	discoverServes = func() []serveHit {
		return []serveHit{{base: "http://127.0.0.1:7777", profile: "", port: "7777"}}
	}

	out, err := capture(t, func() error { return cmdOpen([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("open with live serve: %v", err)
	}
	want := "http://127.0.0.1:7777/#/?issue=NMB-1"
	if *got != want {
		t.Fatalf("opened %q, want serve issue URL %q", *got, want)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("stdout %q, want the URL", out)
	}
}

func TestIssueAndSearchReadTheMirror(t *testing.T) {
	mirror(t, "https://unused.example.com")

	out, err := capture(t, func() error { return cmdIssue([]string{"nmb-1"}) })
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// The key is normalized, and the detail sections come from the mirror.
	for _, want := range []string{"NMB-1", "batch worker drops the last page", "진행 중 (inprogress)",
		"retry drops the key", "reproduced against the sandbox gateway", "trace.har",
		"Blocks outward", "history (1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue output missing %q:\n%s", want, out)
		}
	}
	if _, err := capture(t, func() error { return cmdIssue([]string{"NMB-404"}) }); err == nil {
		t.Error("an unknown key should be an error, not empty output")
	}

	// FTS reaches comment text, not just the summary.
	out, err = capture(t, func() error { return cmdSearch([]string{"sandbox"}) })
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if fields := strings.Split(strings.TrimSpace(out), "\t"); len(fields) != 4 || fields[0] != "NMB-1" {
		t.Fatalf("search line %q", out)
	}

	out, err = capture(t, func() error { return cmdSearch([]string{"--json", "idempotency"}) })
	if err != nil {
		t.Fatalf("search --json: %v", err)
	}
	var body struct {
		Total  int               `json:"total"`
		Issues []store.IssueLite `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if body.Total != 1 || body.Issues[0].IssueKey != "NMB-1" {
		t.Fatalf("search --json %+v", body)
	}
	if !strings.Contains(out, `"issue_key":"NMB-1"`) || !strings.Contains(out, `"key":"NMB-1"`) {
		t.Fatalf("search --json missing issue_key/key alias pair:\n%s", out)
	}

	out, err = capture(t, func() error {
		return cmdSearch([]string{"--jql", `project = NMB AND statusCategory = "In Progress"`})
	})
	if err != nil {
		t.Fatalf("search --jql: %v", err)
	}
	if fields := strings.Split(strings.TrimSpace(out), "\t"); len(fields) != 4 || fields[0] != "NMB-1" {
		t.Fatalf("jql search line %q", out)
	}

	out, err = capture(t, func() error {
		return cmdSearch([]string{"--jql", "--emit", `project = NMB AND statusCategory = "In Progress"`})
	})
	if err != nil {
		t.Fatalf("search --emit: %v", err)
	}
	if !strings.Contains(out, "project = NMB") || !strings.Contains(out, `statusCategory = "In Progress"`) {
		t.Fatalf("emit %q", out)
	}

	out, err = capture(t, func() error {
		return cmdSearch([]string{
			"--jql", `project = NMB AND statusCategory = "In Progress"`, "--json", "--limit", "3",
		})
	})
	if err != nil {
		t.Fatalf("search --jql … --json: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"issue_key":"NMB-1"`) && !strings.Contains(out, `"NMB-1"`) {
		t.Fatalf("trailing --json swallowed into JQL: %s", out)
	}

	out, err = capture(t, func() error {
		return cmdSearch([]string{"https://example.atlassian.net/issues/?jql=project%20%3D%20NMB"})
	})
	if err != nil {
		t.Fatalf("search URL: %v", err)
	}
	if !strings.Contains(out, "NMB-1") {
		t.Fatalf("url search %q", out)
	}
}

// seedIssueEditMeta writes Fields (Kind set) so EditableAliases matches the
// web allowlist, and a fake editmeta that includes an option field with
// allowedValues, a required flag, a version field whose label is `name`,
// and an id that is NOT on the allowlist (must not appear in CLI output).
func seedIssueEditMeta(t *testing.T, f *fakeJira) {
	t.Helper()
	f.editMeta = `{
		"customfield_10092": {"required":true,"schema":{"type":"option"},"operations":["set"],
			"allowedValues":[{"id":"10160","value":"Fixed"},{"id":"10161","value":"Won't Fix"}]},
		"fixVersions": {"schema":{"type":"array","items":"version"},"operations":["set"],
			"allowedValues":[{"id":"v1","name":"1.2.0"}]},
		"customfield_secret": {"schema":{"type":"option"},"operations":["set"],
			"allowedValues":[{"id":"9","value":"hidden"}]}
	}`
	cfg := mirror(t, f.URL)
	cfg.Fields = []config.FieldSpec{
		{Alias: "solution", Label: "Solution", IDs: []string{"customfield_10092"}, Role: "facet", Kind: "option"},
		{Alias: "fix_versions", Label: "Fix Version/s", IDs: []string{"fixVersions"}, Role: "facet", Kind: "version_array"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func findEditMetaField(t *testing.T, fields []map[string]any, alias string) map[string]any {
	t.Helper()
	for _, f := range fields {
		if f["alias"] == alias {
			return f
		}
	}
	t.Fatalf("missing alias %q in %#v", alias, fields)
	return nil
}

// TestIssueEditMetaJSONIntersectsAllowlist is the GDK-514 producer: origin
// GET editmeta ∩ EditableAliases, options from allowedValues, required
// forwarded, allowlist-outside ids omitted.
func TestIssueEditMetaJSONIntersectsAllowlist(t *testing.T) {
	f := newFakeJira(t)
	seedIssueEditMeta(t, f)

	out, err := capture(t, func() error {
		return cmdIssue([]string{"NMB-1", "--editmeta", "--json"})
	})
	if err != nil {
		t.Fatalf("issue --editmeta --json: %v\n%s", err, out)
	}
	if !f.called("GET /issue/NMB-1/editmeta") {
		t.Fatalf("did not GET editmeta: %v", f.calls)
	}
	var doc struct {
		Key    string           `json:"key"`
		Fields []map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if doc.Key != "NMB-1" {
		t.Fatalf("key %q", doc.Key)
	}
	if len(doc.Fields) != 2 {
		t.Fatalf("fields %#v, want 2 (secret is outside the allowlist)", doc.Fields)
	}
	sol := findEditMetaField(t, doc.Fields, "solution")
	if sol["id"] != "customfield_10092" || sol["kind"] != "option" {
		t.Fatalf("solution %#v", sol)
	}
	if sol["required"] != true {
		t.Fatalf("solution.required %#v, want true", sol["required"])
	}
	opts, _ := sol["options"].([]any)
	if len(opts) != 2 {
		t.Fatalf("solution.options %#v", sol["options"])
	}
	first, _ := opts[0].(map[string]any)
	if first["id"] != "10160" || first["name"] != "Fixed" {
		t.Fatalf("first option %#v", first)
	}
	fv := findEditMetaField(t, doc.Fields, "fix_versions")
	if fv["kind"] != "version_array" {
		t.Fatalf("fix_versions %#v", fv)
	}
	vopts, _ := fv["options"].([]any)
	if len(vopts) != 1 {
		t.Fatalf("fix_versions.options %#v", fv["options"])
	}
	v0, _ := vopts[0].(map[string]any)
	if v0["name"] != "1.2.0" {
		t.Fatalf("version label %#v (name, not value — versions have no value)", v0)
	}
	for _, field := range doc.Fields {
		if field["alias"] == "secret" || field["id"] == "customfield_secret" {
			t.Fatalf("allowlist-outside id leaked: %#v", field)
		}
	}

	human, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "--editmeta"}) })
	if err != nil {
		t.Fatalf("issue --editmeta: %v\n%s", err, human)
	}
	if !strings.Contains(human, "solution (option, required)") {
		t.Fatalf("human missing required field:\n%s", human)
	}
	if !strings.Contains(human, "options: Fixed, Won't Fix") {
		t.Fatalf("human missing option names:\n%s", human)
	}
	if strings.Contains(human, "hidden") || strings.Contains(human, "secret") {
		t.Fatalf("human leaked allowlist-outside field:\n%s", human)
	}
}

func TestIssueEditMetaRefusesWithoutCredential(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Token = ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "--editmeta"}) })
	if err == nil || !strings.Contains(err.Error(), config.ErrNotConfigured.Error()) {
		t.Fatalf("no credential: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("called origin anyway: %v", f.calls)
	}
}

func TestIssueEditMetaRefusesUndefinedFlagCombos(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	for _, args := range [][]string{
		{"NMB-1", "--derive", "--editmeta"},
		{"NMB-1", "--editmeta", "--link"},
	} {
		_, err := capture(t, func() error { return cmdIssue(args) })
		if err == nil {
			t.Fatalf("%v: want usage error", args)
		}
		if !strings.Contains(err.Error(), "cannot be combined") || !strings.Contains(err.Error(), "--editmeta") {
			t.Errorf("%v: error %q, want a cannot-be-combined usage refusal that names --editmeta", args, err)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("combination reached origin: %v", f.calls)
	}
}

func TestIssueWithoutFlagsDoesNotHitOrigin(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, out)
	}
	for _, want := range []string{"NMB-1", "batch worker drops the last page", "진행 중 (inprogress)",
		"retry drops the key", "reproduced against the sandbox gateway", "trace.har",
		"Blocks outward", "history (1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue output missing %q:\n%s", want, out)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("flagless issue hit origin: %v", f.calls)
	}
}

const bulkSecondTitle = "bulk second issue"

func issueJSONKey(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var doc struct {
		Issue struct {
			Key string `json:"issue_key"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode issue document: %v\n%s", err, raw)
	}
	if doc.Issue.Key == "" {
		t.Fatalf("issue.issue_key empty in %s", raw)
	}
	return doc.Issue.Key
}

// TestIssueMultipleKeysPrintsInOrder is the GDK-425 producer: extra positionals
// used to be dropped with no error, so a bulk read silently returned the first
// key only.
func TestIssueMultipleKeysPrintsInOrder(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedNamedIssue(t, "NMB-99", bulkSecondTitle)

	one, err := capture(t, func() error { return cmdIssue([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("single issue: %v", err)
	}

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "NMB-99"}) })
	if err != nil {
		t.Fatalf("issue two keys: %v\n%s", err, out)
	}
	sep := "--- NMB-99 ---\n"
	i := strings.Index(out, sep)
	if i < 0 {
		t.Fatalf("missing separator %q (second key was likely ignored):\n%s", strings.TrimSpace(sep), out)
	}
	if got := out[:i]; got != one {
		t.Fatalf("first document drifted from single-key output\n--- single ---\n%s--- multi prefix ---\n%s", one, got)
	}
	rest := out[i+len(sep):]
	if !strings.Contains(rest, bulkSecondTitle) {
		t.Fatalf("second document missing %q:\n%s", bulkSecondTitle, rest)
	}
	if strings.Contains(out[:i], bulkSecondTitle) {
		t.Fatalf("second title leaked into the first document:\n%s", out[:i])
	}
}

func TestIssueMultipleKeysJSONArray(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedNamedIssue(t, "NMB-99", bulkSecondTitle)

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "NMB-99", "--json"}) })
	if err != nil {
		t.Fatalf("issue two keys --json: %v\n%s", err, out)
	}
	var docs []json.RawMessage
	if err := json.Unmarshal([]byte(out), &docs); err != nil {
		t.Fatalf("want a JSON array, got: %v\n%s", err, out)
	}
	if len(docs) != 2 {
		t.Fatalf("array length %d, want 2\n%s", len(docs), out)
	}
	if k := issueJSONKey(t, docs[0]); k != "NMB-1" {
		t.Fatalf("docs[0] issue.issue_key = %q, want NMB-1", k)
	}
	if k := issueJSONKey(t, docs[1]); k != "NMB-99" {
		t.Fatalf("docs[1] issue.issue_key = %q, want NMB-99", k)
	}

	viaKeys, err := capture(t, func() error { return cmdIssue([]string{"--keys", "NMB-1,NMB-99", "--json"}) })
	if err != nil {
		t.Fatalf("issue --keys --json: %v\n%s", err, viaKeys)
	}
	if viaKeys != out {
		t.Fatalf("--keys JSON differed from positional JSON\n--- positional ---\n%s--- --keys ---\n%s", out, viaKeys)
	}
}

func TestIssueJSONIncludesLinkedPRs(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceDevLinks(context.Background(), "NMB-1", store.DevLinksUpdate{Links: []store.DevLink{{
		Kind: "pullrequest", URL: "https://github.com/midagedev/gadak/pull/50",
		Title: "from panel", Status: "merged", UpdatedAt: "2026-08-21T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "--json"}) })
	if err != nil {
		t.Fatalf("issue --json: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"linked_prs"`) {
		t.Fatalf("CLI JSON missing linked_prs:\n%s", out)
	}
	if !strings.Contains(out, "https://github.com/midagedev/gadak/pull/50") {
		t.Fatalf("linked_prs missing the PR URL:\n%s", out)
	}

	human, err := capture(t, func() error { return cmdIssue([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, human)
	}
	if !strings.Contains(human, "Linked PRs") {
		t.Fatalf("human output missing Linked PRs block:\n%s", human)
	}
}

func TestIssueJSONSecurityLevel(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{
			{
				Item: store.Item{
					ID: "jira:sec-1", SourceID: "jira", Kind: "issue", ExternalID: "sec-1",
					Key: "NMB-10", Title: "restricted",
					CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "NMB", Status: "Todo", StatusCategory: "new",
					SecurityLevelID: "10000", SecurityLevel: "내부",
				},
			},
			{
				Item: store.Item{
					ID: "jira:sec-2", SourceID: "jira", Kind: "issue", ExternalID: "sec-2",
					Key: "NMB-11", Title: "unrestricted",
					CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
				},
				Issue: store.Issue{ProjectKey: "NMB", Status: "Todo", StatusCategory: "new"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-10", "--json"}) })
	if err != nil {
		t.Fatalf("issue --json restricted: %v\n%s", err, out)
	}
	var doc struct {
		Issue struct {
			SecurityLevelID *string `json:"security_level_id"`
			SecurityLevel   *string `json:"security_level"`
		} `json:"issue"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if doc.Issue.SecurityLevelID == nil || *doc.Issue.SecurityLevelID != "10000" {
		t.Fatalf("security_level_id = %v, want 10000\n%s", doc.Issue.SecurityLevelID, out)
	}
	if doc.Issue.SecurityLevel == nil || *doc.Issue.SecurityLevel != "내부" {
		t.Fatalf("security_level = %v, want 내부\n%s", doc.Issue.SecurityLevel, out)
	}

	human, err := capture(t, func() error { return cmdIssue([]string{"NMB-10"}) })
	if err != nil {
		t.Fatalf("issue restricted: %v\n%s", err, human)
	}
	if !strings.Contains(human, "security") || !strings.Contains(human, "내부") {
		t.Fatalf("human output missing security line:\n%s", human)
	}

	openJSON, err := capture(t, func() error { return cmdIssue([]string{"NMB-11", "--json"}) })
	if err != nil {
		t.Fatalf("issue --json unrestricted: %v\n%s", err, openJSON)
	}
	if err := json.Unmarshal([]byte(openJSON), &doc); err != nil {
		t.Fatalf("decode unrestricted: %v\n%s", err, openJSON)
	}
	if doc.Issue.SecurityLevelID != nil || doc.Issue.SecurityLevel != nil {
		t.Fatalf("unrestricted security = %v/%v, want null\n%s", doc.Issue.SecurityLevelID, doc.Issue.SecurityLevel, openJSON)
	}

	openHuman, err := capture(t, func() error { return cmdIssue([]string{"NMB-11"}) })
	if err != nil {
		t.Fatalf("issue unrestricted: %v\n%s", err, openHuman)
	}
	if strings.Contains(openHuman, "security") {
		t.Fatalf("unrestricted human output grew a security line:\n%s", openHuman)
	}
}

func TestIssueSingleKeyJSONIsObject(t *testing.T) {
	mirror(t, "https://unused.example.com")

	text, err := capture(t, func() error { return cmdIssue([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if strings.Contains(text, "--- NMB-1 ---") {
		t.Fatalf("single-key text grew a separator:\n%s", text)
	}

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "--json"}) })
	if err != nil {
		t.Fatalf("issue --json: %v\n%s", err, out)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(out), &arr); err == nil {
		t.Fatalf("single-key --json became an array (%d elems):\n%s", len(arr), out)
	}
	if issueJSONKey(t, json.RawMessage([]byte(out))) != "NMB-1" {
		t.Fatalf("single-key --json issue.issue_key:\n%s", out)
	}
	var docMap map[string]any
	if err := json.Unmarshal([]byte(out), &docMap); err != nil {
		t.Fatalf("issue --json: %v\n%s", err, out)
	}
	if docMap["issue_key"] != "NMB-1" || docMap["key"] != docMap["issue_key"] {
		t.Fatalf("issue --json top-level key alias diverged: issue_key=%v key=%v", docMap["issue_key"], docMap["key"])
	}
	issue, _ := docMap["issue"].(map[string]any)
	if issue["issue_key"] != "NMB-1" || issue["key"] != issue["issue_key"] {
		t.Fatalf("issue --json nested IssueLite key alias diverged: %v", issue)
	}
}

func TestIssueMissingKeyContinues(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedNamedIssue(t, "NMB-99", bulkSecondTitle)

	out, stderr, err := captureErr(t, func() error {
		return cmdIssue([]string{"NMB-1", "NOPE-1", "NMB-99"})
	})
	if err == nil {
		t.Fatal("mixed missing key: want non-nil error (exit non-0)")
	}
	if !strings.Contains(stderr, "NOPE-1") {
		t.Fatalf("stderr missing the unknown key, got %q", stderr)
	}
	if !strings.Contains(out, "batch worker drops the last page") {
		t.Fatalf("found key NMB-1 was not printed:\n%s", out)
	}
	if !strings.Contains(out, bulkSecondTitle) {
		t.Fatalf("found key NMB-99 was not printed:\n%s", out)
	}
	if strings.Contains(out, "--- NOPE-1 ---") {
		t.Fatalf("missing key was treated as a document:\n%s", out)
	}
}

func TestIssueDeriveRefusesMultipleKeys(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedNamedIssue(t, "NMB-99", bulkSecondTitle)

	_, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "NMB-99", "--derive"}) })
	if err == nil {
		t.Fatal("--derive with two keys: want usage refusal")
	}
	if !strings.Contains(err.Error(), "cannot be combined") || !strings.Contains(err.Error(), "--derive") {
		t.Fatalf("error %q, want a cannot-be-combined usage refusal that names --derive", err)
	}
}

func TestIssueKeysRejectsPositionals(t *testing.T) {
	mirror(t, "https://unused.example.com")
	err := cmdIssue([]string{"NMB-1", "--keys", "NMB-99"})
	if err == nil {
		t.Fatal("positional + --keys: want usage refusal")
	}
	if !strings.Contains(err.Error(), "--keys") || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("error %q, want --keys/positional usage refusal", err)
	}
}

func TestIssueKeysStdin(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedNamedIssue(t, "NMB-99", bulkSecondTitle)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("NMB-1\nNMB-99\n")
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	out, err := capture(t, func() error { return cmdIssue([]string{"--keys", "-"}) })
	os.Stdin = saved
	if err != nil {
		t.Fatalf("issue --keys -: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--- NMB-99 ---") || !strings.Contains(out, bulkSecondTitle) {
		t.Fatalf("--keys - missing second document:\n%s", out)
	}
}

// TestIssueLink is the GDK-163 producer: gadak issue KEY --link prints the
// same gadak:// form views open composes (deeplink.Compose + workspace.Prefix),
// and --json names the fields deeplink / web so the two commands agree.
func TestIssueLink(t *testing.T) {
	mirror(t, "https://unused.example.com")
	// A live serve on this machine must not flip the web field.
	stubViewsLaunchSeams(t, nil)

	const want = "gadak://view?issue=NMB-1"

	out, err := capture(t, func() error { return cmdIssue([]string{"nmb-1", "--link"}) })
	if err != nil {
		t.Fatalf("issue --link: %v\n%s", err, out)
	}
	if !strings.Contains(out, "deeplink\t"+want+"\n") {
		t.Fatalf("issue --link missing exact deeplink line:\n%s", out)
	}
	if strings.Contains(out, "\nweb\t") || strings.HasPrefix(out, "web\t") {
		t.Fatalf("empty discovery must omit the web line, got:\n%s", out)
	}

	out, err = capture(t, func() error { return cmdIssue([]string{"NMB-1", "--link", "--json"}) })
	if err != nil {
		t.Fatalf("issue --link --json: %v\n%s", err, out)
	}
	var body struct {
		DeepLink string `json:"deeplink"`
		Web      string `json:"web"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if body.DeepLink != want {
		t.Fatalf("deeplink %q, want %q", body.DeepLink, want)
	}
	if body.Web != "" {
		t.Fatalf("web %q, want empty under empty discovery", body.Web)
	}

	// Named profile: same composer, same /w/<name> rule as views open.
	if got := deepLinkURL("oss", "issue=NMB-1"); got != "gadak://view/w/oss?issue=NMB-1" {
		t.Fatalf("named-profile link %q, want gadak://view/w/oss?issue=NMB-1", got)
	}

	if _, err := capture(t, func() error { return cmdIssue([]string{"NMB-404", "--link"}) }); err == nil {
		t.Error("issue --link on an unknown key should be an error, not a dead link")
	}
}

func TestIssueLinkPrintsWebWhenServeFound(t *testing.T) {
	mirror(t, "https://unused.example.com")
	stubViewsLaunchSeams(t, []serveHit{{
		base: "http://127.0.0.1:7777", profile: "", port: "7777",
	}})

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1", "--link", "--json"}) })
	if err != nil {
		t.Fatalf("issue --link --json: %v\n%s", err, out)
	}
	var body struct {
		DeepLink string `json:"deeplink"`
		Web      string `json:"web"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if body.DeepLink != "gadak://view?issue=NMB-1" {
		t.Fatalf("deeplink %q", body.DeepLink)
	}
	const wantWeb = "http://127.0.0.1:7777/#/?issue=NMB-1"
	if body.Web != wantWeb {
		t.Fatalf("web %q, want %q", body.Web, wantWeb)
	}
}

// A query that starts with a `--` comment is what an agent pastes out of
// AGENTS.md, and flag.Parse would read that leading `--` as an undefined flag.
func TestSQLTakesACommentedQueryAndEmitsCSV(t *testing.T) {
	mirror(t, "https://unused.example.com")

	out, err := capture(t, func() error {
		return cmdSQL([]string{"--csv", "-- what is open?\nselect key, resolution from issues"})
	})
	if err != nil {
		t.Fatalf("sql --csv: %v", err)
	}
	// Header row, then the row itself with NULL as empty rather than Go's "<nil>".
	if out != "key,resolution\nNMB-1,\n" {
		t.Fatalf("csv %q", out)
	}
	// The flag is recognized after the query too.
	if out, err = capture(t, func() error {
		return cmdSQL([]string{"-- count\nselect count(*) as n from issues", "--json"})
	}); err != nil || strings.TrimSpace(out) != `{"n":1}` {
		t.Fatalf("sql --json → %q, %v", out, err)
	}
}

func TestTransitionMatchesByNameAndReportsAlternatives(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	// Matched on the *target status* name, which is what a caller knows; this
	// workflow calls the transition "Close".
	out, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "완료"}) })
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if !f.called("POST /issue/NMB-1/transitions") {
		t.Fatalf("calls %v", f.calls)
	}
	if body := f.bodies["POST /issue/NMB-1/transitions"]; !strings.Contains(body, `"id":"31"`) {
		t.Fatalf("sent %s", body)
	}
	// The printed line is the re-read row, not the row the mirror held before.
	if !strings.Contains(out, "NMB-1\t완료\t") {
		t.Fatalf("stale line %q", out)
	}

	_, err = capture(t, func() error { return cmdTransition([]string{"NMB-1", "Ship it"}) })
	if err == nil {
		t.Fatal("an unmatched transition must fail")
	}
	// The error has to carry the way out: what this issue can actually do.
	for _, want := range []string{"Ship it", "Start work", "id 31"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

// searchUsersStub is a Writer that only implements SearchUsers. GDK-406
// pins that Linear assign must go through this instead of cfg.Members.
type searchUsersStub struct {
	query string
	users []jira.User
}

func (s *searchUsersStub) SearchUsers(_ context.Context, q string) ([]jira.User, error) {
	s.query = q
	return s.users, nil
}
func (s *searchUsersStub) CreateMeta(context.Context, []string) ([]jira.CreateMetaProject, error) {
	return nil, errStub
}
func (s *searchUsersStub) CreateFields(context.Context, string, string) ([]jira.CreateFieldMeta, error) {
	return nil, errStub
}
func (s *searchUsersStub) CreateIssue(context.Context, map[string]any) (string, error) {
	return "", errStub
}
func (s *searchUsersStub) EditMeta(context.Context, string) (map[string]jira.FieldMeta, error) {
	return nil, errStub
}
func (s *searchUsersStub) UpdateFields(context.Context, string, map[string]any) error { return errStub }
func (s *searchUsersStub) EditIssue(context.Context, string, map[string]any, map[string]any) error {
	return errStub
}
func (s *searchUsersStub) Transitions(context.Context, string) ([]jira.Transition, error) {
	return nil, errStub
}
func (s *searchUsersStub) Transition(context.Context, string, string, map[string]any, json.RawMessage) error {
	return errStub
}
func (s *searchUsersStub) AddComment(context.Context, string, json.RawMessage, *jira.CommentVisibility, bool) (jira.Comment, error) {
	return jira.Comment{}, errStub
}
func (s *searchUsersStub) SetAssignee(context.Context, string, string) error { return errStub }
func (s *searchUsersStub) PriorityCatalog(context.Context) ([]jira.NamedID, error) {
	return nil, errStub
}
func (s *searchUsersStub) ProjectVersions(context.Context, string) ([]jira.Version, error) {
	return nil, errStub
}
func (s *searchUsersStub) IssueLinkTypes(context.Context) ([]jira.IssueLinkType, error) {
	return nil, errStub
}
func (s *searchUsersStub) LinkIssues(context.Context, string, string, string) error {
	return errStub
}
func (s *searchUsersStub) Upload(context.Context, string, string, io.Reader) ([]jira.Attachment, error) {
	return nil, errStub
}
func (s *searchUsersStub) MediaRef(context.Context, string) (string, string, error) {
	return "", "", errStub
}

var errStub = errStubSentinel("searchUsersStub: unused method")

type errStubSentinel string

func (e errStubSentinel) Error() string { return string(e) }

var _ origin.Writer = (*searchUsersStub)(nil)

func TestResolveAccountLinearSkipsJiraMemberDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	cfg := &config.Config{
		Site: "https://example.invalid", Email: "a@b.c", Token: "tok",
		Members: []config.Member{{Email: "dana@example.com", JiraAccountID: "jira-acc-1"}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	stub := &searchUsersStub{users: []jira.User{{AccountID: "lin-u1", DisplayName: "Dana", Email: "dana@example.com"}}}
	id, err := resolveAccount(context.Background(), stub, "dana@example.com", "linear")
	if err != nil {
		t.Fatal(err)
	}
	if id == "jira-acc-1" {
		t.Fatal("linear assign used the Jira member-directory shortcut")
	}
	if id != "lin-u1" {
		t.Fatalf("id = %q, want lin-u1 from SearchUsers", id)
	}
	if stub.query != "dana@example.com" {
		t.Fatalf("SearchUsers query = %q", stub.query)
	}

	id, err = resolveAccount(context.Background(), stub, "dana@example.com", "jira")
	if err != nil {
		t.Fatal(err)
	}
	if id != "jira-acc-1" {
		t.Fatalf("jira assign id = %q, want the member-directory shortcut", id)
	}
}

func saveResolveAccountConfig(t *testing.T, members []config.Member) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	cfg := &config.Config{
		Site: "https://example.invalid", Email: "a@b.c", Token: "tok",
		Members: members,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

// GDK-515: a site that hides emails still resolves a unique display-name hit.
func TestResolveAccountNameSingleHitNoEmail(t *testing.T) {
	saveResolveAccountConfig(t, nil)
	stub := &searchUsersStub{users: []jira.User{
		{AccountID: "712020:abc", DisplayName: "Dana", Email: ""},
	}}
	id, err := resolveAccount(context.Background(), stub, "Dana", "jira")
	if err != nil {
		t.Fatal(err)
	}
	if id != "712020:abc" {
		t.Fatalf("id = %q, want 712020:abc", id)
	}
	if stub.query != "Dana" {
		t.Fatalf("SearchUsers query = %q", stub.query)
	}
}

// GDK-515: accountId exact match must win among multiple SearchUsers hits.
// Two hits so the single-hit fallback cannot paper over a missing AccountID
// comparison — current source refuses this as ambiguous (FAIL-first).
func TestResolveAccountIDExactMatchAmongMultiple(t *testing.T) {
	saveResolveAccountConfig(t, nil)
	stub := &searchUsersStub{users: []jira.User{
		{AccountID: "712020:abc", DisplayName: "Dana", Email: ""},
		{AccountID: "712020:def", DisplayName: "Dana Kim", Email: ""},
	}}
	id, err := resolveAccount(context.Background(), stub, "712020:abc", "jira")
	if err != nil {
		t.Fatal(err)
	}
	if id != "712020:abc" {
		t.Fatalf("id = %q, want 712020:abc", id)
	}
	if stub.query != "712020:abc" {
		t.Fatalf("SearchUsers query = %q", stub.query)
	}

	_, err = resolveAccount(context.Background(), stub, "712020:ABC", "jira")
	if err == nil {
		t.Fatal("accountId match is case-sensitive; 712020:ABC must not accept 712020:abc")
	}
	if !strings.Contains(err.Error(), "matches 2 users") {
		t.Fatalf("wrong-cased accountId: %v", err)
	}
}

func TestResolveAccountAmbiguousNameStillRefused(t *testing.T) {
	saveResolveAccountConfig(t, nil)
	stub := &searchUsersStub{users: []jira.User{
		{AccountID: "712020:abc", DisplayName: "Dana", Email: ""},
		{AccountID: "712020:def", DisplayName: "Dana Kim", Email: ""},
	}}
	_, err := resolveAccount(context.Background(), stub, "Dana", "jira")
	if err == nil {
		t.Fatal("ambiguous name must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "matches 2 users") {
		t.Fatalf("want candidate-list refusal, got %v", err)
	}
	for _, want := range []string{"Dana", "Dana Kim"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q missing %q", msg, want)
		}
	}
}

func TestResolveAccountEmailWinsOverAccountID(t *testing.T) {
	saveResolveAccountConfig(t, nil)
	who := "overlap@example.com"
	stub := &searchUsersStub{users: []jira.User{
		{AccountID: who, DisplayName: "ID Hit", Email: ""},
		{AccountID: "acc-email-hit", DisplayName: "Email Hit", Email: who},
	}}
	id, err := resolveAccount(context.Background(), stub, who, "jira")
	if err != nil {
		t.Fatal(err)
	}
	if id != "acc-email-hit" {
		t.Fatalf("id = %q, want acc-email-hit (email match wins)", id)
	}
}

func TestResolveAccountMemberDirectoryByAccountID(t *testing.T) {
	saveResolveAccountConfig(t, []config.Member{
		{Email: "dana@example.com", JiraAccountID: "jira-acc-1"},
	})
	stub := &searchUsersStub{}
	id, err := resolveAccount(context.Background(), stub, "jira-acc-1", "jira")
	if err != nil {
		t.Fatal(err)
	}
	if id != "jira-acc-1" {
		t.Fatalf("id = %q, want jira-acc-1 from the member directory", id)
	}
	if stub.query != "" {
		t.Fatalf("SearchUsers query = %q, want none (member-directory shortcut)", stub.query)
	}

	id, err = resolveAccount(context.Background(), stub, "jira-acc-1", "linear")
	if err == nil {
		t.Fatal("linear assign must not use the Jira member-directory accountId shortcut")
	}
	if id != "" {
		t.Fatalf("linear id = %q, want empty on refuse", id)
	}
	if stub.query != "jira-acc-1" {
		t.Fatalf("linear SearchUsers query = %q", stub.query)
	}
}

func TestAssignJoinsTrailingWords(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.searchUsers = func(query string) string {
		if query == "Dana Whitfield" {
			return `[{"accountId":"acc-hc","displayName":"Dana Whitfield","emailAddress":"dana@example.com","active":true}]`
		}
		return "[]"
	}
	if _, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "Dana", "Whitfield"})
	}); err != nil {
		t.Fatalf("assign Dana Whitfield: %v", err)
	}
	if body := f.bodies["PUT /issue/NMB-1/assignee"]; !strings.Contains(body, `"accountId":"acc-hc"`) {
		t.Fatalf("joined name did not resolve: %s (queries %v)", body, f.searchQueries)
	}
	if len(f.searchQueries) == 0 || f.searchQueries[len(f.searchQueries)-1] != "Dana Whitfield" {
		t.Fatalf("SearchUsers query = %v, want Dana Whitfield", f.searchQueries)
	}
}

func TestAssignResolvesEmailAndUnassigns(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	if _, err := capture(t, func() error { return cmdAssign([]string{"NMB-1", "marco@example.com"}) }); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if body := f.bodies["PUT /issue/NMB-1/assignee"]; !strings.Contains(body, `"accountId":"acc-mr"`) {
		t.Fatalf("sent %s (calls %v)", body, f.calls)
	}
	if _, err := capture(t, func() error { return cmdAssign([]string{"NMB-1", "-"}) }); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if body := f.bodies["PUT /issue/NMB-1/assignee"]; !strings.Contains(body, `"accountId":null`) {
		t.Fatalf("unassign sent %s", body)
	}
	// `-` never asks Jira who that is.
	if f.called("GET /user/search") && len(f.calls) < 2 {
		t.Errorf("calls %v", f.calls)
	}
}

func TestCommentSendsADFAndRefusesAnEmptyBody(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "checked on staging", "--json"})
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	body := f.bodies["POST /issue/NMB-1/comment"]
	if !strings.Contains(body, `"type":"doc"`) || !strings.Contains(body, "checked on staging") {
		t.Fatalf("sent %s", body)
	}
	var res struct {
		Issue   store.IssueLite `json:"issue"`
		Comment struct {
			CommentID string `json:"comment_id"`
			Body      string `json:"body"`
		} `json:"comment"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Issue.IssueKey != "NMB-1" || res.Issue.Status != "완료" || res.Comment.CommentID != "c-99" {
		t.Fatalf("response %+v", res)
	}

	if _, err := capture(t, func() error { return cmdComment([]string{"NMB-1", "-m", "   "}) }); err == nil {
		t.Error("an empty comment must not reach Jira")
	}

	// `-m -` reads the body from stdin, which is how anything multi-line gets in.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("line one\nline two\n")
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	_, err = capture(t, func() error { return cmdComment([]string{"NMB-1", "-m", "-"}) })
	os.Stdin = saved
	if err != nil {
		t.Fatalf("comment -m -: %v", err)
	}
	if body := f.bodies["POST /issue/NMB-1/comment"]; !strings.Contains(body, "line two") {
		t.Fatalf("stdin body not sent: %s", body)
	}
}

// GDK-315: trailing positional words are the body, like create's positional
// SUMMARY. Both positional text and -m is ambiguous and refused.
func TestCommentTakesPositionalBody(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "landed", "on", "main,", "CI", "green"})
	}); err != nil {
		t.Fatalf("positional comment: %v", err)
	}
	if body := f.bodies["POST /issue/NMB-1/comment"]; !strings.Contains(body, "landed on main, CI green") {
		t.Fatalf("sent %s", body)
	}

	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "positional", "-m", "flagged"})
	}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("ambiguous body not refused: %v", err)
	}
}

func commentADF(f *fakeJira) string {
	return f.bodies["POST /issue/NMB-1/comment"]
}

func TestCommentMentionSingleHitBecomesNode(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(q string) string {
		if q == "Dana" {
			return `[{"accountId":"acc-dana","displayName":"Dana Whitfield","emailAddress":"dana@example.com","active":true}]`
		}
		return `[]`
	}
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "hi @Dana"})
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	body := commentADF(f)
	if !strings.Contains(body, `"type":"mention"`) {
		t.Fatalf("ADF missing mention node: %s", body)
	}
	if !strings.Contains(body, `"id":"acc-dana"`) {
		t.Fatalf("mention attrs.id: %s", body)
	}
}

func TestCommentMentionAmbiguousRefusesWrite(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(q string) string {
		if q == "Dana" {
			return `[{"accountId":"acc-dw","displayName":"Dana Whitfield","active":true},{"accountId":"acc-dk","displayName":"Dana Kim","active":true}]`
		}
		return `[]`
	}
	mirror(t, f.URL)

	stdout, stderr, err := captureBoth(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "hi @Dana"})
	})
	if err == nil {
		t.Fatal("ambiguous mention must fail")
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("comment was sent: calls %v body %s", f.calls, commentADF(f))
	}
	msg := err.Error() + stderr
	for _, want := range []string{"Dana Whitfield", "Dana Kim"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error/stderr %q missing %q", msg, want)
		}
	}
	// Two hits is over-specification, not absence: a refusal that says "no
	// user matching" sends the reader looking for a name that was found twice.
	if strings.Contains(msg, "no user matching") {
		t.Errorf("ambiguous refusal claims the name matched nobody: %q", msg)
	}
	if !strings.Contains(msg, "matches 2 users") {
		t.Errorf("ambiguous refusal does not say how many matched: %q", msg)
	}
	if strings.Contains(stdout, "Dana Whitfield") && strings.Contains(stdout, "matches") {
		t.Errorf("ambiguous warning leaked to stdout: %q", stdout)
	}
}

func TestCommentMentionZeroHitsStaysPlainAndWarns(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(string) string { return `[]` }
	mirror(t, f.URL)

	stdout, stderr, err := captureBoth(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "hi @Dana"})
	})
	if err != nil {
		t.Fatalf("zero-hit mention must still post: %v", err)
	}
	if !f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("comment not sent; calls %v", f.calls)
	}
	body := commentADF(f)
	if strings.Contains(body, `"type":"mention"`) {
		t.Fatalf("zero-hit must stay plain text: %s", body)
	}
	if !strings.Contains(stderr, "Dana") {
		t.Fatalf("stderr must name the unresolved token: %q", stderr)
	}
	if strings.Contains(stdout, "plain text") || strings.Contains(stdout, "did not resolve") {
		t.Fatalf("warning leaked to stdout: %q", stdout)
	}
}

func TestCommentEmailDoesNotSearchUsers(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(string) string {
		t.Fatal("SearchUsers must not run for a@b.com")
		return `[]`
	}
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "mail me at a@b.com"})
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	t.Logf("SearchUsers queries=%v count=%d", f.searchQueries, len(f.searchQueries))
	if len(f.searchQueries) != 0 {
		t.Fatalf("SearchUsers queries %v, want none", f.searchQueries)
	}
	if !f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("comment not sent; calls %v", f.calls)
	}
}

// A real Jira user search is fuzzy: it returns the person for any query that
// contains their name, extra words included. Live measurement 2026-08-21:
// `-m "@김현철 GDK-510 멘션 해석이 …"` posted a comment whose body had lost
// "GDK-510 멘션" — the three-word candidate matched and the mention node ate
// it. Resolution must never consume words the shorter name already resolved.
func TestCommentMentionDoesNotSwallowFollowingWords(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(q string) string {
		if strings.Contains(q, "Dana") {
			return `[{"accountId":"acc-dw","displayName":"Dana Whitfield","active":true}]`
		}
		return `[]`
	}
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "@Dana please review GDK-510"})
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	body := commentADF(f)
	if !strings.Contains(body, `"id":"acc-dw"`) {
		t.Fatalf("mention not resolved: %s", body)
	}
	if !strings.Contains(body, `"text":"@Dana"`) {
		t.Fatalf("mention consumed more than the name: %s", body)
	}
	for _, word := range []string{"please", "review", "GDK-510"} {
		if !strings.Contains(body, word) {
			t.Fatalf("body lost %q: %s", word, body)
		}
	}
	// One hit on the first (shortest) query is the whole answer — the longer
	// candidates must not even be asked.
	t.Logf("SearchUsers queries=%v", f.searchQueries)
	if len(f.searchQueries) != 1 || f.searchQueries[0] != "Dana" {
		t.Fatalf("queries %v, want exactly [Dana]", f.searchQueries)
	}
}

// A shorter name that names two people is the only reason to reach for a
// longer one. The fake matches the two-word name exactly, as a site whose
// search is stricter would.
func TestCommentTwoWordMentionExtendsOnlyWhenNeeded(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(q string) string {
		if q == "Dana Whitfield" {
			return `[{"accountId":"acc-dw","displayName":"Dana Whitfield","active":true}]`
		}
		return `[]`
	}
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "@Dana Whitfield please look"})
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	body := commentADF(f)
	if !strings.Contains(body, `"type":"mention"`) || !strings.Contains(body, `"id":"acc-dw"`) {
		t.Fatalf("two-word mention not adopted: %s", body)
	}
	t.Logf("SearchUsers queries=%v count=%d", f.searchQueries, len(f.searchQueries))
	if len(f.searchQueries) > 3 {
		t.Fatalf("SearchUsers calls %d (%v), want ≤ 3", len(f.searchQueries), f.searchQueries)
	}
	// Shortest first, and the search stops as soon as one name resolves — the
	// original assertion here pinned longest-first, which is the order that
	// swallowed body text against a live site (see
	// TestCommentMentionDoesNotSwallowFollowingWords).
	if len(f.searchQueries) == 0 || f.searchQueries[0] != "Dana" {
		t.Fatalf("first query %v, want the 1-word candidate first", f.searchQueries)
	}
	if last := f.searchQueries[len(f.searchQueries)-1]; last != "Dana Whitfield" {
		t.Fatalf("stopped at %q, want to stop at the resolving 2-word name", last)
	}
	foundTwo := false
	for _, q := range f.searchQueries {
		if q == "Dana Whitfield" {
			foundTwo = true
		}
	}
	if !foundTwo {
		t.Fatalf("2-word candidate never queried: %v", f.searchQueries)
	}
}

func TestCommentRepeatedMentionUsesSearchCache(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(q string) string {
		if q == "Dana" {
			return `[{"accountId":"acc-dana","displayName":"Dana Whitfield","active":true}]`
		}
		return `[]`
	}
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "hi @Dana and again @Dana"})
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	body := commentADF(f)
	if !strings.Contains(body, `"type":"mention"`) || !strings.Contains(body, `"id":"acc-dana"`) {
		t.Fatalf("mention not sent: %s", body)
	}
	t.Logf("SearchUsers queries=%v count=%d", f.searchQueries, len(f.searchQueries))
	nDana := 0
	for _, q := range f.searchQueries {
		if q == "Dana" {
			nDana++
		}
	}
	if nDana != 1 {
		t.Fatalf("SearchUsers(%q) = %d (%v), want 1 (cache)", "Dana", nDana, f.searchQueries)
	}
}

func commentPOSTBody(t *testing.T, f *fakeJira) map[string]json.RawMessage {
	t.Helper()
	raw := f.bodies["POST /issue/NMB-1/comment"]
	if raw == "" {
		t.Fatalf("no comment POST (calls %v)", f.calls)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("comment POST %s: %v", raw, err)
	}
	return body
}

// TestCommentWithoutFlagsOmitsVisibilityAndProperties is the GDK-511
// regression: a flagless comment POST is still {"body":…} only.
func TestCommentWithoutFlagsOmitsVisibilityAndProperties(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "checked on staging"})
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	body := commentPOSTBody(t, f)
	if _, ok := body["visibility"]; ok {
		t.Errorf("flagless POST must omit visibility: %s", f.bodies["POST /issue/NMB-1/comment"])
	}
	if _, ok := body["properties"]; ok {
		t.Errorf("flagless POST must omit properties: %s", f.bodies["POST /issue/NMB-1/comment"])
	}
	if _, ok := body["body"]; !ok {
		t.Errorf("flagless POST missing body: %s", f.bodies["POST /issue/NMB-1/comment"])
	}
	want, err := json.Marshal(map[string]any{"body": jira.Doc("checked on staging", nil)})
	if err != nil {
		t.Fatal(err)
	}
	if f.bodies["POST /issue/NMB-1/comment"] != string(want) {
		t.Errorf("flagless POST body not byte-identical\n got: %s\nwant: %s", f.bodies["POST /issue/NMB-1/comment"], want)
	}
}

func TestCommentInternalPostsJSMProperty(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "x", "--internal"})
	}); err != nil {
		t.Fatalf("comment --internal: %v", err)
	}
	body := commentPOSTBody(t, f)
	raw, ok := body["properties"]
	if !ok {
		t.Fatalf("missing properties: %s", f.bodies["POST /issue/NMB-1/comment"])
	}
	var props []struct {
		Key   string `json:"key"`
		Value struct {
			Internal bool `json:"internal"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &props); err != nil {
		t.Fatalf("properties %s: %v", raw, err)
	}
	if len(props) == 0 || props[0].Key != "sd.public.comment" || !props[0].Value.Internal {
		t.Errorf("properties = %s, want [{key:sd.public.comment, value:{internal:true}}]", raw)
	}
}

func TestCommentVisibilityPostsRole(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "x", "--visibility", "role=Administrators"})
	}); err != nil {
		t.Fatalf("comment --visibility: %v", err)
	}
	body := commentPOSTBody(t, f)
	raw, ok := body["visibility"]
	if !ok {
		t.Fatalf("missing visibility: %s", f.bodies["POST /issue/NMB-1/comment"])
	}
	var vis struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &vis); err != nil {
		t.Fatalf("visibility %s: %v", raw, err)
	}
	if vis.Type != "role" || vis.Value != "Administrators" {
		t.Errorf("visibility = %+v, want role/Administrators", vis)
	}
}

func TestIssueMarksRestrictedAndInternalComments(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	jsdFalse := false
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:vis-1", SourceID: "jira", Kind: "issue", ExternalID: "vis-1",
				Key: "VIS-1", Title: "visibility fixture",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", StatusCategory: "new", Status: "To Do", StatusID: "1"},
			Comments: []store.Comment{
				{
					ID: "jira:c-vis", ExternalID: "c-vis", Author: "Dana",
					BodyText: "admins only", CreatedAt: "2026-07-02T00:00:00.000Z",
					VisibilityType: "role", VisibilityValue: "Administrators",
				},
				{
					ID: "jira:c-int", ExternalID: "c-int", Author: "Dana",
					BodyText: "jsm note", CreatedAt: "2026-07-03T00:00:00.000Z",
					JsdPublic: &jsdFalse,
				},
				{
					ID: "jira:c-pub", ExternalID: "c-pub", Author: "Dana",
					BodyText: "everyone", CreatedAt: "2026-07-04T00:00:00.000Z",
				},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdIssue([]string{"VIS-1"}) })
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !strings.Contains(out, "[restricted: role Administrators]") {
		t.Errorf("human output missing restricted mark:\n%s", out)
	}
	if !strings.Contains(out, "[internal]") {
		t.Errorf("human output missing internal mark:\n%s", out)
	}
	if !strings.Contains(out, "everyone") {
		t.Errorf("human output missing unrestricted body:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "everyone") && (strings.Contains(line, "[restricted") || strings.Contains(line, "[internal]")) {
			t.Errorf("unrestricted comment was marked: %s", line)
		}
	}

	js, err := capture(t, func() error { return cmdIssue([]string{"VIS-1", "--json"}) })
	if err != nil {
		t.Fatalf("issue --json: %v", err)
	}
	var doc struct {
		Comments []store.DetailComment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatalf("decode %s: %v", js, err)
	}
	if len(doc.Comments) != 3 {
		t.Fatalf("comments = %d, want 3", len(doc.Comments))
	}
	byID := map[string]store.DetailComment{}
	for _, c := range doc.Comments {
		byID[c.ExternalID] = c
	}
	vis := byID["c-vis"]
	if vis.VisibilityType != "role" || vis.VisibilityValue != "Administrators" {
		t.Errorf("c-vis visibility = %q/%q", vis.VisibilityType, vis.VisibilityValue)
	}
	if vis.JsdPublic != nil {
		t.Errorf("c-vis jsd_public = %v, want null", vis.JsdPublic)
	}
	in := byID["c-int"]
	if in.JsdPublic == nil || *in.JsdPublic {
		t.Errorf("c-int jsd_public = %v, want false", in.JsdPublic)
	}
	if in.VisibilityType != "" || in.VisibilityValue != "" {
		t.Errorf("c-int visibility = %q/%q, want empty", in.VisibilityType, in.VisibilityValue)
	}
	pub := byID["c-pub"]
	if pub.VisibilityType != "" || pub.VisibilityValue != "" || pub.JsdPublic != nil {
		t.Errorf("c-pub visibility=%q/%q jsd=%v, want empty/null", pub.VisibilityType, pub.VisibilityValue, pub.JsdPublic)
	}
	if !strings.Contains(js, `"jsd_public": null`) && !strings.Contains(js, `"jsd_public":null`) {
		t.Errorf("JSON missing null jsd_public:\n%s", js)
	}
}

func TestCommentVisibilityDuplicateIsUsageError(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "x",
			"--visibility", "role=Administrators", "--visibility", "group=jira-software-users"})
	})
	if err == nil {
		t.Fatal("duplicate --visibility must be refused")
	}
	if !strings.Contains(err.Error(), "visibility") {
		t.Errorf("error %q, want it to name visibility", err)
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("duplicate flag reached Jira: %v", f.calls)
	}
}

func TestWritesRefuseToRunWithoutACredential(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Token = ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	tmp := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(tmp, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func() error{
		"comment":    func() error { return cmdComment([]string{"NMB-1", "-m", "hello"}) },
		"transition": func() error { return cmdTransition([]string{"NMB-1", "완료"}) },
		"assign":     func() error { return cmdAssign([]string{"NMB-1", "marco@example.com"}) },
		"create":     func() error { return cmdCreate([]string{"a summary", "--project", "NMB", "--type", "Task"}) },
		"attach":     func() error { return cmdAttach([]string{"NMB-1", tmp}) },
		"edit":       func() error { return cmdEdit([]string{"NMB-1", "--summary", "renamed"}) },
		"link":       func() error { return cmdLink([]string{"NMB-1", "NMB-2", "--type", "blocks"}) },
	} {
		_, err := capture(t, run)
		// GDK-454: writes share config.ErrNotConfigured (plus the
		// writes-go-to-Jira addendum). The old "no Jira credential" copy
		// was one of five sentences for the same empty workspace.
		if err == nil || !strings.Contains(err.Error(), config.ErrNotConfigured.Error()) {
			t.Errorf("%s without a credential: %v", name, err)
		}
	}
	// Nothing was attempted against Jira, so no partial write can be in flight.
	if len(f.calls) != 0 {
		t.Errorf("called Jira anyway: %v", f.calls)
	}
}

/* ── issue --derive (GDK-111) ── */

// seedDerivable adds a second issue to the mirror mirror() already created, with
// a changelog that exercises every rule store.Derive implements: a resolution
// that is later undone, two reopens (one of them into a status the site's
// category list does not cover), an assignee change, and a comment that follows
// the last reopen. It goes in through db.UpsertIssues — the same path the
// connector uses — so the stored derived columns are store.Derive's own output
// and never a hand-written expectation.
//
// Status display names are Korean while the ids stay the site's: a rule that
// branched on a name instead of a category would score this issue differently
// from the English site, which is the defect data-model.md's "Derived field
// rules" and the repo CLAUDE.md both call out.
//
// It also parks one issue in each of the workflow's other statuses. The site's
// status list is fetched from Jira at sync time and never mirrored, so an
// offline reader can only recover a status id's category from an issue that
// currently sits in it — the same shape a real mirror has, where every status
// the workflow uses is occupied by something.
func seedDerivable(t *testing.T) {
	t.Helper()
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ko := map[string]string{"1": "해야 할 일", "3": "진행 중", "5": "완료", "9": "릴리즈 대기"}
	at := func(day int) string { return "2026-07-0" + strconv.Itoa(day) + "T00:00:00.000Z" }
	status := func(day int, from, to string) store.ChangeEntry {
		return store.ChangeEntry{
			ID: "jira:h7-" + strconv.Itoa(day), At: at(day), Author: "Dana Whitfield",
			Field: "status", FromValue: ko[from], FromID: from, ToValue: ko[to], ToID: to,
		}
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		// "9" is deliberately absent: an id the site's status list does not cover
		// counts as not-done, which can only ever miss a reopen (data-model.md).
		Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"},
		Priorities: []string{"Highest", "High", "Medium"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1007", SourceID: "jira", Kind: "issue", ExternalID: "1007", Key: "NMB-7",
				Title:     "the retry fix keeps coming back",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-08T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: ko["3"], StatusID: "3", StatusCategory: "inprogress",
				Priority: "Medium", Assignee: "Grace Okafor", AssigneeID: "acc-go",
			},
			Comments: []store.Comment{
				{ID: "jira:c-7a", ExternalID: "c-7a", Author: "Marco Reyes",
					BodyText: "before the last reopen", CreatedAt: at(2)},
				{ID: "jira:c-7b", ExternalID: "c-7b", Author: "Grace Okafor",
					BodyText: "the retry fix regressed on the sandbox gateway", CreatedAt: "2026-07-06T01:00:00.000Z"},
				{ID: "jira:c-7c", ExternalID: "c-7c", Author: "Dana Whitfield",
					BodyText: "a later comment", CreatedAt: at(8)},
			},
			Changelog: []store.ChangeEntry{
				status(1, "1", "3"),
				status(2, "3", "5"), // into done
				status(3, "5", "1"), // reopen #1
				{ID: "jira:h7-4", At: at(4), Author: "Dana Whitfield",
					Field: "assignee", FromValue: "Dana Whitfield", ToValue: "Grace Okafor"},
				status(5, "1", "5"), // into done again
				status(6, "5", "9"), // reopen #2: 9 is unmapped, so not done
				status(7, "9", "3"), // unmapped -> inprogress is not a reopen
			},
			Links: []store.Link{{Type: "Cloners", Direction: "inward", TargetKey: "NMS-42"}},
		}, anchor("1008", "NMB-8", "1", "new", ko), anchor("1009", "NMB-9", "5", "done", ko)},
	}); err != nil {
		t.Fatalf("seed NMB-7: %v", err)
	}
}

// anchor is an issue parked in one status, so the mirror carries that status
// id's category. No changelog: its only job is to be somewhere.
func anchor(id, key, statusID, category string, names map[string]string) store.IssueRecord {
	return store.IssueRecord{
		Item: store.Item{
			ID: "jira:" + id, SourceID: "jira", Kind: "issue", ExternalID: id, Key: key,
			Title: "parked in " + names[statusID], CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-07-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001",
			Status: names[statusID], StatusID: statusID, StatusCategory: category,
		},
	}
}

// issuesColumns is the set contract 2 is checked against: the real column names
// of the issues table, read from the mirror rather than typed out here.
func issuesColumns(t *testing.T) map[string]bool {
	t.Helper()
	db, err := openReadOnly()
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name FROM pragma_table_info('issues')`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		cols[n] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !cols["reopen_count"] {
		t.Fatalf("pragma returned no issues columns: %v", cols)
	}
	return cols
}

// derivedFieldNames pulls every `name = value` head off the rendered derivation.
// The command prints a derived field name in column 0 and nothing else there,
// which is what makes contract 2 checkable instead of eyeballed.
func derivedFieldNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		name, _, ok := strings.Cut(line, " = ")
		if !ok || strings.ContainsAny(name, " \t") {
			continue
		}
		names = append(names, name)
	}
	return names
}

// TestIssueDeriveShowsTheDerivation is the recurrence gate for GDK-111. It pins
// the rendered derivation against the very values internal/store/derive_test.go
// asserts (TestDerive's "multiple reopens keep the newest" and "an unmapped
// status counts as not done", and TestDeriveReopenReason), by reading them back
// off the row store.Derive itself wrote.
func TestIssueDeriveShowsTheDerivation(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedDerivable(t)

	out, err := capture(t, func() error { return cmdIssue([]string{"nmb-7", "--derive"}) })
	if err != nil {
		t.Fatalf("issue --derive: %v\n%s", err, out)
	}

	// The stored row is store.Derive's output at sync time; the command must
	// print the same numbers by calling the same function, not by recomputing.
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	lites, err := lookup(db, []string{"NMB-7"})
	if err != nil || len(lites) != 1 {
		t.Fatalf("lookup: %v %d", err, len(lites))
	}
	_ = db.Close()
	l := lites[0]
	if l.ReopenCount != 2 {
		t.Fatalf("fixture stored reopen_count = %d, want 2 — the seed no longer matches derive_test.go", l.ReopenCount)
	}
	if l.ResolvedAt != nil {
		t.Fatalf("fixture stored resolved_at = %q, want NULL (the issue is not done now)", *l.ResolvedAt)
	}

	for _, want := range []string{
		// reopen_count, and the two rows that produced it, by category.
		"reopen_count = 2",
		"2026-07-03T00:00:00.000Z",
		"2026-07-06T00:00:00.000Z",
		"reopened_at = 2026-07-06T00:00:00.000Z",
		// resolved_at is NULL and the output says which rule cleared it.
		"resolved_at = (null)",
		// Categories, never display names, are what the rules key on.
		"done → new",
		"inprogress → done",
		// An id the site's category list does not cover is named as such.
		"(unmapped)",
		// The comment reopen_reason came from, identified.
		"the retry fix regressed on the sandbox gateway",
		// Fields the same one pass computes.
		"assignee_changed_at = 2026-07-04T00:00:00.000Z",
		"status_changed_at = 2026-07-07T00:00:00.000Z",
		"cloned_from = NMS-42",
		"priority_rank = 3",
		"comment_count = 3",
		// Calling the same function must reproduce what the sync wrote.
		"every value above matches what the last sync wrote",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--derive output missing %q:\n%s", want, out)
		}
	}
	// Korean display names must be shown (they are the operator's vocabulary)
	// without any rule keying on them.
	if !strings.Contains(out, "진행 중") {
		t.Errorf("--derive dropped the site's own status names:\n%s", out)
	}

	// Contract 2: every field name the derivation prints is a real issues column.
	cols := issuesColumns(t)
	names := derivedFieldNames(out)
	if len(names) < 8 {
		t.Fatalf("only %d derived field names found — the output shape changed: %v", len(names), names)
	}
	unknown := []string{}
	for _, n := range names {
		if !cols[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) != 0 {
		t.Errorf("--derive invented %d field names that are not issues columns: %v", len(unknown), unknown)
	}

	// An issue with no reopen must still explain itself rather than print nothing.
	out, err = capture(t, func() error { return cmdIssue([]string{"NMB-1", "--derive"}) })
	if err != nil {
		t.Fatalf("issue --derive (no reopen): %v", err)
	}
	for _, want := range []string{"reopen_count = 0", "reopen_reason = (empty)"} {
		if !strings.Contains(out, want) {
			t.Errorf("--derive on a never-reopened issue missing %q:\n%s", want, out)
		}
	}

	// --json is an agent-facing contract; --derive must not reshape it.
	out, err = capture(t, func() error { return cmdIssue([]string{"NMB-7", "--json"}) })
	if err != nil {
		t.Fatalf("issue --json: %v", err)
	}
	if strings.Contains(out, "derivation") || !strings.Contains(out, `"reopen_count": 2`) {
		t.Errorf("--json changed shape:\n%s", out)
	}
}

// TestIssueDeriveReportsAnIncompleteCategoryMap pins the honest-failure half of
// the command. The site's status list is fetched from Jira during sync and is
// not mirrored, so an offline recomputation can lose a status id no issue sits
// in any more. When that happens --derive must say so and show the divergence
// from the stored column — never print the smaller number as if it were fact.
func TestIssueDeriveReportsAnIncompleteCategoryMap(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedDerivable(t)

	// NMB-9 was the only issue sitting in status 5, so removing it takes the
	// "done" category for that id out of everything an offline reader can see.
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteItems(context.Background(), "jira", []string{"NMB-9"}); err != nil {
		t.Fatalf("delete NMB-9: %v", err)
	}
	_ = db.Close()

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-7", "--derive"}) })
	if err != nil {
		t.Fatalf("issue --derive: %v", err)
	}
	for _, want := range []string{
		"NOT covered",
		"reopen_count: stored 2, recomputed 0",
		"re-run `gadak sync`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--derive hid an incomplete category map, missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "every value above matches what the last sync wrote") {
		t.Errorf("--derive claimed agreement while the category map was short:\n%s", out)
	}
}
