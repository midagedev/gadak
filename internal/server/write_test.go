package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/sync"
)

// fakeJira is enough of Jira Cloud to drive the write-through paths, and it
// records what was sent so a test can assert on the request rather than only on
// the response. The issue it hands back on re-read is deliberately *different*
// from the fixture's row (status 완료), which is how a test proves a write
// actually refreshed the mirror.
type fakeJira struct {
	*httptest.Server
	t *testing.T

	calls          []string                   // "METHOD /path"
	bodies         map[string]json.RawMessage // last body per "METHOD /path"
	status         int                        // when non-zero, the next mutating call fails with it
	errBody        string
	newKey         string // key POST /issue answers with
	editMeta       string // editmeta fields object
	rereadStatus   int    // when non-zero, GET /search/jql (mirror re-read) fails with it
	createMetaJSON string // when set, GET /issue/createmeta answers this
}

func newFakeJira(t *testing.T) *fakeJira {
	f := &fakeJira{t: t, bodies: map[string]json.RawMessage{}, newKey: "NMB-1"}
	f.editMeta = `{
		"customfield_10092": {"schema":{"type":"option"},"operations":["set"],
			"allowedValues":[{"id":"10160","value":"Fixed"},{"id":"10161","value":"Won't Fix"}]},
		"fixVersions": {"schema":{"type":"array","items":"version"},"operations":["set"],
			"allowedValues":[{"id":"v1","name":"1.2.0"}]},
		"customfield_20000": {"schema":{"type":"user"},"operations":["set"]}
	}`
	f.Server = httptest.NewServer(http.HandlerFunc(f.route))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeJira) route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
	tag := r.Method + " " + path
	f.calls = append(f.calls, tag)
	if body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)); err == nil && len(body) > 0 && json.Valid(body) {
		f.bodies[tag] = body
	}
	if r.Header.Get("Authorization") == "" {
		f.t.Errorf("%s: no Authorization header", tag)
	}
	// A configured failure applies to the state-changing calls only, so the
	// metadata reads a write depends on keep working.
	if f.status != 0 && r.Method != http.MethodGet {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.errBody))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch {
	case path == "/myself":
		_, _ = w.Write([]byte(`{"accountId":"acc-hc","displayName":"김현철","emailAddress":"hc@example.com"}`))
	case path == "/status":
		_, _ = w.Write([]byte(`[{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}},
			{"id":"10001","name":"완료","statusCategory":{"key":"done"}}]`))
	case path == "/priority":
		_, _ = w.Write([]byte(`[{"id":"1","name":"Highest"},{"id":"2","name":"High"},{"id":"3","name":"Medium"}]`))
	case path == "/search/jql":
		if f.rereadStatus != 0 {
			w.WriteHeader(f.rereadStatus)
			return
		}
		// The re-read. Status differs from the fixture on purpose.
		// Only answer for keys this fake knows (NMB-1 / newKey); anything else
		// is an empty hit so SyncIssue can surface ErrNotFound → 404.
		jql := ""
		if raw := f.bodies[tag]; len(raw) > 0 {
			var body struct {
				JQL string `json:"jql"`
			}
			_ = json.Unmarshal(raw, &body)
			jql = body.JQL
		}
		wantKeys := []string{`"NMB-1"`, `"` + f.newKey + `"`}
		known := false
		for _, k := range wantKeys {
			if strings.Contains(jql, k) {
				known = true
				break
			}
		}
		// empty jql (body not recorded yet on some paths) → treat as known for
		// backward compatibility with tests that only assert the response shape.
		if jql == "" || known {
			key := "NMB-1"
			if f.newKey != "" && strings.Contains(jql, `"`+f.newKey+`"`) {
				key = f.newKey
			}
			_, _ = w.Write([]byte(`{"issues":[{"id":"1001","key":"` + key + `","fields":{
			"summary":"batch worker drops the last page",
			"status":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},
			"project":{"key":"NMB"},"issuetype":{"id":"10004","name":"Bug"},
			"labels":["batch"],"components":[{"name":"api"}],
			"assignee":{"accountId":"acc-hc","displayName":"김현철","emailAddress":"hc@example.com"},
			"reporter":{"accountId":"acc-rp","displayName":"박보고","emailAddress":"rp@example.com"},
			"created":"2026-07-01T00:00:00.000+0900","updated":"2026-08-04T12:00:00.000+0900"
		}}],"isLast":true}`))
		} else {
			_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
		}
	case strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`{"transitions":[{"id":"31","name":"완료로","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}}]}`))
	case strings.HasSuffix(path, "/editmeta"):
		_, _ = w.Write([]byte(`{"fields":` + f.editMeta + `}`))
	case strings.HasSuffix(path, "/comment") && r.Method == http.MethodPost:
		_, _ = w.Write([]byte(`{"id":"c-99","author":{"displayName":"김현철"},
			"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"확인"}]}]},
			"created":"2026-08-04T12:00:00.000+0900"}`))
	case strings.HasSuffix(path, "/attachments") && r.Method == http.MethodPost:
		if r.Header.Get("X-Atlassian-Token") != "no-check" {
			f.t.Error("attachment upload without the nosniff header")
		}
		_, _ = w.Write([]byte(`[{"id":"9001","filename":"shot.png","mimeType":"image/png","size":11}]`))
	case path == "/issue" && r.Method == http.MethodPost:
		_, _ = w.Write([]byte(`{"id":"1001","key":"` + f.newKey + `"}`))
	case path == "/issue/createmeta":
		if f.createMetaJSON != "" {
			_, _ = w.Write([]byte(f.createMetaJSON))
			return
		}
		_, _ = w.Write([]byte(`{"projects":[{"key":"NMB","name":"Numbers","issuetypes":[{"id":"10004","name":"Bug"}]}]}`))
	case path == "/user/search":
		_, _ = w.Write([]byte(`[{"accountId":"acc-cl","displayName":"이클라","emailAddress":"cl@example.com",
			"avatarUrls":{"48x48":"https://a/48.png"},"active":true}]`))
	default:
		// transitions POST, assignee PUT, issue PUT: 204, like Jira.
		w.WriteHeader(http.StatusNoContent)
	}
}

func (f *fakeJira) called(tag string) bool {
	for _, c := range f.calls {
		if c == tag {
			return true
		}
	}
	return false
}

// writable is the fixture pointed at the fake Jira, with an edit allowlist.
func writable(t *testing.T) (*fakeJira, http.Handler, *config.Config) {
	t.Helper()
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Site = f.URL
	cfg.EditableFields = map[string]string{
		"solution": "customfield_10092", "fix_versions": "fixVersions",
		"development_test_assignee": "customfield_20000",
	}
	return f, New(db, cfg), cfg
}

func send(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := testRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTransitionWritesThroughToTheMirror(t *testing.T) {
	f, h, _ := writable(t)

	before := get(t, h, apiBase+"bootstrap/", nil).Header().Get("ETag")
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/", `{"transition_id":"31"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !f.called("POST /issue/NMB-1/transitions") {
		t.Fatalf("calls %v", f.calls)
	}
	var sent struct {
		Transition struct{ ID string } `json:"transition"`
	}
	_ = json.Unmarshal(f.bodies["POST /issue/NMB-1/transitions"], &sent)
	if sent.Transition.ID != "31" {
		t.Fatalf("transition id %q", sent.Transition.ID)
	}

	// The response carries the re-read row, not the row we had before the write.
	var body struct {
		Issue struct {
			Status         string `json:"status"`
			StatusCategory string `json:"status_category"`
			TeamGroup      string `json:"team_group"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Issue.Status != "완료" || body.Issue.StatusCategory != "done" {
		t.Fatalf("stale row returned: %+v", body.Issue)
	}
	// It is a full IssueLite, group injection included.
	if body.Issue.TeamGroup != "batch" {
		t.Errorf("team_group %q", body.Issue.TeamGroup)
	}
	// And the mirror itself moved, so the next poll and the ETag agree with it.
	if after := get(t, h, apiBase+"bootstrap/", nil).Header().Get("ETag"); after == before {
		t.Errorf("sync version did not move: %s", after)
	}
}

func TestCommentSendsMentionsAsADF(t *testing.T) {
	f, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/",
		`{"text":"@김현철 확인 부탁","mentions":[{"account_id":"acc-hc","display_name":"김현철"}],"attachment_ids":["1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue/NMB-1/comment"])
	if !strings.Contains(sent, `"type":"mention"`) || !strings.Contains(sent, `"id":"acc-hc"`) {
		t.Fatalf("mention not sent as ADF: %s", sent)
	}
	var body struct {
		Issue   map[string]any `json:"issue"`
		Comment struct {
			CommentID string `json:"comment_id"`
			Body      string `json:"body"`
		} `json:"comment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Comment.CommentID != "c-99" || body.Comment.Body != "확인" || body.Issue == nil {
		t.Fatalf("response %+v", body)
	}
	// An empty comment never reaches Jira.
	if rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", `{"text":"  "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty comment → %d", rec.Code)
	}
}

func TestAssigneeSetAndClear(t *testing.T) {
	f, h, _ := writable(t)
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/assignee/", `{"account_id":"acc-cl"}`); rec.Code != http.StatusOK {
		t.Fatalf("set → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1/assignee"]); got != `{"accountId":"acc-cl"}` {
		t.Fatalf("set body %s", got)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/assignee/", `{"account_id":null}`); rec.Code != http.StatusOK {
		t.Fatalf("clear → %d", rec.Code)
	}
	if got := string(f.bodies["PUT /issue/NMB-1/assignee"]); got != `{"accountId":null}` {
		t.Fatalf("clear body %s", got)
	}
}

func TestPrioritySetAndClear(t *testing.T) {
	f, h, _ := writable(t)
	rec := get(t, h, apiBase+"priorities/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list → %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[struct {
		Priorities []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"priorities"`
	}](t, rec)
	if len(got.Priorities) != 3 || got.Priorities[0].ID != "1" || got.Priorities[0].Name != "Highest" {
		t.Fatalf("catalog %v", got.Priorities)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/priority/", `{"priority_id":"2"}`); rec.Code != http.StatusOK {
		t.Fatalf("set → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(body, `"priority":{"id":"2"}`) {
		t.Fatalf("set body %s", body)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/priority/", `{"priority_id":null}`); rec.Code != http.StatusOK {
		t.Fatalf("clear → %d", rec.Code)
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(body, `"priority":null`) {
		t.Fatalf("clear body %s", body)
	}
}

func TestSummarySet(t *testing.T) {
	f, h, _ := writable(t)
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/summary/", `{"summary":"  renamed  "}`); rec.Code != http.StatusOK {
		t.Fatalf("set → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(body, `"summary":"renamed"`) {
		t.Fatalf("set body %s", body)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/summary/", `{"summary":"  "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("blank → %d", rec.Code)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/summary/", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing → %d", rec.Code)
	}
}

func TestLabelsSetAndClear(t *testing.T) {
	f, h, _ := writable(t)
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/labels/", `{"labels":[" batch ","tech-debt","batch"]}`); rec.Code != http.StatusOK {
		t.Fatalf("set → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(got, `"labels":["batch","tech-debt"]`) {
		t.Fatalf("set body %s", got)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/labels/", `{"labels":[]}`); rec.Code != http.StatusOK {
		t.Fatalf("clear → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(got, `"labels":[]`) {
		t.Fatalf("clear body %s", got)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/labels/", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing labels → %d", rec.Code)
	}
	// watches/favorites share this PUT pattern with assignee; each must land correctly.
	if rec := send(t, h, http.MethodPut, apiBase+"watches/NMB-1/", ``); rec.Code != http.StatusNoContent {
		t.Fatalf("watch PUT → %d", rec.Code)
	}
	if got := decode[struct{ Keys []string }](t, get(t, h, apiBase+"watches/", nil)); len(got.Keys) != 1 {
		t.Fatalf("watches %v", got.Keys)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"favorites/NMB-1/", ``); rec.Code != http.StatusNoContent {
		t.Fatalf("favorite PUT → %d", rec.Code)
	}
	if got := decode[struct{ Keys []string }](t, get(t, h, apiBase+"favorites/", nil)); len(got.Keys) != 1 || got.Keys[0] != "NMB-1" {
		t.Fatalf("favorites %v", got.Keys)
	}
}

func TestFieldEditAllowlistAndShapes(t *testing.T) {
	f, h, _ := writable(t)

	// Not in the allowlist: refused here regardless of what the UI offered.
	rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"summary","value":"pwned"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "field_not_editable" {
		t.Fatalf("error %q", got)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatal("a refused edit still reached Jira")
	}

	// option → {"id": …}
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"solution","value":"10160"}`); rec.Code != http.StatusOK {
		t.Fatalf("option edit → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_10092":{"id":"10160"}}}` {
		t.Fatalf("option body %s", got)
	}
	// version array → [{"id": …}]
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"fix_versions","value":["v1"]}`); rec.Code != http.StatusOK {
		t.Fatalf("version edit → %d", rec.Code)
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"fixVersions":[{"id":"v1"}]}}` {
		t.Fatalf("version body %s", got)
	}
	// user → {"accountId": …}
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/",
		`{"field":"development_test_assignee","value":"acc-cl"}`); rec.Code != http.StatusOK {
		t.Fatalf("user edit → %d", rec.Code)
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_20000":{"accountId":"acc-cl"}}}` {
		t.Fatalf("user body %s", got)
	}
	// null clears
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"solution","value":null}`); rec.Code != http.StatusOK {
		t.Fatalf("clear → %d", rec.Code)
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_10092":null}}` {
		t.Fatalf("clear body %s", got)
	}

	// A field Jira says is not editable on this issue is refused even when configured.
	f.editMeta = `{}`
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"solution","value":"10160"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("uneditable field → %d", rec.Code)
	}
}

func TestEditMetaOnlyExposesAllowlistedFields(t *testing.T) {
	_, h, _ := writable(t)
	got := decode[struct {
		Fields map[string]struct {
			Kind     string              `json:"kind"`
			Editable bool                `json:"editable"`
			Options  []map[string]string `json:"options"`
		} `json:"fields"`
	}](t, get(t, h, apiBase+"NMB-1/editmeta/", nil))

	if len(got.Fields) != 3 {
		t.Fatalf("fields %+v", got.Fields)
	}
	if got.Fields["solution"].Kind != "option" || !got.Fields["solution"].Editable {
		t.Fatalf("solution %+v", got.Fields["solution"])
	}
	if got.Fields["solution"].Options[0]["value"] != "Fixed" {
		t.Fatalf("options %+v", got.Fields["solution"].Options)
	}
	if got.Fields["fix_versions"].Kind != "version_array" {
		t.Fatalf("fix_versions %+v", got.Fields["fix_versions"])
	}
	// The version option's label comes from `name` when there is no `value`.
	if got.Fields["fix_versions"].Options[0]["value"] != "1.2.0" {
		t.Fatalf("version label %+v", got.Fields["fix_versions"].Options)
	}
	if got.Fields["development_test_assignee"].Kind != "user" {
		t.Fatalf("user field %+v", got.Fields["development_test_assignee"])
	}

	// An empty allowlist hides the editor entirely, which is the default.
	db, cfg := fixture(t)
	cfg.EditableFields = nil
	empty := decode[struct {
		Fields map[string]any `json:"fields"`
	}](t, get(t, New(db, cfg), apiBase+"NMB-1/editmeta/", nil))
	if len(empty.Fields) != 0 {
		t.Fatalf("fields leaked without an allowlist: %+v", empty.Fields)
	}
}

func TestCreateIssue(t *testing.T) {
	f, h, _ := writable(t)

	// A project outside the mirror would never come back from the re-read.
	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"ZZZ","issue_type":"10004","summary":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unmirrored project → %d", rec.Code)
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "project_not_mirrored" {
		t.Fatalf("error %q", got)
	}

	rec = send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"새 버그","description_text":"본문","labels":["batch"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	for _, want := range []string{`"key":"NMB"`, `"summary":"새 버그"`, `"type":"doc"`, `"labels":["batch"]`} {
		if !strings.Contains(sent, want) {
			t.Errorf("create body missing %s: %s", want, sent)
		}
	}
	if !strings.Contains(rec.Body.String(), `"issue"`) {
		t.Errorf("no issue in response: %s", rec.Body.String())
	}
}

const manyCreateTypes = `{"projects":[{"key":"NMB","name":"Numbers","issuetypes":[
	{"id":"10001","name":"Task"},{"id":"10002","name":"작업"},{"id":"10004","name":"Bug"}]}]}`

func TestCreateIssueOmitsTypeUsesConfigDefault(t *testing.T) {
	f, h, cfg := writable(t)
	f.createMetaJSON = manyCreateTypes
	cfg.DefaultIssueTypeID = "10001"

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","summary":"from default"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"id":"10001"`) {
		t.Fatalf("default type not sent: %s", sent)
	}
	if !strings.Contains(rec.Body.String(), `"source":"config"`) {
		t.Errorf("resolved source missing: %s", rec.Body.String())
	}
}

func TestCreateIssueOmitsTypeUsesSole(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","summary":"sole type"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"id":"10004"`) {
		t.Fatalf("sole type not sent: %s", sent)
	}
	if !strings.Contains(rec.Body.String(), `"source":"sole"`) {
		t.Errorf("resolved source missing: %s", rec.Body.String())
	}
}

func TestCreateIssueOmitsTypeFailsWhenMany(t *testing.T) {
	f, h, _ := writable(t)
	f.createMetaJSON = manyCreateTypes

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","summary":"needs a type"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "pass --type") {
		t.Errorf("error %s", rec.Body.String())
	}
	if f.called("POST /issue") {
		t.Fatalf("omitted type reached Jira: %v", f.calls)
	}
}

func TestCreateIssueStaleDefaultType(t *testing.T) {
	f, h, cfg := writable(t)
	f.createMetaJSON = manyCreateTypes
	cfg.DefaultIssueTypeID = "99999"

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","summary":"stale"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"99999", "not available", "NMB"} {
		if !strings.Contains(body, want) {
			t.Errorf("error %s missing %q", body, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("stale default reached Jira: %v", f.calls)
	}
}

func TestCreateIssueDefaultProject(t *testing.T) {
	f, h, cfg := writable(t)
	cfg.DefaultProject = "NMB"

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"issue_type":"10004","summary":"from default project"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"key":"NMB"`) {
		t.Fatalf("default project not sent: %s", sent)
	}
}

func TestCreateIssueEmptyOptionalFieldsOmittedFromPayload(t *testing.T) {
	f, h, cfg := writable(t)
	f.createMetaJSON = manyCreateTypes
	cfg.DefaultIssueTypeID = "10001"

	// Empty string is "no value", not "set empty". issue_type:"" must resolve
	// via the default; description/priority must not appear on the Jira body.
	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"","summary":"omit empties","description_text":"","priority":"","labels":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"id":"10001"`) {
		t.Fatalf("resolved type missing: %s", sent)
	}
	if strings.Contains(sent, `"id":""`) {
		t.Fatalf("empty issue type id sent: %s", sent)
	}
	for _, forbidden := range []string{`"description"`, `"priority"`, `"labels"`} {
		if strings.Contains(sent, forbidden) {
			t.Errorf("optional field %s present in payload: %s", forbidden, sent)
		}
	}
}

func TestCreateIssueEmptySummaryStillRequired(t *testing.T) {
	f, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "project_issue_type_and_summary_required" {
		t.Fatalf("error %q", got)
	}
	if f.called("POST /issue") {
		t.Fatalf("empty summary reached Jira: %v", f.calls)
	}
}

func TestUploadProxiesAndReturnsContentURL(t *testing.T) {
	f, h, _ := writable(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "shot.png")
	_, _ = part.Write([]byte("PNGBYTES\x00\x01"))
	_ = mw.Close()

	req := testRequest(http.MethodPost, apiBase+"NMB-1/attachments/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload → %d: %s", rec.Code, rec.Body.String())
	}
	if !f.called("POST /issue/NMB-1/attachments") {
		t.Fatalf("calls %v", f.calls)
	}
	got := decode[struct {
		Attachments []struct {
			ID         string `json:"id"`
			ContentURL string `json:"content_url"`
			IsImage    bool   `json:"is_image"`
		} `json:"attachments"`
	}](t, rec)
	if len(got.Attachments) != 1 || got.Attachments[0].ID != "9001" {
		t.Fatalf("attachments %+v", got.Attachments)
	}
	if want := apiBase + "NMB-1/attachments/9001/content/"; got.Attachments[0].ContentURL != want {
		t.Fatalf("content_url %q want %q", got.Attachments[0].ContentURL, want)
	}
	if !got.Attachments[0].IsImage {
		t.Error("png not flagged as an image")
	}
	// Missing file part is the client's error, not Jira's.
	if rec := send(t, h, http.MethodPost, apiBase+"NMB-1/attachments/", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no file → %d", rec.Code)
	}
}

// D10: Jira accepted the upload but the mirror re-read failed. Contract is
// 502 write_applied_mirror_stale, not 200 with the uploaded ids.
func TestUploadMirrorRereadFailureIs502(t *testing.T) {
	f, h, _ := writable(t)
	// 422 is not retried by atlhttp (500/502/503/504 are), so the test
	// observes the handler's own status instead of a 15s retry budget.
	f.rereadStatus = http.StatusUnprocessableEntity

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "shot.png")
	_, _ = part.Write([]byte("PNGBYTES"))
	_ = mw.Close()

	req := testRequest(http.MethodPost, apiBase+"NMB-1/attachments/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("stale-mirror upload → %d %s, want 502", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "write_applied_mirror_stale" {
		t.Fatalf("error %q, want write_applied_mirror_stale", got)
	}
	if !f.called("POST /issue/NMB-1/attachments") {
		t.Fatalf("upload never reached Jira: %v", f.calls)
	}
}

func TestTransitionsAndUsersAndCreateMeta(t *testing.T) {
	_, h, _ := writable(t)

	tr := decode[struct {
		Transitions []transitionDoc `json:"transitions"`
	}](t, get(t, h, apiBase+"NMB-1/transitions/", nil))
	if len(tr.Transitions) != 1 || tr.Transitions[0].ToStatus != "완료" {
		t.Fatalf("transitions %+v", tr.Transitions)
	}
	// Jira's own category key, not the normalized one: the client's type says so.
	if tr.Transitions[0].ToCategory != "done" {
		t.Fatalf("to_category %q", tr.Transitions[0].ToCategory)
	}

	users := decode[struct {
		Users []map[string]any `json:"users"`
	}](t, get(t, h, apiBase+"users/?q=이클", nil))
	if len(users.Users) != 1 || users.Users[0]["account_id"] != "acc-cl" ||
		users.Users[0]["avatar_url"] != "https://a/48.png" || users.Users[0]["active"] != true {
		t.Fatalf("users %+v", users.Users)
	}

	meta := decode[struct {
		Projects []struct {
			Key        string              `json:"key"`
			IssueTypes []map[string]string `json:"issue_types"`
		} `json:"projects"`
	}](t, get(t, h, apiBase+"create-meta/", nil))
	if len(meta.Projects) != 1 || meta.Projects[0].Key != "NMB" ||
		meta.Projects[0].IssueTypes[0]["name"] != "Bug" {
		t.Fatalf("create-meta %+v", meta.Projects)
	}

	wm := decode[map[string]json.RawMessage](t, get(t, h, apiBase+"meta/write/", nil))
	if string(wm["transitions"]) != "{}" {
		t.Errorf("transitions map is precomputed now? %s", wm["transitions"])
	}
	if !strings.Contains(string(wm["create_meta"]), `"NMB"`) {
		t.Errorf("meta/write create_meta: %s", wm["create_meta"])
	}
}

func TestWritesRequireACredential(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{Projects: []string{"NMB"}})
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, apiBase + "NMB-1/transitions/", ""},
		{http.MethodPost, apiBase + "NMB-1/transition/", `{"transition_id":"31"}`},
		{http.MethodPost, apiBase + "NMB-1/comment/", `{"text":"hi"}`},
		{http.MethodPut, apiBase + "NMB-1/assignee/", `{"account_id":null}`},
		{http.MethodPut, apiBase + "NMB-1/labels/", `{"labels":["x"]}`},
		{http.MethodPut, apiBase + "NMB-1/priority/", `{"priority_id":"2"}`},
		{http.MethodPut, apiBase + "NMB-1/summary/", `{"summary":"x"}`},
		{http.MethodGet, apiBase + "priorities/", ""},
		{http.MethodPatch, apiBase + "NMB-1/fields/", `{"field":"solution","value":"1"}`},
		{http.MethodGet, apiBase + "NMB-1/editmeta/", ""},
		{http.MethodPost, apiBase + "create/", `{"project_key":"NMB","issue_type":"1","summary":"x"}`},
		{http.MethodGet, apiBase + "create-meta/", ""},
		{http.MethodGet, apiBase + "users/?q=a", ""},
		{http.MethodPost, apiBase + "NMB-1/resync/", ""},
		{http.MethodPost, apiBase + "pages/100/resync/", ""},
	} {
		rec := send(t, h, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s %s → %d, want 409", tc.method, tc.path, rec.Code)
			continue
		}
		if got := decode[map[string]string](t, rec)["error"]; got != "credential_required" {
			t.Errorf("%s %s error %q", tc.method, tc.path, got)
		}
	}
	// meta/write is on the boot path, so it degrades instead of failing.
	if rec := get(t, h, apiBase+"meta/write/", nil); rec.Code != http.StatusOK {
		t.Errorf("meta/write without a credential → %d", rec.Code)
	}
}

func TestJiraErrorsPassThrough(t *testing.T) {
	f, h, _ := writable(t)
	f.status = http.StatusBadRequest
	f.errBody = `{"errorMessages":["Field is required"],"errors":{"summary":"Summary must be set"}}`

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/", `{"transition_id":"31"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Error      string            `json:"error"`
		JiraErrors map[string]string `json:"jira_errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(body.Error, "Field is required") {
		t.Fatalf("error %q", body.Error)
	}
	if body.JiraErrors["summary"] != "Summary must be set" {
		t.Fatalf("jira_errors %+v", body.JiraErrors)
	}

	// A rejected/expired token is credential_rejected (not credential_required).
	f.status = http.StatusUnauthorized
	f.errBody = ``
	rec = send(t, h, http.MethodPost, apiBase+"NMB-1/transition/", `{"transition_id":"31"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("401 from jira → %d", rec.Code)
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "credential_rejected" {
		t.Fatalf("error %q", got)
	}
}

func TestCredentialLifecycle(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = f.URL, "", ""
	h := New(db, cfg)

	if got := decode[credentialDoc](t, get(t, h, apiBase+"credential/", nil)); got.Configured {
		t.Fatalf("configured before a token: %+v", got)
	}

	rec := send(t, h, http.MethodPut, apiBase+"credential/",
		`{"jira_email":"hc@example.com","api_token":"tok-SECRET-1234"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put → %d: %s", rec.Code, rec.Body.String())
	}
	if !f.called("GET /myself") {
		t.Fatal("token stored without verifying it")
	}
	got := decode[credentialDoc](t, rec)
	if !got.Configured || got.DisplayName != "김현철" || got.VerifiedAt == "" {
		t.Fatalf("credential %+v", got)
	}
	if got.TokenHint != "…1234" {
		t.Fatalf("token_hint %q", got.TokenHint)
	}
	// The token itself never appears in a response.
	if strings.Contains(rec.Body.String(), "tok-SECRET-1234") {
		t.Fatalf("token echoed: %s", rec.Body.String())
	}
	saved, err := config.Load()
	if err != nil || saved.Token != "tok-SECRET-1234" {
		t.Fatalf("not persisted: %+v %v", saved, err)
	}
	if saved.TokenExpirySource != config.TokenExpirySourceAssumed || saved.TokenExpiresAt == "" {
		t.Fatalf("replace-token should assume expiry: source=%q at=%q", saved.TokenExpirySource, saved.TokenExpiresAt)
	}
	// The file holding a token is readable by its owner only.
	path, _ := config.Path()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("config mode %o, want 600", mode)
	}
	// A write works immediately, without restarting the server.
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/assignee/", `{"account_id":null}`); rec.Code != http.StatusOK {
		t.Fatalf("write after storing a credential → %d: %s", rec.Code, rec.Body.String())
	}

	if rec := send(t, h, http.MethodDelete, apiBase+"credential/", ``); rec.Code != http.StatusOK {
		t.Fatalf("delete → %d", rec.Code)
	}
	if got := decode[credentialDoc](t, get(t, h, apiBase+"credential/", nil)); got.Configured || got.TokenHint != "" {
		t.Fatalf("survived deletion: %+v", got)
	}
	if saved, _ := config.Load(); saved.Token != "" || saved.TokenOwner != "" || saved.TokenExpiresAt != "" || saved.TokenExpirySource != "" {
		t.Fatalf("token left on disk: %+v", saved)
	}
}

func TestPutCredentialStoresUserExpiry(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = f.URL, "", ""
	h := New(db, cfg)

	rec := send(t, h, http.MethodPut, apiBase+"credential/",
		`{"jira_email":"hc@example.com","api_token":"tok-SECRET-1234","token_expires_at":"2027-06-15"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put → %d: %s", rec.Code, rec.Body.String())
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.TokenExpirySource != config.TokenExpirySourceUser || saved.TokenExpiresAt != "2027-06-15T00:00:00.000Z" {
		t.Fatalf("user expiry: source=%q at=%q", saved.TokenExpirySource, saved.TokenExpiresAt)
	}
}

func TestRejectedCredentialIsNotStored(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	f := newFakeJira(t)
	f.Close()
	// A site that answers 401 to /myself.
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthorized.Close()

	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = unauthorized.URL, "", ""
	h := New(db, cfg)

	rec := send(t, h, http.MethodPut, apiBase+"credential/", `{"jira_email":"a@b.c","api_token":"bad"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "credential_rejected" {
		t.Fatalf("error %q", got)
	}
	if saved, _ := config.Load(); saved.Token != "" {
		t.Fatalf("rejected token stored: %+v", saved)
	}
}

func TestTokenNeverReachesResponsesOrLogs(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })

	f := newFakeJira(t)
	db, cfg, path := fixtureAt(t)
	cfg.Site = f.URL
	h := New(db, cfg)
	f.status = http.StatusInternalServerError
	f.errBody = `{"errorMessages":["boom"]}`

	bodies := []string{
		send(t, h, http.MethodPost, apiBase+"NMB-1/transition/", `{"transition_id":"31"}`).Body.String(),
		send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", `{"text":"hi"}`).Body.String(),
		get(t, h, apiBase+"credential/", nil).Body.String(),
		get(t, h, apiBase+"settings/", nil).Body.String(),
		get(t, h, apiBase+"bootstrap/", nil).Body.String(),
	}
	doc, err := WebConfig(cfg)
	if err != nil {
		t.Fatalf("WebConfig: %v", err)
	}
	bodies = append(bodies, string(doc), logs.String())
	for i, b := range bodies {
		if strings.Contains(b, "secret-token") {
			t.Fatalf("token leaked in output %d: %s", i, b)
		}
	}
	// The mirror is a file agents read directly, so the token may not be in it —
	// including in the raw issue JSON the sync stores (constitution article 8).
	for _, suffix := range []string{"", "-wal"} {
		raw, err := os.ReadFile(path + suffix)
		if err != nil {
			continue // no WAL file yet is fine
		}
		if bytes.Contains(raw, []byte("secret-token")) {
			t.Fatalf("token found in %s", path+suffix)
		}
	}
}

func TestIssueResyncRefreshesMirror(t *testing.T) {
	f, h, _ := writable(t)

	// Fixture row is 진행 중; fake re-read returns 완료 (Korean status names).
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/resync/", ``)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !f.called("POST /search/jql") {
		t.Fatalf("resync never hit Jira search: %v", f.calls)
	}
	var body struct {
		Issue struct {
			Status         string `json:"status"`
			StatusCategory string `json:"status_category"`
			Summary        string `json:"summary"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Issue.Status != "완료" || body.Issue.StatusCategory != "done" {
		t.Fatalf("stale row returned: %+v", body.Issue)
	}
	if body.Issue.Summary != "batch worker drops the last page" {
		t.Fatalf("summary %q", body.Issue.Summary)
	}

	// Mirror itself moved — bootstrap list row agrees with the response.
	boot := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	var status string
	for _, iss := range boot.Issues {
		if iss.IssueKey == "NMB-1" {
			status = iss.Status
			break
		}
	}
	if status != "완료" {
		t.Fatalf("mirror status %q, want 완료 (bootstrap)", status)
	}
}

func TestIssueResyncNotFound(t *testing.T) {
	_, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"NOPE-9/resync/", ``)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "not_found" {
		t.Fatalf("error %q", got)
	}
}

// confPageMock is a minimal Confluence stand-in for page resync: GET content/{id}
// and child comments. Title/body carry Hangul so localization traps show up.
type confPageMock struct {
	pages map[string]confPageMockRow
}

type confPageMockRow struct {
	Title   string
	Body    string
	Space   string
	Version int
	When    string
}

func (m *confPageMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	if strings.HasSuffix(path, "/child/comment") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "size": 0, "limit": 100})
		return
	}
	if !strings.HasPrefix(path, "/wiki/rest/api/content/") {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(path, "/wiki/rest/api/content/")
	if strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	p, ok := m.pages[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	adf, _ := json.Marshal(map[string]any{
		"type": "doc", "version": 1,
		"content": []any{
			map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": p.Body},
			}},
		},
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": id, "type": "page", "status": "current", "title": p.Title,
		"space": map[string]any{"key": p.Space, "name": "제품"},
		"version": map[string]any{
			"number": p.Version, "when": p.When,
			"by": map[string]any{"accountId": "acc-1", "displayName": "김현철"},
		},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{"value": string(adf), "representation": "atlas_doc_format"},
		},
		"ancestors": []any{},
		"metadata": map[string]any{
			"labels": map[string]any{"results": []any{}, "size": 0, "limit": 25, "start": 0},
		},
	})
}

func TestPageResyncRefreshesMirror(t *testing.T) {
	mock := &confPageMock{pages: map[string]confPageMockRow{
		"100": {
			Title: "빌링 품질 회의록 (개정)", Body: "개정된 본문 — 로그인 실패 재현",
			Space: "PROD", Version: 3, When: "2026-08-05T15:00:00.000Z",
		},
	}}
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)

	db, cfg := fixturePages(t)
	cfg.Site = srv.URL
	h := New(db, cfg)
	before, err := db.SyncState(t.Context(), sync.ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}

	rec := send(t, h, http.MethodPost, apiBase+"pages/100/resync/", ``)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// The one-page resync must not advance the source watermark: that page's
	// lastModified would become the incremental floor and every page edited
	// before it would be skipped on the next pass (sync.SyncPage's contract).
	after, err := db.SyncState(t.Context(), sync.ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Watermark != before.Watermark {
		t.Fatalf("watermark moved by single-page resync: %q -> %q", before.Watermark, after.Watermark)
	}

	detail := decode[struct {
		Title   string          `json:"title"`
		Version int             `json:"version"`
		BodyADF json.RawMessage `json:"body_adf"`
	}](t, get(t, h, apiBase+"pages/100/", nil))
	if detail.Title != "빌링 품질 회의록 (개정)" {
		t.Fatalf("title %q", detail.Title)
	}
	if detail.Version != 3 {
		t.Fatalf("version %d, want 3", detail.Version)
	}
	if !strings.Contains(string(detail.BodyADF), "개정된 본문") {
		t.Fatalf("body_adf missing Korean text: %s", detail.BodyADF)
	}
}

func TestPageResyncNotFound(t *testing.T) {
	mock := &confPageMock{pages: map[string]confPageMockRow{}}
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)

	db, cfg := fixturePages(t)
	cfg.Site = srv.URL
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"pages/99999/resync/", ``)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "not_found" {
		t.Fatalf("error %q", got)
	}
}
