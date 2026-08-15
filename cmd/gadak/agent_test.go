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
		_, _ = w.Write([]byte(`{"transitions":[
			{"id":"21","name":"Start work","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}},
			{"id":"31","name":"Close","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}}]}`))
	case strings.HasSuffix(path, "/comment") && r.Method == http.MethodPost:
		_, _ = w.Write([]byte(`{"id":"c-99","author":{"displayName":"Dana Whitfield"},
			"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"checked"}]}]},
			"created":"2026-08-04T12:00:00.000+0900"}`))
	case path == "/user/search":
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
		if err == nil || !strings.Contains(err.Error(), "gadak init") {
			t.Errorf("%s without a credential: %v", name, err)
		}
	}
	// Nothing was attempted against Jira, so no partial write can be in flight.
	if len(f.calls) != 0 {
		t.Errorf("called Jira anyway: %v", f.calls)
	}
}
