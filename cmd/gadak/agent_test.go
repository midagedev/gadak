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
func (s *searchUsersStub) Transition(context.Context, string, string) error { return errStub }
func (s *searchUsersStub) AddComment(context.Context, string, json.RawMessage) (jira.Comment, error) {
	return jira.Comment{}, errStub
}
func (s *searchUsersStub) SetAssignee(context.Context, string, string) error { return errStub }
func (s *searchUsersStub) PriorityCatalog(context.Context) ([]jira.NamedID, error) {
	return nil, errStub
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
