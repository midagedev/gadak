package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"context"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
)

// fakeJira is enough of Jira Cloud to drive the write-through paths, and it
// records what was sent so a test can assert on the request rather than only on
// the response. The issue it hands back on re-read is deliberately *different*
// from the fixture's row (status 완료), which is how a test proves a write
// actually refreshed the mirror.
type fakeJira struct {
	*httptest.Server
	t *testing.T

	calls              []string                   // "METHOD /path"
	bodies             map[string]json.RawMessage // last body per "METHOD /path"
	status             int                        // when non-zero, the next mutating call fails with it
	errBody            string
	newKey             string // key POST /issue answers with
	editMeta           string // editmeta fields object
	rereadStatus       int    // when non-zero, GET /search/jql (mirror re-read) fails with it
	createMetaJSON     string // when set, GET /issue/createmeta answers this
	createFieldsJSON   string // when set, GET /issue/createmeta/{p}/issuetypes/{t} answers this
	createFieldsStatus int    // when non-zero, that GET fails with it
	transitionsJSON    string // when set, GET /issue/{key}/transitions answers this
	// issueStatusJSON overrides GET /issue/{key}?fields=status,assignee —
	// the category no-op's origin read. Empty keeps NMB-1 in progress.
	issueStatusJSON string
	// linkTypesJSON overrides GET /issueLinkType. Empty keeps the Blocks
	// catalog the CLI link tests use (GDK-19 / GDK-85).
	linkTypesJSON string
	// description, when set, is the ADF the search answers with for NMB-1 —
	// what origin.CurrentDescription reads before a description write.
	description string
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
	case path == "/resolution":
		_, _ = w.Write([]byte(`[{"id":"10000","name":"Done"},{"id":"10002","name":"Won't Do"}]`))
	case path == "/search/jql":
		if f.rereadStatus != 0 {
			w.WriteHeader(f.rereadStatus)
			return
		}
		// The re-read. Status differs from the fixture on purpose.
		// Only answer for keys this fake knows (NMB-1 / NMB-2 / newKey);
		// anything else is an empty hit so SyncIssue can surface ErrNotFound → 404.
		// NMB-2 keeps fixture id 1002 so a two-key refresh (GDK-85 link) does
		// not collide with NMB-1's item id.
		jql := ""
		if raw := f.bodies[tag]; len(raw) > 0 {
			var body struct {
				JQL string `json:"jql"`
			}
			_ = json.Unmarshal(raw, &body)
			jql = body.JQL
		}
		wantKeys := []string{`"NMB-1"`, `"NMB-2"`, `"` + f.newKey + `"`}
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
			key, id := "NMB-1", "1001"
			switch {
			case strings.Contains(jql, `"NMB-2"`):
				key, id = "NMB-2", "1002"
			case f.newKey != "" && f.newKey != "NMB-1" && strings.Contains(jql, `"`+f.newKey+`"`):
				key = f.newKey
			}
			desc := ""
			if f.description != "" && key == "NMB-1" {
				desc = `"description":` + f.description + `,`
			}
			_, _ = w.Write([]byte(`{"issues":[{"id":"` + id + `","key":"` + key + `","fields":{
			"summary":"batch worker drops the last page",` + desc + `
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
		if f.transitionsJSON != "" {
			_, _ = w.Write([]byte(f.transitionsJSON))
			return
		}
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
	case strings.HasPrefix(path, "/issue/createmeta/") && strings.Contains(path, "/issuetypes/"):
		if f.createFieldsStatus != 0 {
			w.WriteHeader(f.createFieldsStatus)
			_, _ = w.Write([]byte(f.errBody))
			return
		}
		if f.createFieldsJSON != "" {
			_, _ = w.Write([]byte(f.createFieldsJSON))
			return
		}
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"total":4,"fields":[
			{"fieldId":"issuetype","name":"Issue Type","required":true,"hasDefaultValue":false,"schema":{"type":"issuetype"}},
			{"fieldId":"project","name":"Project","required":true,"hasDefaultValue":false,"schema":{"type":"project"}},
			{"fieldId":"reporter","name":"Reporter","required":true,"hasDefaultValue":true,"schema":{"type":"user"}},
			{"fieldId":"summary","name":"Summary","required":true,"hasDefaultValue":false,"schema":{"type":"string"}}
		]}`))
	case path == "/issue/createmeta":
		if f.createMetaJSON != "" {
			_, _ = w.Write([]byte(f.createMetaJSON))
			return
		}
		_, _ = w.Write([]byte(`{"projects":[{"key":"NMB","name":"Numbers","issuetypes":[{"id":"10004","name":"Bug"}]}]}`))
	case path == "/user/search":
		_, _ = w.Write([]byte(`[{"accountId":"acc-cl","displayName":"이클라","emailAddress":"cl@example.com",
			"avatarUrls":{"48x48":"https://a/48.png"},"active":true}]`))
	case path == "/issueLinkType":
		raw := f.linkTypesJSON
		if raw == "" {
			raw = `{"issueLinkTypes":[{"id":"10000","name":"Blocks","outward":"blocks","inward":"is blocked by"}]}`
		}
		_, _ = w.Write([]byte(raw))
	case strings.HasPrefix(path, "/issue/") && r.Method == http.MethodGet && !strings.Contains(strings.TrimPrefix(path, "/issue/"), "/"):
		raw := f.issueStatusJSON
		if raw == "" {
			raw = `{"fields":{"status":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}},
				"assignee":{"accountId":"acc-hc","displayName":"김현철"}}}`
		}
		_, _ = w.Write([]byte(raw))
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

func TestTransitionRESTForwardsFieldsAndComment(t *testing.T) {
	f, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/",
		`{"transition_id":"31","fields":{"resolution":{"id":"10002"}},"comment":"closing out"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	raw := f.bodies["POST /issue/NMB-1/transitions"]
	var sent map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	var fields struct {
		Resolution struct {
			ID string `json:"id"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(sent["fields"], &fields); err != nil {
		t.Fatalf("fields %s: %v", sent["fields"], err)
	}
	if fields.Resolution.ID != "10002" {
		t.Fatalf("resolution id %q", fields.Resolution.ID)
	}
	var update struct {
		Comment []struct {
			Add struct {
				Body json.RawMessage `json:"body"`
			} `json:"add"`
		} `json:"comment"`
	}
	if err := json.Unmarshal(sent["update"], &update); err != nil {
		t.Fatalf("update %s: %v", sent["update"], err)
	}
	if len(update.Comment) != 1 {
		t.Fatalf("comment ops %d; %s", len(update.Comment), sent["update"])
	}
	want := string(jira.Doc("closing out", nil))
	if string(update.Comment[0].Add.Body) != want {
		t.Fatalf("comment ADF %s, want %s", update.Comment[0].Add.Body, want)
	}
}

func TestTransitionRESTWithoutExtrasOmitsFieldsAndUpdate(t *testing.T) {
	f, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/", `{"transition_id":"31"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var wrap struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wrap); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if !wrap.Changed {
		t.Fatalf("write path must report changed=true: %s", rec.Body.String())
	}
	raw := string(f.bodies["POST /issue/NMB-1/transitions"])
	if raw != `{"transition":{"id":"31"}}` {
		t.Fatalf("POST body %q, want exactly {\"transition\":{\"id\":\"31\"}}", raw)
	}
}

func TestTransitionRESTCategoryAlreadyThereIsNoop(t *testing.T) {
	f, h, _ := writable(t)
	f.transitionsJSON = `{"transitions":[]}`
	f.issueStatusJSON = `{"fields":{"status":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},"assignee":null}}`
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/",
		`{"transition_id":"done","comment":"retry"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var wrap struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wrap); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if wrap.Changed {
		t.Fatalf("already done must report changed=false: %s", rec.Body.String())
	}
	if f.called("POST /issue/NMB-1/transitions") {
		t.Fatalf("must not POST; body %s", f.bodies["POST /issue/NMB-1/transitions"])
	}
}

// GDK-341: the REST surface resolves the same identifiers as the CLI —
// target status id, name, category — and refuses garbage with the resolver's
// candidate list. The fake serves one transition: id 31 → 완료 (10001, done).
func TestTransitionRESTResolvesCLIIdentifiers(t *testing.T) {
	for _, ident := range []string{"10001", "완료로", "완료", "done"} {
		f, h, _ := writable(t)
		rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/",
			`{"transition_id":`+strconv.Quote(ident)+`}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: status %d: %s", ident, rec.Code, rec.Body.String())
		}
		if raw := string(f.bodies["POST /issue/NMB-1/transitions"]); raw != `{"transition":{"id":"31"}}` {
			t.Fatalf("%q resolved to %q, want transition 31", ident, raw)
		}
	}

	_, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/", `{"transition_id":"nonsense"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unresolvable identifier: status %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "완료로") {
		t.Fatalf("refusal must name the candidates: %s", rec.Body.String())
	}
}

// requiredResolutionJSON is one done-category transition whose screen requires
// resolution. allowedValues ids differ from GET /resolution so a catalog
// lookup cannot accidentally satisfy the name test (same fixture as CLI).
const requiredResolutionJSON = `{"transitions":[
	{"id":"41","name":"Resolve","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},
	 "fields":{"resolution":{"required":true,"name":"Resolution","schema":{"type":"resolution"},
	   "allowedValues":[{"id":"10099","name":"Won't Do"},{"id":"10000","name":"Done"}]}}}]}`

// GDK-578: REST now shares the CLI core — name/id resolution lookup, required
// screen-field refusal, and field-alias remap.

func TestTransitionRESTResolutionNameUsesAllowedValues(t *testing.T) {
	f, h, _ := writable(t)
	f.transitionsJSON = requiredResolutionJSON
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/",
		`{"transition_id":"done","resolution":"Won't Do"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	raw := f.bodies["POST /issue/NMB-1/transitions"]
	var sent map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	var fields struct {
		Resolution struct {
			ID string `json:"id"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(sent["fields"], &fields); err != nil {
		t.Fatalf("fields %s: %v", sent["fields"], err)
	}
	if fields.Resolution.ID != "10099" {
		t.Fatalf("resolution id %q, want 10099 from allowedValues", fields.Resolution.ID)
	}
}

func TestTransitionRESTRequiredResolutionRefusesWithoutValue(t *testing.T) {
	f, h, _ := writable(t)
	f.transitionsJSON = requiredResolutionJSON
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/", `{"transition_id":"done"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if f.called("POST /issue/NMB-1/transitions") {
		t.Fatalf("must not POST; body %s", f.bodies["POST /issue/NMB-1/transitions"])
	}
	msg := rec.Body.String()
	for _, want := range []string{"resolution", "Won't Do", "Done"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestTransitionRESTFieldAliasRemap(t *testing.T) {
	f, h, cfg := writable(t)
	cfg.Fields = []config.FieldSpec{
		{Alias: "severity", Label: "Severity", IDs: []string{"customfield_10001"}, Role: "facet", Kind: "option"},
	}
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/",
		`{"transition_id":"31","fields":{"severity":"High"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := string(f.bodies["POST /issue/NMB-1/transitions"])
	if !strings.Contains(body, `"customfield_10001"`) {
		t.Fatalf("alias was not resolved to customfield id: %s", body)
	}
	if strings.Contains(body, `"severity"`) {
		t.Fatalf("alias name leaked into transition fields: %s", body)
	}
}

// GDK-341: GET transitions carries to_id so a reader holding issues_full's
// status_id can send it straight back.
func TestTransitionsGETIncludesTargetStatusID(t *testing.T) {
	_, h, _ := writable(t)
	rec := get(t, h, apiBase+"NMB-1/transitions/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"to_id":"10001"`) {
		t.Fatalf("no to_id in %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"fields"`) {
		t.Fatalf("no required screen fields: fields key must be omitted, got %s", rec.Body.String())
	}
}

// GDK-83: GET transitions exposes required screen fields only. Optional fields
// stay off the document; a transition with none omits the fields key.
func TestTransitionsGETRequiredFieldsOnly(t *testing.T) {
	f, h, _ := writable(t)
	f.transitionsJSON = `{"transitions":[
		{"id":"21","name":"Start","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}},
		 "fields":{"summary":{"required":false,"name":"Summary","schema":{"type":"string"}}}},
		{"id":"41","name":"Resolve","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},
		 "fields":{
		   "resolution":{"required":true,"name":"Resolution","schema":{"type":"resolution"},
		     "allowedValues":[{"id":"10099","name":"Won't Do"},{"id":"10000","name":"Done"}]},
		   "customfield_1":{"required":false,"name":"Optional","schema":{"type":"string"}},
		   "customfield_10092":{"required":true,"name":"Severity","schema":{"type":"option"},
		     "allowedValues":[{"id":"10160","value":"High"},{"id":"10161","value":"Low"}]}}}
	]}`
	rec := get(t, h, apiBase+"NMB-1/transitions/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var raw struct {
		Transitions []json.RawMessage `json:"transitions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw.Transitions) != 2 {
		t.Fatalf("transitions %d, want 2: %s", len(raw.Transitions), rec.Body.String())
	}
	if strings.Contains(string(raw.Transitions[0]), `"fields"`) {
		t.Fatalf("optional-only transition must omit fields: %s", raw.Transitions[0])
	}
	var resolve struct {
		ID     string `json:"id"`
		Fields []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Options []struct {
				ID    string `json:"id"`
				Value string `json:"value"`
			} `json:"options"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw.Transitions[1], &resolve); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolve.ID != "41" || len(resolve.Fields) != 2 {
		t.Fatalf("resolve fields %+v, want 2 required", resolve.Fields)
	}
	if resolve.Fields[0].ID != "customfield_10092" || resolve.Fields[0].Name != "Severity" ||
		resolve.Fields[0].Type != "option" || len(resolve.Fields[0].Options) != 2 ||
		resolve.Fields[0].Options[0].ID != "10160" || resolve.Fields[0].Options[0].Value != "High" {
		t.Fatalf("option field %+v", resolve.Fields[0])
	}
	if resolve.Fields[1].ID != "resolution" || resolve.Fields[1].Name != "Resolution" ||
		resolve.Fields[1].Type != "resolution" || len(resolve.Fields[1].Options) != 2 ||
		resolve.Fields[1].Options[0].ID != "10099" || resolve.Fields[1].Options[0].Value != "Won't Do" ||
		resolve.Fields[1].Options[1].ID != "10000" || resolve.Fields[1].Options[1].Value != "Done" {
		t.Fatalf("resolution field %+v", resolve.Fields[1])
	}
	if strings.Contains(string(raw.Transitions[1]), `"customfield_1"`) {
		t.Fatalf("optional field leaked: %s", raw.Transitions[1])
	}
}

// GDK-83: Apply already forwards fields.resolution {id} (same shape the web
// inline form sends). Pin that a required screen is satisfied this way, not
// only via body.resolution.
func TestTransitionRESTRequiredResolutionAcceptsFieldsID(t *testing.T) {
	f, h, _ := writable(t)
	f.transitionsJSON = requiredResolutionJSON
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/transition/",
		`{"transition_id":"41","fields":{"resolution":{"id":"10099"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	raw := f.bodies["POST /issue/NMB-1/transitions"]
	var sent map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	var fields struct {
		Resolution struct {
			ID string `json:"id"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(sent["fields"], &fields); err != nil {
		t.Fatalf("fields %s: %v", sent["fields"], err)
	}
	if fields.Resolution.ID != "10099" {
		t.Fatalf("resolution id %q, want 10099 from fields", fields.Resolution.ID)
	}
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

func TestCommentPassesVisibilityAndInternal(t *testing.T) {
	f, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/",
		`{"text":"restricted","visibility":{"type":"role","value":"Administrators"},"internal":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue/NMB-1/comment"])
	if !strings.Contains(sent, `"visibility"`) || !strings.Contains(sent, `"Administrators"`) {
		t.Fatalf("visibility not sent: %s", sent)
	}
	if !strings.Contains(sent, "sd.public.comment") {
		t.Fatalf("internal property not sent: %s", sent)
	}

	// A bad visibility shape never reaches Jira.
	for _, bad := range []string{
		`{"text":"x","visibility":{"type":"team","value":"a"}}`,
		`{"text":"x","visibility":{"type":"role","value":"  "}}`,
	} {
		if rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad visibility %s → %d", bad, rec.Code)
		}
	}

	// Body-only comments stay byte-identical: no visibility key, no properties.
	f.bodies = map[string]json.RawMessage{}
	if rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", `{"text":"plain"}`); rec.Code != http.StatusOK {
		t.Fatalf("plain comment → %d", rec.Code)
	}
	sent = string(f.bodies["POST /issue/NMB-1/comment"])
	if strings.Contains(sent, `"visibility"`) || strings.Contains(sent, "sd.public.comment") {
		t.Fatalf("plain comment must omit visibility/properties: %s", sent)
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

func TestDescriptionSetAndClear(t *testing.T) {
	f, h, _ := writable(t)
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/description/", `{"description":"plain body"}`); rec.Code != http.StatusOK {
		t.Fatalf("set → %d: %s", rec.Code, rec.Body.String())
	}
	body := string(f.bodies["PUT /issue/NMB-1"])
	for _, want := range []string{`"description"`, `"type":"doc"`, `"text":"plain body"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("ADF wrap missing %s: %s", want, body)
		}
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/description/", `{"description":null}`); rec.Code != http.StatusOK {
		t.Fatalf("null clear → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(got, `"description":null`) {
		t.Fatalf("null body %s", got)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/description/", `{"description":""}`); rec.Code != http.StatusOK {
		t.Fatalf("empty clear → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(got, `"description":null`) {
		t.Fatalf("empty body %s", got)
	}
	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/description/", `{}`); rec.Code != http.StatusBadRequest {
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

func TestFieldEditScalarKinds(t *testing.T) {
	f, h, cfg := writable(t)
	cfg.EditableFields = nil
	cfg.Fields = []config.FieldSpec{
		{Alias: "note", Label: "Note", IDs: []string{"customfield_text"}, Role: "plain", Kind: "text"},
		{Alias: "score", Label: "Score", IDs: []string{"customfield_num"}, Role: "plain", Kind: "number"},
		{Alias: "due_custom", Label: "Custom due", IDs: []string{"customfield_date"}, Role: "plain", Kind: "date"},
	}
	f.editMeta = `{
		"customfield_text": {"schema":{"type":"string","custom":"com.atlassian.jira.plugin.system.customfieldtypes:textfield"}},
		"customfield_num": {"schema":{"type":"number"}},
		"customfield_date": {"schema":{"type":"date"}}
	}`

	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"note","value":"hello"}`); rec.Code != http.StatusOK {
		t.Fatalf("text → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_text":"hello"}}` {
		t.Fatalf("text body %s", got)
	}
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"score","value":42}`); rec.Code != http.StatusOK {
		t.Fatalf("number → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_num":42}}` {
		t.Fatalf("number body %s", got)
	}
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"due_custom","value":"2026-09-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("date → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_date":"2026-09-01"}}` {
		t.Fatalf("date body %s", got)
	}
	if rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"note","value":null}`); rec.Code != http.StatusOK {
		t.Fatalf("text clear → %d", rec.Code)
	}
	if got := string(f.bodies["PUT /issue/NMB-1"]); got != `{"fields":{"customfield_text":null}}` {
		t.Fatalf("text clear body %s", got)
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

func TestComponentsBuiltinEditMetaAndWrite(t *testing.T) {
	f, h, cfg := writable(t)
	cfg.EditableFields = nil
	cfg.Fields = nil
	f.editMeta = `{
		"components": {"schema":{"type":"array","items":"component"},"operations":["set"],
			"allowedValues":[{"id":"10000","name":"Dashboard"},{"id":"10001","name":"API"}]}
	}`

	got := decode[struct {
		Fields map[string]struct {
			Kind     string              `json:"kind"`
			Editable bool                `json:"editable"`
			Options  []map[string]string `json:"options"`
		} `json:"fields"`
	}](t, get(t, h, apiBase+"NMB-1/editmeta/", nil))
	if len(got.Fields) != 1 {
		t.Fatalf("fields %+v", got.Fields)
	}
	c := got.Fields["components"]
	if c.Kind != "component_array" || !c.Editable {
		t.Fatalf("components %+v", c)
	}
	if len(c.Options) != 2 || c.Options[0]["id"] != "10000" || c.Options[0]["value"] != "Dashboard" {
		t.Fatalf("options %+v (name fallback)", c.Options)
	}

	rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"components","value":["10000","10001"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"components":[{"id":"10000"},{"id":"10001"}]}}` {
		t.Fatalf("UpdateFields body %s", body)
	}

	f.editMeta = `{}`
	rec = send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"components","value":["10000"]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing editmeta → %d", rec.Code)
	}
	if err := decode[map[string]string](t, rec)["error"]; err != "field_not_editable" {
		t.Fatalf("error %q", err)
	}
}

func TestParentBuiltinEditMetaAndWrite(t *testing.T) {
	f, h, cfg := writable(t)
	cfg.EditableFields = nil
	cfg.Fields = nil
	f.editMeta = `{
		"parent": {"schema":{"type":"issuelink","system":"parent"},"operations":["set"]}
	}`

	got := decode[struct {
		Fields map[string]struct {
			Kind     string              `json:"kind"`
			Editable bool                `json:"editable"`
			Options  []map[string]string `json:"options"`
		} `json:"fields"`
	}](t, get(t, h, apiBase+"NMB-1/editmeta/", nil))
	if len(got.Fields) != 1 {
		t.Fatalf("fields %+v", got.Fields)
	}
	p := got.Fields["parent"]
	if p.Kind != "parent" || !p.Editable {
		t.Fatalf("parent %+v", p)
	}
	if p.Options == nil {
		t.Fatalf("options nil, want empty array")
	}
	if len(p.Options) != 0 {
		t.Fatalf("options %+v want []", p.Options)
	}

	rec := send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"parent","value":"NMA-2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"parent":{"key":"NMA-2"}}}` {
		t.Fatalf("UpdateFields body %s", body)
	}

	rec = send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"parent","value":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"parent":null}}` {
		t.Fatalf("clear body %s", body)
	}

	f.editMeta = `{}`
	rec = send(t, h, http.MethodPatch, apiBase+"NMB-1/fields/", `{"field":"parent","value":"NMA-2"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing editmeta → %d", rec.Code)
	}
	if err := decode[map[string]string](t, rec)["error"]; err != "field_not_editable" {
		t.Fatalf("error %q", err)
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

// GDK-1034: `gadak init --standalone` leaves projects: null in the home's
// config, and a serve of that home must still accept creates for the seeded
// project. The empty list means "no explicit scope", not deny-all — the
// decision has one owner, Config.ProjectMirrored (GDK-467 semantics), so
// this test pins REST to the same answer the CLI pre-check gives.
func TestCreateIssueEmptyProjectsAllows(t *testing.T) {
	f, h, cfg := writable(t)
	cfg.Projects = nil // the CLI-created local-origin home shape

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"no local scope"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create with empty Projects → %d: %s", rec.Code, rec.Body.String())
	}
	if !f.called("POST /issue") {
		t.Fatalf("empty Projects denied before the origin: %v", f.calls)
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
	// 2026-08-19: retargeted from the CLI prose "pass --type" to the stable
	// wire code. That assertion froze the F6 leak (internal/create composed
	// CLI flag names; REST copied err.Error() into the JSON body). Same 400
	// and no-Jira-call contract; the body now keys on the house-style code
	// the way project_not_mirrored does (fail() in server.go), not on a
	// surface sentence.
	if got := decode[map[string]string](t, rec)["error"]; got != "issue_type_required" {
		t.Errorf("error %q", got)
	}
	if f.called("POST /issue") {
		t.Fatalf("omitted type reached Jira: %v", f.calls)
	}
}

func TestCreateIssueOmitsProjectFailsWhenAmbiguous(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"summary":"needs a project","issue_type":"10004"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "project_required" {
		t.Errorf("error %q", got)
	}
	if f.called("POST /issue") {
		t.Fatalf("omitted project reached Jira: %v", f.calls)
	}
}

// GDK-485: REST sibling of cmd/gadak TestCreatePairedUnreachableIsNotPassProject.
// A dead home serve must not become project_required 400.
func TestCreatePairedUnreachableIsNotProjectRequired(t *testing.T) {
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

	db, cfg := fixture(t)
	// Site/email/token keep HasCredential true; origin.Client still prefers
	// remote-origin.json (pairedRemote is first). DefaultProject empty so
	// omitted project_key is NeedProjectError — the GDK-453 disguise.
	cfg.DefaultProject = ""
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"summary":"hello from a paired workspace"}`)
	body := rec.Body.String()
	if rec.Code == http.StatusOK {
		t.Fatalf("create against a closed home serve must fail: %s", body)
	}
	got := decode[map[string]string](t, rec)["error"]
	if got == "project_required" {
		t.Fatalf("connection failure was folded into project_required: %s", body)
	}
	if !strings.Contains(got, "cannot reach the home serve") {
		t.Fatalf("want pairing unreachable sentence, got %q", got)
	}
	if strings.HasPrefix(got, "GET ") || strings.HasPrefix(got, "POST ") {
		t.Fatalf("REST method leaked onto the error: %q", got)
	}
}

func TestPairedWrite401IsPairingErrorNotCredentialRejected(t *testing.T) {
	// GDK-543: a paired-transport 401 on a mutate must surface as the pairing
	// sentence, not credential_rejected (which opens the Jira-token dialog).
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Gadak-Pairing", "revoked")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"pairing_rejected","reason":"revoked"}`))
	}))
	t.Cleanup(srv.Close)
	if err := pairing.SaveRemote(home, pairing.Remote{Endpoint: srv.URL, Token: "pair-token", Label: "laptop"}); err != nil {
		t.Fatal(err)
	}

	db, cfg := fixture(t)
	h := New(db, cfg)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", `{"text":"hello from a paired workspace"}`)
	body := rec.Body.String()
	if strings.Contains(body, "credential_rejected") {
		t.Fatalf("paired 401 mapped to credential_rejected: %s", body)
	}
	if !strings.Contains(body, "pairing:") {
		t.Fatalf("want pairing sentence, got %q", body)
	}
}

// cliFlagToken matches a GNU-style long option in a REST body. The class is
// "any token starting with --", not today's three create cases: a future
// shared-package error that names a flag must fail here too.
var cliFlagToken = regexp.MustCompile(`--[A-Za-z][A-Za-z0-9_-]*`)

func TestCreateRESTErrorBodiesHaveNoCLIFlagTokens(t *testing.T) {
	f, h, cfg := writable(t)
	f.createMetaJSON = manyCreateTypes
	cfg.DefaultProject = ""
	cfg.DefaultIssueTypeID = ""

	cases := []struct {
		name, body string
	}{
		{"omitted project", `{"summary":"x","issue_type":"10004"}`},
		{"omitted type", `{"project_key":"NMB","summary":"needs a type"}`},
		{"unmatched type", `{"project_key":"NMB","issue_type":"Nope","summary":"x"}`},
		{"unmirrored project", `{"project_key":"ZZZ","issue_type":"10004","summary":"x"}`},
		{"empty summary", `{"project_key":"NMB","issue_type":"10004"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := send(t, h, http.MethodPost, apiBase+"create/", tc.body)
			if rec.Code < 400 {
				t.Fatalf("want 4xx, got %d %s", rec.Code, rec.Body.String())
			}
			if hits := cliFlagToken.FindAllString(rec.Body.String(), -1); len(hits) > 0 {
				t.Errorf("REST body contains CLI flag token(s) %v: %s", hits, rec.Body.String())
			}
		})
	}

	// Stale configured default is another create 400; include it so a future
	// rewrite of that path cannot sneak a -- token in.
	cfg.DefaultIssueTypeID = "99999"
	rec := send(t, h, http.MethodPost, apiBase+"create/", `{"project_key":"NMB","summary":"stale"}`)
	if rec.Code < 400 {
		t.Fatalf("stale default → %d %s", rec.Code, rec.Body.String())
	}
	if hits := cliFlagToken.FindAllString(rec.Body.String(), -1); len(hits) > 0 {
		t.Errorf("stale-default REST body contains CLI flag token(s) %v: %s", hits, rec.Body.String())
	}

	// REST create does not call create.Priority today (priority_id is an id).
	// failCreate still owns that Need* type so a future caller cannot dump
	// "--priority" onto the wire.
	recP := httptest.NewRecorder()
	failCreate(recP, &create.NeedPriorityError{Available: []jira.NamedID{{ID: "1", Name: "Highest"}}})
	if recP.Code < 400 {
		t.Fatalf("need priority → %d %s", recP.Code, recP.Body.String())
	}
	if hits := cliFlagToken.FindAllString(recP.Body.String(), -1); len(hits) > 0 {
		t.Errorf("need-priority body contains CLI flag token(s) %v: %s", hits, recP.Body.String())
	}
	if got := decode[map[string]string](t, recP)["error"]; got != "priority_required" {
		t.Errorf("need-priority code %q", got)
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
		`{"project_key":"NMB","issue_type":"","summary":"omit empties","description_text":"","priority_id":"","labels":[]}`)
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

func TestCreateIssueSendsPriorityByID(t *testing.T) {
	f, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"with pri","priority_id":"2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"priority":{"id":"2"}`) {
		t.Fatalf("priority id missing: %s", sent)
	}
	if strings.Contains(sent, `"priority":{"name"`) {
		t.Fatalf("priority sent by name: %s", sent)
	}
}

func TestCreateIssueIgnoresPriorityNameField(t *testing.T) {
	f, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"old name","priority":"High"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if strings.Contains(sent, `"priority"`) {
		t.Fatalf("legacy priority name must not reach Jira: %s", sent)
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

// GDK-740: routing refresh failures through failMirrorStale must keep the
// emitted status and write_applied_mirror_stale wire code byte-identical.
func TestWriteAppliedMirrorStaleStatusUnchanged(t *testing.T) {
	cases := []struct {
		name  string
		run   func(t *testing.T, f *fakeJira, h http.Handler) *httptest.ResponseRecorder
		wrote func(t *testing.T, f *fakeJira)
	}{
		{
			name: "mutate comment",
			run: func(t *testing.T, f *fakeJira, h http.Handler) *httptest.ResponseRecorder {
				return send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", `{"text":"hi"}`)
			},
			wrote: func(t *testing.T, f *fakeJira) {
				if !f.called("POST /issue/NMB-1/comment") {
					t.Fatalf("comment never reached Jira: %v", f.calls)
				}
			},
		},
		{
			name: "create",
			run: func(t *testing.T, f *fakeJira, h http.Handler) *httptest.ResponseRecorder {
				return send(t, h, http.MethodPost, apiBase+"create/",
					`{"project_key":"NMB","issue_type":"10004","summary":"stale reread"}`)
			},
			wrote: func(t *testing.T, f *fakeJira) {
				if !f.called("POST /issue") {
					t.Fatalf("create never reached Jira: %v", f.calls)
				}
			},
		},
		{
			name: "upload",
			run: func(t *testing.T, f *fakeJira, h http.Handler) *httptest.ResponseRecorder {
				var buf bytes.Buffer
				mw := multipart.NewWriter(&buf)
				part, _ := mw.CreateFormFile("file", "shot.png")
				_, _ = part.Write([]byte("PNGBYTES"))
				_ = mw.Close()
				req := testRequest(http.MethodPost, apiBase+"NMB-1/attachments/", &buf)
				req.Header.Set("Content-Type", mw.FormDataContentType())
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				return rec
			},
			wrote: func(t *testing.T, f *fakeJira) {
				if !f.called("POST /issue/NMB-1/attachments") {
					t.Fatalf("upload never reached Jira: %v", f.calls)
				}
			},
		},
		{
			name: "link",
			run: func(t *testing.T, f *fakeJira, h http.Handler) *httptest.ResponseRecorder {
				return send(t, h, http.MethodPost, apiBase+"NMB-1/link/", `{"type":"blocks","key":"NMB-2"}`)
			},
			wrote: func(t *testing.T, f *fakeJira) {
				if !f.called("POST /issueLink") {
					t.Fatalf("link never reached Jira: %v", f.calls)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, h, _ := writable(t)
			f.rereadStatus = http.StatusUnprocessableEntity
			rec := tc.run(t, f, h)
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status %d %s, want 502", rec.Code, rec.Body.String())
			}
			if got := decode[map[string]string](t, rec)["error"]; got != "write_applied_mirror_stale" {
				t.Fatalf("error %q, want write_applied_mirror_stale", got)
			}
			tc.wrote(t, f)
		})
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

// TestCreateMetaForwardsSubtaskAndHierarchyLevel is GDK-329: the origin
// sends both fields on issuetypes; the REST create-meta/ answer must not
// drop them. False/0 are omitted (same omitempty as NamedID.value).
func TestCreateMetaForwardsSubtaskAndHierarchyLevel(t *testing.T) {
	f, h, _ := writable(t)
	f.createMetaJSON = `{"projects":[{"key":"NMB","name":"Numbers","issuetypes":[
		{"id":"10001","name":"에픽","subtask":false,"hierarchyLevel":1},
		{"id":"10004","name":"버그","subtask":false,"hierarchyLevel":0},
		{"id":"10005","name":"하위 작업","subtask":true,"hierarchyLevel":-1}
	]}]}`

	rec := get(t, h, apiBase+"create-meta/", nil)
	t.Logf("create-meta JSON: %s", rec.Body.Bytes())
	meta := decode[struct {
		Projects []struct {
			Key        string           `json:"key"`
			IssueTypes []map[string]any `json:"issue_types"`
		} `json:"projects"`
	}](t, rec)
	if len(meta.Projects) != 1 || meta.Projects[0].Key != "NMB" || len(meta.Projects[0].IssueTypes) != 3 {
		t.Fatalf("create-meta %+v", meta.Projects)
	}
	byID := map[string]map[string]any{}
	for _, it := range meta.Projects[0].IssueTypes {
		id, _ := it["id"].(string)
		byID[id] = it
	}

	epic := byID["10001"]
	if epic["name"] != "에픽" {
		t.Errorf("epic name %v", epic["name"])
	}
	if _, ok := epic["subtask"]; ok {
		t.Errorf("epic subtask=false must omit: %v", epic)
	}
	if epic["hierarchyLevel"] != float64(1) {
		t.Errorf("epic hierarchyLevel=%v want 1", epic["hierarchyLevel"])
	}

	bug := byID["10004"]
	if _, ok := bug["subtask"]; ok {
		t.Errorf("standard subtask=false must omit: %v", bug)
	}
	if _, ok := bug["hierarchyLevel"]; ok {
		t.Errorf("standard hierarchyLevel=0 must omit: %v", bug)
	}

	sub := byID["10005"]
	if sub["subtask"] != true {
		t.Errorf("sub-task subtask=%v want true", sub["subtask"])
	}
	if sub["hierarchyLevel"] != float64(-1) {
		t.Errorf("sub-task hierarchyLevel=%v want -1", sub["hierarchyLevel"])
	}

	wm := decode[map[string]json.RawMessage](t, get(t, h, apiBase+"meta/write/", nil))
	var createMeta struct {
		Projects []struct {
			IssueTypes []map[string]any `json:"issue_types"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(wm["create_meta"], &createMeta); err != nil {
		t.Fatalf("meta/write create_meta: %v %s", err, wm["create_meta"])
	}
	if len(createMeta.Projects) != 1 || len(createMeta.Projects[0].IssueTypes) != 3 {
		t.Fatalf("meta/write create_meta types: %+v", createMeta)
	}
	foundSub := false
	for _, it := range createMeta.Projects[0].IssueTypes {
		if it["id"] == "10005" {
			foundSub = true
			if it["subtask"] != true || it["hierarchyLevel"] != float64(-1) {
				t.Errorf("meta/write sub-task %v", it)
			}
		}
	}
	if !foundSub {
		t.Fatalf("meta/write dropped types: %s", wm["create_meta"])
	}
}

func TestWritesRequireACredential(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{Projects: []string{"NMB"}})
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, apiBase + "NMB-1/transitions/", ""},
		{http.MethodPost, apiBase + "NMB-1/transition/", `{"transition_id":"31"}`},
		{http.MethodPost, apiBase + "NMB-1/comment/", `{"text":"hi"}`},
		{http.MethodGet, apiBase + "NMB-1/linktypes/", ""},
		{http.MethodPost, apiBase + "NMB-1/link/", `{"type":"blocks","key":"NMB-2"}`},
		{http.MethodPut, apiBase + "NMB-1/assignee/", `{"account_id":null}`},
		{http.MethodPut, apiBase + "NMB-1/labels/", `{"labels":["x"]}`},
		{http.MethodPut, apiBase + "NMB-1/priority/", `{"priority_id":"2"}`},
		{http.MethodPut, apiBase + "NMB-1/summary/", `{"summary":"x"}`},
		{http.MethodPut, apiBase + "NMB-1/description/", `{"description":"x"}`},
		{http.MethodPut, apiBase + "NMB-1/duedate/", `{"duedate":"2026-09-01"}`},
		{http.MethodGet, apiBase + "priorities/", ""},
		{http.MethodGet, apiBase + "NMB-1/priorities/", ""},
		{http.MethodGet, apiBase + "NMB-1/users/?q=a", ""},
		{http.MethodPatch, apiBase + "NMB-1/fields/", `{"field":"solution","value":"1"}`},
		{http.MethodGet, apiBase + "NMB-1/editmeta/", ""},
		{http.MethodPost, apiBase + "create/", `{"project_key":"NMB","issue_type":"1","summary":"x"}`},
		{http.MethodGet, apiBase + "create-meta/", ""},
		{http.MethodGet, apiBase + "create-meta/fields/?project=NMB&issue_type=10004", ""},
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

// GDK-455: an empty token on a configured workspace means "keep the stored
// one" — parity with `gadak init`, which keeps credentials on empty answers.
// The web settings form can then submit an expiry-only change without asking
// the user to re-paste a token they already stored.
func TestPutCredentialKeepsStoredTokenOnEmpty(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = f.URL, "hc@example.com", "tok-SECRET-1234"
	h := New(db, cfg)

	// Expiry-only save: email and token both empty, stored credential kept.
	rec := send(t, h, http.MethodPut, apiBase+"credential/",
		`{"jira_email":"","api_token":"","token_expires_at":"2027-06-15"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expiry-only put → %d: %s", rec.Code, rec.Body.String())
	}
	if !f.called("GET /myself") {
		t.Fatal("kept credential stored without re-verifying it")
	}
	got := decode[credentialDoc](t, rec)
	if got.TokenHint != "…1234" || got.JiraEmail != "hc@example.com" {
		t.Fatalf("kept credential answered %+v", got)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Token != "tok-SECRET-1234" || saved.Email != "hc@example.com" {
		t.Fatalf("stored credential changed: email=%q token=%q", saved.Email, saved.Token)
	}
	if saved.TokenExpirySource != config.TokenExpirySourceUser || saved.TokenExpiresAt != "2027-06-15T00:00:00.000Z" {
		t.Fatalf("user expiry: source=%q at=%q", saved.TokenExpirySource, saved.TokenExpiresAt)
	}

	// Keeping the token without a new expiry must not reset the stored date
	// (same contract as init's ApplyTokenExpiryIfNeeded).
	rec = send(t, h, http.MethodPut, apiBase+"credential/", `{"jira_email":"","api_token":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("keep-only put → %d: %s", rec.Code, rec.Body.String())
	}
	saved, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.TokenExpirySource != config.TokenExpirySourceUser || saved.TokenExpiresAt != "2027-06-15T00:00:00.000Z" {
		t.Fatalf("keep-on-empty reset the expiry: source=%q at=%q", saved.TokenExpirySource, saved.TokenExpiresAt)
	}

	// Unconfigured stays guarded: with nothing stored, empty still means empty.
	t.Setenv("GADAK_HOME", t.TempDir())
	db2, cfg2 := fixture(t)
	cfg2.Site, cfg2.Email, cfg2.Token = f.URL, "", ""
	h2 := New(db2, cfg2)
	rec = send(t, h2, http.MethodPut, apiBase+"credential/", `{"jira_email":"","api_token":"","token_expires_at":"2027-06-15"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unconfigured empty put → %d, want 400", rec.Code)
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "email_and_token_required" {
		t.Fatalf("error %q", got)
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

func TestCreateIssueSendsDuedate(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"has due","duedate":"2026-09-01"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"duedate":"2026-09-01"`) {
		t.Fatalf("create body missing duedate: %s", sent)
	}
}

func TestCreateIssueOmitsEmptyDuedate(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"no due","duedate":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if strings.Contains(sent, `"duedate"`) {
		t.Fatalf("empty duedate sent: %s", sent)
	}
}

func TestCreateIssueRejectsInvalidDuedateBeforeJira(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"bad due","duedate":"01/09/2026"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid duedate → %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "YYYY-MM-DD") {
		t.Fatalf("must say what is expected: %s", body)
	}
	if !strings.Contains(body, "01/09/2026") {
		t.Fatalf("must name the value: %s", body)
	}
	if f.called("POST /issue") {
		t.Fatalf("invalid duedate reached Jira: %v", f.calls)
	}
}

func TestEditDuedateSetAndClear(t *testing.T) {
	f, h, _ := writable(t)

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/duedate/", `{"duedate":"2026-09-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("set → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(body, `"duedate":"2026-09-01"`) {
		t.Fatalf("set body %s", body)
	}

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/duedate/", `{"duedate":null}`); rec.Code != http.StatusOK {
		t.Fatalf("clear → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(body, `"duedate":null`) {
		t.Fatalf("clear body %s", body)
	}

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/duedate/", `{"duedate":""}`); rec.Code != http.StatusOK {
		t.Fatalf("empty clear → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); !strings.Contains(body, `"duedate":null`) {
		t.Fatalf("empty body %s", body)
	}
}

func TestEditDuedateRejectsInvalidBeforeJira(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/duedate/", `{"duedate":"September 1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid → %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "YYYY-MM-DD") {
		t.Fatalf("must say what is expected: %s", rec.Body.String())
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("invalid duedate reached Jira: %v", f.calls)
	}
}

func TestCreateIssueSendsParent(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"has parent","parent":"NMB-1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"parent":{"key":"NMB-1"}`) {
		t.Fatalf("create body missing parent: %s", sent)
	}
}

func TestCreateIssueOmitsEmptyParent(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"no parent","parent":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if strings.Contains(sent, `"parent"`) {
		t.Fatalf("empty parent sent: %s", sent)
	}

	rec = send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"blank parent","parent":"  "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create whitespace → %d: %s", rec.Code, rec.Body.String())
	}
	sent = string(f.bodies["POST /issue"])
	if strings.Contains(sent, `"parent"`) {
		t.Fatalf("whitespace parent sent: %s", sent)
	}
}

func TestCreateIssueRejectsInvalidParentBeforeJira(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"bad parent","parent":"not-a-key"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid parent → %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `not a Jira key (want ABC-123)`) {
		t.Fatalf("must say what is expected: %s", body)
	}
	if !strings.Contains(body, "not-a-key") {
		t.Fatalf("must name the value: %s", body)
	}
	if f.called("POST /issue") {
		t.Fatalf("invalid parent reached Jira: %v", f.calls)
	}
}

func TestCreateIssueNormalizesParentKey(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"lower parent","parent":"gdk-1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	sent := string(f.bodies["POST /issue"])
	if !strings.Contains(sent, `"parent":{"key":"GDK-1"}`) {
		t.Fatalf("create parent not normalized: %s", sent)
	}
}

func TestEditParentSetAndClear(t *testing.T) {
	f, h, _ := writable(t)

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"NMB-2"}`); rec.Code != http.StatusOK {
		t.Fatalf("set → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"parent":{"key":"NMB-2"}}}` {
		t.Fatalf("set body %s", body)
	}

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":null}`); rec.Code != http.StatusOK {
		t.Fatalf("clear → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"parent":null}}` {
		t.Fatalf("clear body %s", body)
	}

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":""}`); rec.Code != http.StatusOK {
		t.Fatalf("empty clear → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"parent":null}}` {
		t.Fatalf("empty body %s", body)
	}

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"  "}`); rec.Code != http.StatusOK {
		t.Fatalf("whitespace clear → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"parent":null}}` {
		t.Fatalf("whitespace body %s", body)
	}

	if rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"gdk-1"}`); rec.Code != http.StatusOK {
		t.Fatalf("lower set → %d: %s", rec.Code, rec.Body.String())
	}
	if body := string(f.bodies["PUT /issue/NMB-1"]); body != `{"fields":{"parent":{"key":"GDK-1"}}}` {
		t.Fatalf("normalized body %s", body)
	}
}

func TestEditParentRejectsInvalidBeforeJira(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"not-a-key"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid → %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `not a Jira key (want ABC-123)`) {
		t.Fatalf("must say what is expected: %s", body)
	}
	if !strings.Contains(body, "not-a-key") {
		t.Fatalf("must name the value: %s", body)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("invalid parent reached Jira: %v", f.calls)
	}
}

func TestEditParentRejectsSelfBeforeJira(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"NMB-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self → %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NMB-1") {
		t.Fatalf("must name the value: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "is this issue") {
		t.Fatalf("must say it is this issue: %s", rec.Body.String())
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("self parent reached Jira: %v", f.calls)
	}

	rec = send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"nmb-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self lower → %d: %s", rec.Code, rec.Body.String())
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("self parent (lower) reached Jira: %v", f.calls)
	}
}

// Cloud parent-rejection bodies measured 2026-08-21 (CLI parent_hint_test.go).
// REST used to pass them through failJira with no mirror hierarchy hint (GDK-635).
const (
	restCreateParent400 = `{"errors":{"parent":"유효한 상위 업무를 선택하세요.","parentId":"유효한 상위 업무를 선택하세요."}}`
	restEditParent400   = `{"errors":{"pid":"이 이슈 유형의 이슈는 상위 이슈와 같은 프로젝트에 만들어야 합니다."}}`
)

func decodeParentReject(t *testing.T, rec *httptest.ResponseRecorder) (errorText string, jiraErrors map[string]string) {
	t.Helper()
	var body struct {
		Error      string            `json:"error"`
		JiraErrors map[string]string `json:"jira_errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	return body.Error, body.JiraErrors
}

// TestRESTParentRejectionCreateCarriesHierarchyHint is FAIL-first for GDK-635:
// POST create/ used to answer the origin 400 with no mirror hierarchy line.
func TestRESTParentRejectionCreateCarriesHierarchyHint(t *testing.T) {
	f, h, _ := writable(t)
	f.status = http.StatusBadRequest
	f.errBody = restCreateParent400

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"child","parent":"NMB-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got, jiraErrors := decodeParentReject(t, rec)
	if !strings.Contains(got, "유효한 상위 업무를 선택하세요.") {
		t.Fatalf("origin 400 missing from error: %q", got)
	}
	if !strings.Contains(got, "hint:") {
		t.Fatalf("missing hierarchy hint: %q", got)
	}
	if !strings.Contains(got, "NMB-1") {
		t.Fatalf("hint must name the parent: %q", got)
	}
	if jiraErrors["parent"] == "" && jiraErrors["parentId"] == "" {
		t.Fatalf("jira_errors dropped: %+v", jiraErrors)
	}
}

// TestRESTParentRejectionEditCarriesHierarchyHint is FAIL-first for GDK-635:
// PUT {key}/parent/ used to answer the origin 400 with no mirror hierarchy line.
func TestRESTParentRejectionEditCarriesHierarchyHint(t *testing.T) {
	f, h, _ := writable(t)
	f.status = http.StatusBadRequest
	f.errBody = restEditParent400

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"NMB-2"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got, jiraErrors := decodeParentReject(t, rec)
	if !strings.Contains(got, "pid:") && !strings.Contains(got, "같은 프로젝트") {
		t.Fatalf("origin 400 missing from error: %q", got)
	}
	if !strings.Contains(got, "hint:") {
		t.Fatalf("missing hierarchy hint: %q", got)
	}
	if !strings.Contains(got, "NMB-2") {
		t.Fatalf("hint must name the parent: %q", got)
	}
	if jiraErrors["pid"] == "" {
		t.Fatalf("jira_errors dropped: %+v", jiraErrors)
	}
}

func TestRESTParentRejectionUnrelated400HasNoHint(t *testing.T) {
	f, h, _ := writable(t)
	f.status = http.StatusBadRequest
	f.errBody = `{"errors":{"summary":"You must specify a summary."}}`

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/parent/", `{"parent":"NMB-2"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := decodeParentReject(t, rec)
	if strings.Contains(got, "hint:") {
		t.Fatalf("unrelated 400 grew a parent hint: %q", got)
	}
	if !strings.Contains(got, "You must specify a summary.") {
		t.Fatalf("origin 400 missing: %q", got)
	}
}

func TestLocalOriginOriginSecondSessionWrites(t *testing.T) {
	h, cfg := localOriginServer(t)
	if _, err := origin.Client(cfg); err != nil {
		t.Fatal(err)
	}
	origin.ForgetLive()

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", `{"text":"hi"}`)
	if rec.Code == http.StatusConflict {
		t.Fatalf("second session 409 after lock removal: %s", rec.Body.String())
	}
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		// NMB-1 may not exist on a fresh local-origin origin; the point is
		// the mapper did not answer workspace_busy or credential_required.
		var body struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Error == "workspace_busy" || body.Error == "credential_required" {
			t.Fatalf("second session error %q: %s", body.Error, rec.Body.String())
		}
	}
}

// TestLocalOriginOriginPersistFailureIsNotCredentialRequired: a broken
// persist path used to 409 credential_required. It is a 5xx with the
// original error, not a missing token.
//
// FAIL-first (2026-08-20, pre-fix): body error was credential_required.
func TestLocalOriginOriginPersistFailureIsNotCredentialRequired(t *testing.T) {
	h, cfg := localOriginServer(t)
	persist := origin.PersistPath(cfg.Directory())
	if persist == "" {
		t.Fatal("local-origin fixture has no persist path")
	}
	if err := os.MkdirAll(filepath.Dir(persist), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(persist)
	if err := os.Mkdir(persist, 0o700); err != nil {
		t.Fatal(err)
	}

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/comment/", `{"text":"hi"}`)
	if rec.Code < 500 || rec.Code > 599 {
		t.Fatalf("status %d, want 5xx: %s", rec.Code, rec.Body.String())
	}
	got := decode[map[string]string](t, rec)["error"]
	if got == "credential_required" {
		t.Fatal("local-origin persist failure disguised as credential_required")
	}
}

const (
	wikiSimpleADF  = `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"plain"}]}]}`
	wikiComplexADF = `{"type":"doc","version":1,"content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Steps"}]},` +
		`{"type":"panel","attrs":{"panelType":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"note"}]}]}]}` // GDK-1384: a heading alone is markdown now; the panel is what a text replace would lose
)

type wikiPageState struct {
	Title   string
	Space   string
	ADF     string
	Version int
	When    string
}

type wikiCommentState struct {
	ID   string
	ADF  string
	When string
}

// wikiOrigin is a Confluence stand-in for wiki write-through tests: GET/PUT
// content/{id}, POST content (page or comment), and child comments.
type wikiOrigin struct {
	*httptest.Server
	pages         map[string]*wikiPageState
	comments      map[string][]wikiCommentState
	nextID        int
	puts          []int // version.number values received on PUT
	creates       int
	commentPosts  int
	lastPutADF    string
	lastCreateADF string
}

func newWikiOrigin(t *testing.T) *wikiOrigin {
	t.Helper()
	w := &wikiOrigin{
		pages: map[string]*wikiPageState{
			"100": {
				Title: "빌링 품질 회의록", Space: "PROD", ADF: wikiComplexADF,
				Version: 5, When: "2026-08-05T15:00:00.000Z",
			},
			"200": {
				Title: "Architecture", Space: "ENG", ADF: wikiSimpleADF,
				Version: 1, When: "2026-07-15T00:00:00.000Z",
			},
		},
		comments: map[string][]wikiCommentState{},
		nextID:   900,
	}
	w.Server = httptest.NewServer(http.HandlerFunc(w.route))
	t.Cleanup(w.Close)
	return w
}

func (w *wikiOrigin) route(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost && path == "/wiki/rest/api/content":
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		typ, _ := payload["type"].(string)
		if typ == "comment" {
			w.commentPosts++
			container, _ := payload["container"].(map[string]any)
			pageID, _ := container["id"].(string)
			if _, ok := w.pages[pageID]; !ok {
				rw.WriteHeader(http.StatusNotFound)
				_, _ = rw.Write([]byte(`{"statusCode":404,"message":"content not found"}`))
				return
			}
			adf := wikiADFFromBody(payload)
			id := fmt.Sprintf("c-%d", w.nextID)
			w.nextID++
			w.comments[pageID] = append(w.comments[pageID], wikiCommentState{
				ID: id, ADF: adf, When: "2026-08-20T12:00:00.000Z",
			})
			_ = json.NewEncoder(rw).Encode(wikiCommentJSON(id, adf, "2026-08-20T12:00:00.000Z"))
			return
		}
		w.creates++
		w.lastCreateADF = wikiADFFromBody(payload)
		title, _ := payload["title"].(string)
		space, _ := payload["space"].(map[string]any)
		spaceKey, _ := space["key"].(string)
		id := fmt.Sprintf("%d", w.nextID)
		w.nextID++
		p := &wikiPageState{
			Title: title, Space: spaceKey, ADF: w.lastCreateADF,
			Version: 1, When: "2026-08-20T12:00:00.000Z",
		}
		w.pages[id] = p
		_ = json.NewEncoder(rw).Encode(wikiPageJSON(id, p))
		return
	case strings.HasSuffix(path, "/child/comment"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/wiki/rest/api/content/"), "/child/comment")
		results := []any{}
		for _, c := range w.comments[id] {
			results = append(results, wikiCommentJSON(c.ID, c.ADF, c.When))
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{"results": results, "size": len(results), "limit": 100})
		return
	}
	if !strings.HasPrefix(path, "/wiki/rest/api/content/") {
		http.NotFound(rw, r)
		return
	}
	id := strings.TrimPrefix(path, "/wiki/rest/api/content/")
	if strings.Contains(id, "/") {
		http.NotFound(rw, r)
		return
	}
	p, ok := w.pages[id]
	if !ok {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte(`{"statusCode":404,"message":"content not found"}`))
		return
	}
	if r.Method == http.MethodPut {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		verObj, _ := payload["version"].(map[string]any)
		num, _ := verObj["number"].(float64)
		gotVer := int(num)
		w.puts = append(w.puts, gotVer)
		if gotVer != p.Version+1 {
			rw.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"statusCode": 409,
				"message":    fmt.Sprintf("Version must be incremented on update. Current version is: %d", p.Version),
			})
			return
		}
		if title, _ := payload["title"].(string); title != "" {
			p.Title = title
		}
		p.ADF = wikiADFFromBody(payload)
		w.lastPutADF = p.ADF
		p.Version = gotVer
		p.When = "2026-08-20T12:01:00.000Z"
	}
	_ = json.NewEncoder(rw).Encode(wikiPageJSON(id, p))
}

func wikiADFFromBody(payload map[string]any) string {
	body, _ := payload["body"].(map[string]any)
	adf, _ := body["atlas_doc_format"].(map[string]any)
	v, _ := adf["value"].(string)
	return v
}

func wikiPageJSON(id string, p *wikiPageState) map[string]any {
	return map[string]any{
		"id": id, "type": "page", "status": "current", "title": p.Title,
		"space": map[string]any{"key": p.Space, "name": p.Space},
		"version": map[string]any{
			"number": p.Version, "when": p.When,
			"by": map[string]any{"accountId": "acc-1", "displayName": "김현철"},
		},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{"value": p.ADF, "representation": "atlas_doc_format"},
		},
		"ancestors": []any{},
		"metadata": map[string]any{
			"labels": map[string]any{"results": []any{}, "size": 0, "limit": 25, "start": 0},
		},
	}
}

func wikiCommentJSON(id, adf, when string) map[string]any {
	return map[string]any{
		"id": id, "type": "comment", "title": "Re:",
		"body": map[string]any{
			"atlas_doc_format": map[string]any{"value": adf, "representation": "atlas_doc_format"},
		},
		"version": map[string]any{
			"number": 1, "when": when,
			"by": map[string]any{"accountId": "acc-2", "displayName": "Dana"},
		},
	}
}

func wikiWritable(t *testing.T) (*wikiOrigin, http.Handler, *config.Config) {
	t.Helper()
	origin := newWikiOrigin(t)
	db, cfg := fixturePages(t)
	cfg.Site = origin.URL
	return origin, New(db, cfg), cfg
}

func wikiErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[map[string]string](t, rec)["error"]
}

// TestFailJiraMapsOriginHTTP is FAIL-first for GDK-410 + GDK-404: confluence
// and Linear sentinels used to fall through to 502 jira_unavailable.
func TestFailJiraMapsOriginHTTP(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, apiBase+"pages/1/edit/", nil)
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
	}{
		{"jira.ErrAuth", jira.ErrAuth, http.StatusConflict, "credential_rejected"},
		{"confluence.ErrAuth", confluence.ErrAuth, http.StatusConflict, "credential_rejected"},
		{"wrapped confluence.ErrAuth", fmt.Errorf("GET /content/1: %w", confluence.ErrAuth), http.StatusConflict, "credential_rejected"},
		{"linear.ErrAuth", linear.ErrAuth, http.StatusConflict, "credential_rejected"},
		{"sync.ErrNotFound", sync.ErrNotFound, http.StatusNotFound, "not_found"},
		{"confluence.ErrNotFound", confluence.ErrNotFound, http.StatusNotFound, "not_found"},
		{"confluence 409", &confluence.APIError{Status: http.StatusConflict, Body: "stale version"}, http.StatusConflict, "origin_rejected"},
		{"wrapped confluence 409", fmt.Errorf("PUT /content/1: %w", &confluence.APIError{Status: http.StatusConflict, Body: "stale version"}), http.StatusConflict, "origin_rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			failJira(rec, req, nil, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			got := wikiErrorCode(t, rec)
			if got != tc.wantError {
				t.Fatalf("error %q, want %q; body %s", got, tc.wantError, rec.Body.String())
			}
			if tc.wantError == "origin_rejected" {
				msg := decode[map[string]string](t, rec)["message"]
				if !strings.Contains(msg, "stale version") {
					t.Fatalf("message %q, want the origin snippet", msg)
				}
			}
		})
	}
}

// GDK-685: capability refusals and transition.IsRefused must 400 with the
// origin/resolver sentence, not 502 jira_unavailable. Transport stays 502.
func TestFailJiraMapsUnsupportedAndRefused(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, apiBase+"ISSUE-1/edit/", nil)
	unsupportedSentence := `linear: field "labels" is not editable on this origin`
	refusedSentence := "no transition matching nonsense"
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
		contains   string
	}{
		{
			name:       "ErrUnsupported keeps the origin sentence",
			err:        fmt.Errorf("%s: %w", unsupportedSentence, origin.ErrUnsupported),
			wantStatus: http.StatusBadRequest,
			contains:   unsupportedSentence,
		},
		{
			name:       "ErrNoIssueLinks is ErrUnsupported",
			err:        origin.ErrNoIssueLinks,
			wantStatus: http.StatusBadRequest,
			wantError:  "linear: issue links are not supported on this origin",
		},
		{
			name:       "transition.IsRefused without jira.APIError wrap",
			err:        &transition.Refused{Msg: refusedSentence},
			wantStatus: http.StatusBadRequest,
			contains:   refusedSentence,
		},
		{
			name:       "transport stays 502 jira_unavailable",
			err:        fmt.Errorf("connection refused"),
			wantStatus: http.StatusBadGateway,
			wantError:  "jira_unavailable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			failJira(rec, req, nil, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			got := wikiErrorCode(t, rec)
			if tc.wantError != "" && got != tc.wantError {
				t.Fatalf("error %q, want %q; body %s", got, tc.wantError, rec.Body.String())
			}
			if tc.contains != "" && !strings.Contains(got, tc.contains) {
				t.Fatalf("error %q, want it to contain %q; body %s", got, tc.contains, rec.Body.String())
			}
			if tc.wantStatus == http.StatusBadRequest && got == "jira_unavailable" {
				t.Fatalf("capability/refusal collapsed to jira_unavailable: %s", rec.Body.String())
			}
		})
	}
}

func TestPageCreateUpdatesMirrorAfterOrigin(t *testing.T) {
	wo, h, _ := wikiWritable(t)
	rec := send(t, h, http.MethodPost, apiBase+"pages/",
		`{"space":"PROD","title":"Wiki create probe","text":"hello from create"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if wo.creates == 0 {
		t.Fatal("origin CreatePage was not called")
	}
	var body struct {
		Page struct {
			Key   string `json:"key"`
			Title string `json:"title"`
		} `json:"page"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Page.Title != "Wiki create probe" {
		t.Fatalf("title %q", body.Page.Title)
	}
	if body.Page.Key == "" {
		t.Fatal("created page key is empty — mirror was not refreshed")
	}
	got := decode[struct {
		Title string `json:"title"`
	}](t, get(t, h, apiBase+"pages/"+body.Page.Key+"/", nil))
	if got.Title != "Wiki create probe" {
		t.Fatalf("mirror GET title %q", got.Title)
	}
}

func TestPageEditExplicitVersionConflict(t *testing.T) {
	wo, h, _ := wikiWritable(t)
	rec := send(t, h, http.MethodPut, apiBase+"pages/100/edit/",
		`{"title":"Renamed under lock","version":3}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409; body %s", rec.Code, rec.Body.String())
	}
	if got := wikiErrorCode(t, rec); got != "origin_rejected" {
		t.Fatalf("error %q, want origin_rejected", got)
	}
	if len(wo.puts) == 0 {
		t.Fatal("origin UpdatePage was not called")
	}
	if wo.puts[len(wo.puts)-1] != 4 {
		t.Fatalf("PUT version.number = %v, want 4 (explicit 3+1, not HEAD 5+1)", wo.puts)
	}
}

func TestPageCommentMissingIsNotFound(t *testing.T) {
	_, h, _ := wikiWritable(t)
	rec := send(t, h, http.MethodPost, apiBase+"pages/99999/comment/", `{"text":"hello"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404; body %s", rec.Code, rec.Body.String())
	}
	if got := wikiErrorCode(t, rec); got != "not_found" {
		t.Fatalf("error %q, want not_found", got)
	}
}

func TestPageEditInvalidADF(t *testing.T) {
	wo, h, _ := wikiWritable(t)
	rec := send(t, h, http.MethodPut, apiBase+"pages/200/edit/", `{"adf":"not-json"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if got := wikiErrorCode(t, rec); got != "invalid_adf" {
		t.Fatalf("error %q, want invalid_adf", got)
	}
	if len(wo.puts) != 0 {
		t.Fatalf("invalid ADF reached origin PUT: %v", wo.puts)
	}
}

func TestPageCreateInvalidADF(t *testing.T) {
	wo, h, _ := wikiWritable(t)
	rec := send(t, h, http.MethodPost, apiBase+"pages/",
		`{"space":"PROD","title":"x","adf":"{\"type\":\"paragraph\"}"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if got := wikiErrorCode(t, rec); got != "invalid_adf" {
		t.Fatalf("error %q, want invalid_adf", got)
	}
	if wo.creates != 0 {
		t.Fatal("invalid ADF reached origin create")
	}
}

func TestPageEditTextFormatLoss(t *testing.T) {
	wo, h, _ := wikiWritable(t)
	rec := send(t, h, http.MethodPut, apiBase+"pages/100/edit/", `{"text":"plain replacement"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409; body %s", rec.Code, rec.Body.String())
	}
	if got := wikiErrorCode(t, rec); got != "format_loss" {
		t.Fatalf("error %q, want format_loss", got)
	}
	if len(wo.puts) != 0 {
		t.Fatalf("format_loss reached origin PUT: %v", wo.puts)
	}

	rec = send(t, h, http.MethodPut, apiBase+"pages/100/edit/",
		`{"text":"plain replacement","force":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("force:true → %d: %s", rec.Code, rec.Body.String())
	}
	if len(wo.puts) == 0 {
		t.Fatal("force:true did not PUT")
	}
}

func seedLinearIssue(t *testing.T, db *store.DB, key string) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "linear:" + key, SourceID: "linear", ExternalID: key, Key: key,
				Title: "linear row", CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "FIX", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func decodePriorities(t *testing.T, rec *httptest.ResponseRecorder) []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
} {
	t.Helper()
	return decode[struct {
		Priorities []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"priorities"`
	}](t, rec).Priorities
}

// GDK-396 FAIL-first: the open-issue picker must not use the global Jira
// catalog. GET {key}/priorities/ is the per-origin catalog; a Linear row
// answers Linear's 0-4 scale, a Jira row matches GET priorities/.
func TestKeyPrioritiesRoutesBySource(t *testing.T) {
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Site = f.URL
	cfg.Linear = &config.LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}
	seedLinearIssue(t, db, "FIX-1")
	h := New(db, cfg)

	global := get(t, h, apiBase+"priorities/", nil)
	if global.Code != http.StatusOK {
		t.Fatalf("GET priorities/ → %d: %s", global.Code, global.Body.String())
	}
	jiraCat := decodePriorities(t, global)
	if len(jiraCat) != 3 || jiraCat[0].ID != "1" || jiraCat[0].Name != "Highest" {
		t.Fatalf("global catalog %v, want Jira Highest/High/Medium", jiraCat)
	}

	jiraKey := get(t, h, apiBase+"NMB-1/priorities/", nil)
	if jiraKey.Code != http.StatusOK {
		t.Fatalf("GET NMB-1/priorities/ → %d: %s (GDK-396: per-key catalog missing)", jiraKey.Code, jiraKey.Body.String())
	}
	gotJira := decodePriorities(t, jiraKey)
	if len(gotJira) != len(jiraCat) || gotJira[0].ID != jiraCat[0].ID || gotJira[0].Name != jiraCat[0].Name {
		t.Fatalf("Jira per-key catalog %v, want same as global %v", gotJira, jiraCat)
	}

	lin := get(t, h, apiBase+"FIX-1/priorities/", nil)
	if lin.Code != http.StatusOK {
		t.Fatalf("GET FIX-1/priorities/ → %d: %s (GDK-396: Linear catalog missing)", lin.Code, lin.Body.String())
	}
	gotLin := decodePriorities(t, lin)
	wantIDs := []string{"1", "2", "3", "4", "0"}
	if len(gotLin) != len(wantIDs) {
		t.Fatalf("Linear catalog %v, want ids %v", gotLin, wantIDs)
	}
	for i, id := range wantIDs {
		if gotLin[i].ID != id {
			t.Fatalf("Linear catalog[%d].id = %q, want %q (full %v)", i, gotLin[i].ID, id, gotLin)
		}
	}
	// Jira's "Highest" must not appear — that is the mixed-catalog defect.
	for _, p := range gotLin {
		if p.Name == "Highest" {
			t.Fatalf("Linear catalog carried a Jira name: %v", gotLin)
		}
	}
}

func TestKeyUsersMatchesGlobalOnJiraRow(t *testing.T) {
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Site = f.URL
	h := New(db, cfg)

	global := get(t, h, apiBase+"users/?q=cl", nil)
	if global.Code != http.StatusOK {
		t.Fatalf("GET users/ → %d: %s", global.Code, global.Body.String())
	}
	perKey := get(t, h, apiBase+"NMB-1/users/?q=cl", nil)
	if perKey.Code != http.StatusOK {
		t.Fatalf("GET NMB-1/users/ → %d: %s", perKey.Code, perKey.Body.String())
	}
	if global.Body.String() != perKey.Body.String() {
		t.Fatalf("per-key users %s != global %s", perKey.Body.String(), global.Body.String())
	}
}

// GDK-401 FAIL-first: credential/ must advertise whether Linear is configured
// so a Linear-only profile can open the write UI without a Jira token.
func TestLinearKeyPrioritiesWithoutJiraCredential(t *testing.T) {
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	cfg.Linear = &config.LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}
	seedLinearIssue(t, db, "FIX-1")
	h := New(db, cfg)

	lin := get(t, h, apiBase+"FIX-1/priorities/", nil)
	if lin.Code != http.StatusOK {
		t.Fatalf("Linear-only GET FIX-1/priorities/ → %d: %s", lin.Code, lin.Body.String())
	}
	jiraKey := get(t, h, apiBase+"NMB-1/priorities/", nil)
	if jiraKey.Code != http.StatusConflict {
		t.Fatalf("Jira row without token → %d, want 409", jiraKey.Code)
	}
}

func TestCredentialAdvertisesLinear(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	none := decode[map[string]any](t, get(t, h, apiBase+"credential/", nil))
	if v, ok := none["linear"]; !ok || v != false {
		t.Fatalf("without Linear block, linear=%v present=%v (body keys %v)", v, ok, none)
	}

	cfg.Linear = &config.LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}
	h = New(db, cfg)
	rec := get(t, h, apiBase+"credential/", nil)
	got := decode[map[string]any](t, rec)
	if got["linear"] != true {
		t.Fatalf("with Linear block, linear=%v body=%s", got["linear"], rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), cfg.Linear.APIKey) {
		t.Fatal("Linear API key leaked in credential/")
	}
}

func TestWikiWritesRequireCredential(t *testing.T) {
	db, _ := fixturePages(t)
	h := New(db, &config.Config{Projects: []string{"NMB"}})
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, apiBase + "pages/", `{"space":"PROD","title":"x","text":"hi"}`},
		{http.MethodPut, apiBase + "pages/100/edit/", `{"title":"x"}`},
		{http.MethodPost, apiBase + "pages/100/comment/", `{"text":"hi"}`},
	} {
		rec := send(t, h, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s %s → %d, want 409; body %s", tc.method, tc.path, rec.Code, rec.Body.String())
			continue
		}
		if got := wikiErrorCode(t, rec); got != "credential_required" {
			t.Errorf("%s %s error %q, want credential_required", tc.method, tc.path, got)
		}
	}
}

// clientCallAllowlist is the only remaining s.client() callers in write.go.
// Each entry must say why that call is not an issue-origin write. A new
// write handler that mints origin.Client this way fails
// TestWriteHandlersDoNotCallClient (GDK-681).
var clientCallAllowlist = map[string]string{
	"handlePageResync":   "wiki page re-read; credential gate only — issue writes must not share this Jira mint",
	"handlePriorities":   "GET workspace-wide Jira priority catalog; Linear rows use handleKeyPriorities / keyWriter",
	"handleCreateMeta":   "GET creatable project/type catalog; not a write",
	"handleCreateFields": "GET create-time field catalog for one project+type; not a write (Linear has no CreateFieldCatalog face)",
	"handleUsers":        "GET user search catalog; per-key users use handleKeyUsers / keyWriter",
}

// TestWriteHandlersDoNotCallClient is the GDK-681 lock: issue write handlers
// in write.go must route through writerFor / keyWriter / createWriter, not
// s.client() (origin.Client is Jira-only; a Linear apiKey still passes
// HasCredential). Catalog GETs stay on the allowlist above with a reason.
func TestWriteHandlersDoNotCallClient(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src := filepath.Join(filepath.Dir(thisFile), "write.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		t.Fatalf("parse write.go: %v", err)
	}

	var hits []string
	seen := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "client" {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != "s" {
				return true
			}
			seen[name] = true
			if _, allowed := clientCallAllowlist[name]; allowed {
				return true
			}
			pos := fset.Position(sel.Pos())
			hits = append(hits, fmt.Sprintf("%s:%d %s", filepath.Base(pos.Filename), pos.Line, name))
			return true
		})
	}
	if len(hits) > 0 {
		t.Fatalf("issue write handlers must not call s.client() (use writerFor / keyWriter / createWriter); allowlisted catalog GETs need a reason in clientCallAllowlist:\n  %s", strings.Join(hits, "\n  "))
	}
	for name, reason := range clientCallAllowlist {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("clientCallAllowlist[%q] has no reason", name)
		}
		if !seen[name] {
			t.Errorf("clientCallAllowlist[%q] is stale — %s no longer calls s.client()", name, name)
		}
	}
}

// TestOriginClientLinearOnlyNeedsAtlassian is GDK-681 task ①: measure what
// origin.Client returns when the workspace has a Linear key and no Atlassian
// site. HasCredential is true (the Linear key counts), so s.client() does
// not 409 — the mint itself is what happens next.
func TestOriginClientLinearOnlyNeedsAtlassian(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Linear: &config.LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}}
	if !cfg.HasCredential() {
		t.Fatal("Linear-only HasCredential is false; s.client() would 409 before origin.Client")
	}
	if cfg.HasAtlassianCredential() {
		t.Fatal("Linear-only HasAtlassianCredential is true")
	}
	c, err := origin.Client(cfg)
	if c != nil {
		t.Fatalf("origin.Client returned a Jira client BaseURL=%q (would send requests somewhere)", c.BaseURL())
	}
	if err == nil {
		t.Fatal("origin.Client succeeded with no Atlassian site")
	}
	const want = "origin: site, email and token are required"
	if err.Error() != want {
		t.Fatalf("origin.Client error = %q, want %q", err.Error(), want)
	}
}

func linearTestdataFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "linear", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func linearIssueFromPage(t *testing.T) json.RawMessage {
	t.Helper()
	var wrap struct {
		Data struct {
			Issues struct {
				Nodes []json.RawMessage `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(linearTestdataFile(t, "issues_page1.json"), &wrap); err != nil {
		t.Fatal(err)
	}
	if len(wrap.Data.Issues.Nodes) == 0 {
		t.Fatal("issues_page1.json has no nodes")
	}
	return wrap.Data.Issues.Nodes[0]
}

func linearIssueFromCreate(t *testing.T) json.RawMessage {
	t.Helper()
	var wrap struct {
		Data struct {
			IssueCreate struct {
				Issue json.RawMessage `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(linearTestdataFile(t, "issue_create.json"), &wrap); err != nil {
		t.Fatal(err)
	}
	if len(wrap.Data.IssueCreate.Issue) == 0 {
		t.Fatal("issue_create.json has no issue")
	}
	return wrap.Data.IssueCreate.Issue
}

type linearFake struct {
	creates int
	updates int
}

func startLinearFake(t *testing.T, issue json.RawMessage) *linearFake {
	t.Helper()
	f := &linearFake{}
	issueDoc, err := json.Marshal(map[string]any{
		"data": map[string]json.RawMessage{"issue": issue},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		q := string(raw)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "query Teams"):
			_, _ = w.Write(linearTestdataFile(t, "teams.json"))
		case strings.Contains(q, "mutation IssueCreate"):
			f.creates++
			_, _ = w.Write(linearTestdataFile(t, "issue_create.json"))
		case strings.Contains(q, "mutation IssueUpdate"):
			f.updates++
			_, _ = w.Write(linearTestdataFile(t, "issue_update.json"))
		case strings.Contains(q, "query Issue("):
			_, _ = w.Write(issueDoc)
		default:
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	t.Cleanup(srv.Close)
	origin.LinearEndpoint = srv.URL
	t.Cleanup(func() { origin.LinearEndpoint = "" })
	return f
}

func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
}

func ensureLinearSource(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
}

func seedLinearIssueFromJSON(t *testing.T, db *store.DB, issue json.RawMessage) {
	t.Helper()
	ensureLinearSource(t, db)
	var iss struct {
		ID         string `json:"id"`
		Identifier string `json:"identifier"`
	}
	if err := json.Unmarshal(issue, &iss); err != nil {
		t.Fatal(err)
	}
	if iss.ID == "" || iss.Identifier == "" {
		t.Fatalf("linear issue json missing id/identifier: %s", issue)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "linear:" + iss.ID, SourceID: "linear", ExternalID: iss.ID, Key: iss.Identifier,
				Title: "linear row", CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "FIX", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// GDK-681: PATCH {key}/fields/ on a Linear-only workspace used to mint
// origin.Client and 500 "site, email and token are required".
func TestLinearOnlyFieldEditRoutesToLinear(t *testing.T) {
	isolateHome(t)
	lin := startLinearFake(t, linearIssueFromPage(t))
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	cfg.Linear = &config.LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}
	cfg.EditableFields = map[string]string{"summary": "summary"}
	seedLinearIssueFromJSON(t, db, linearIssueFromPage(t))
	h := New(db, cfg)

	rec := send(t, h, http.MethodPatch, apiBase+"FIX-1/fields/", `{"field":"summary","value":"renamed"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Linear-only PATCH fields → %d: %s", rec.Code, rec.Body.String())
	}
	if lin.updates != 1 {
		t.Fatalf("IssueUpdate mutations = %d, want 1; body %s", lin.updates, rec.Body.String())
	}
	if got := decode[map[string]any](t, rec)["origin"]; got != "linear" {
		t.Fatalf("origin = %v, want linear; body %s", got, rec.Body.String())
	}
}

// GDK-681: POST create/ on a Linear-only workspace used to mint origin.Client
// (Jira) instead of WriterFor("linear").
func TestLinearOnlyCreateRoutesToLinear(t *testing.T) {
	isolateHome(t)
	lin := startLinearFake(t, linearIssueFromCreate(t))
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	cfg.Linear = &config.LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}
	cfg.Projects = []string{"FIX"}
	ensureLinearSource(t, db)
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"FIX","summary":"a summary"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Linear-only POST create → %d: %s", rec.Code, rec.Body.String())
	}
	if lin.creates != 1 {
		t.Fatalf("IssueCreate mutations = %d, want 1; body %s", lin.creates, rec.Body.String())
	}
	got := decode[map[string]any](t, rec)
	if got["origin"] != "linear" {
		t.Fatalf("origin = %v, want linear; body %s", got["origin"], rec.Body.String())
	}
}

// GDK-681: in a jira+linear workspace the project key decides — a mirrored
// Linear team must not POST /issue on Jira.
func TestCreateRoutesLinearTeamInMixedWorkspace(t *testing.T) {
	isolateHome(t)
	jiraFake := newFakeJira(t)
	lin := startLinearFake(t, linearIssueFromCreate(t))
	db, cfg := fixture(t)
	cfg.Site = jiraFake.URL
	cfg.Linear = &config.LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}
	cfg.Projects = append(append([]string{}, cfg.Projects...), "FIX")
	seedLinearIssue(t, db, "FIX-1")
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"FIX","summary":"a summary"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mixed POST create FIX → %d: %s", rec.Code, rec.Body.String())
	}
	if lin.creates != 1 {
		t.Fatalf("IssueCreate mutations = %d, want 1; body %s", lin.creates, rec.Body.String())
	}
	if jiraFake.called("POST /issue") {
		t.Fatalf("Linear team create reached Jira: %v", jiraFake.calls)
	}
	if got := decode[map[string]any](t, rec)["origin"]; got != "linear" {
		t.Fatalf("origin = %v, want linear; body %s", got, rec.Body.String())
	}
}

func TestCreateIssueReportsOrigin(t *testing.T) {
	_, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"create/",
		`{"project_key":"NMB","issue_type":"10004","summary":"새 버그"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create → %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]any](t, rec)["origin"]; got != "jira" {
		t.Fatalf("origin = %v, want jira; body %s", got, rec.Body.String())
	}
}
