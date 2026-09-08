package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The Jira Server / Data Center dialect (GDK-1636). NewServer builds the
// client whose REST base is v2 — Server has no v3 at all and answers a v3
// route with 401 — so every path this package builds must follow the
// client's own base, and the endpoints whose v2 shape differs by more than
// the version number are pinned here against a v2-shaped httptest origin.

func serverTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewServer(srv.URL, "pat-token")
	c.Retries, c.Backoff = 4, 0
	return c
}

// TestAPIBasePerConstructor pins which dialect each constructor builds:
// Cloud and the anonymous deployment probe stay v3 (issuetap implements the
// Cloud shape and is driven through the same constructors); only NewServer
// is v2. Before GDK-1636 one package constant pinned every client to v3.
func TestAPIBasePerConstructor(t *testing.T) {
	var last string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL.Path
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	for _, tc := range []struct {
		name string
		c    *Client
		want string
	}{
		{"New", New(srv.URL, "a@b.c", "tok"), "/rest/api/3"},
		{"NewAnonymous", NewAnonymous(srv.URL), "/rest/api/3"},
		{"NewServer", NewServer(srv.URL, "tok"), "/rest/api/2"},
	} {
		tc.c.Retries, tc.c.Backoff = 4, 0
		if _, err := tc.c.Myself(context.Background()); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if last != tc.want+"/myself" {
			t.Errorf("%s requested %s, want %s/myself", tc.name, last, tc.want)
		}
	}
}

// TestNewServerRequestsStartAtV2 is the round-one FAIL-first test (GDK-1636):
// with the shared /rest/api/3 constant a Server client asked a route Server
// does not serve, and the 401 came back dressed as a rejected credential.
func TestNewServerRequestsStartAtV2(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/2/") {
			t.Errorf("request path %s, want it to start with /rest/api/2/", r.URL.Path)
		}
		w.Write([]byte(`[]`))
	}))
	if _, err := c.Statuses(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestServerCreateMetaUsesPerProjectRoute pins the measured Server shape:
// the bulk createmeta Cloud serves is gone, the per-project
// /issue/createmeta/{key}/issuetypes route is the replacement, and it
// answers a paged values envelope rather than a projects object.
func TestServerCreateMetaUsesPerProjectRoute(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s, want GET", r.Method)
		}
		if r.URL.Path != "/rest/api/2/issue/createmeta/GDK/issuetypes" {
			t.Errorf("path %s, want /rest/api/2/issue/createmeta/GDK/issuetypes", r.URL.Path)
		}
		w.Write([]byte(`{"maxResults":100,"startAt":0,"total":2,"isLast":true,"values":[{"id":"10001","name":"Task","subtask":false},{"id":"10002","name":"Bug","subtask":false}]}`))
	}))
	got, err := c.CreateMeta(context.Background(), []string{"GDK"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "GDK" || len(got[0].IssueTypes) != 2 {
		t.Fatalf("CreateMeta = %+v, want one GDK project with two issue types", got)
	}
	if typ := got[0].IssueTypes[0]; typ.ID != "10001" || typ.Name != "Task" {
		t.Errorf("first issue type = %+v, want id 10001 name Task", typ)
	}
}

// TestServerCreateMetaPagesIssueTypes walks Server's values envelope to its
// total: a client that stops after page one would report half the types.
func TestServerCreateMetaPagesIssueTypes(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("startAt") {
		case "", "0":
			w.Write([]byte(`{"maxResults":2,"startAt":0,"total":3,"isLast":false,"values":[{"id":"10001","name":"Task"},{"id":"10002","name":"Bug"}]}`))
		case "2":
			w.Write([]byte(`{"maxResults":2,"startAt":2,"total":3,"isLast":true,"values":[{"id":"10003","name":"Sub-task","subtask":true}]}`))
		default:
			t.Errorf("unexpected startAt %q", r.URL.Query().Get("startAt"))
		}
	}))
	got, err := c.CreateMeta(context.Background(), []string{"GDK"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].IssueTypes) != 3 {
		t.Fatalf("issue types = %+v, want 3", got)
	}
	if typ := got[0].IssueTypes[2]; typ.ID != "10003" || !typ.Subtask {
		t.Errorf("page-2 type = %+v, want id 10003 subtask", typ)
	}
}

// TestServerCreateMetaWithoutScopeRefuses: Cloud's unscoped createmeta is the
// site-wide list; Server has no bulk route, so an unscoped ask must be a
// named refusal, never a request against a route Server does not serve.
func TestServerCreateMetaWithoutScopeRefuses(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("refusal must not leave the process: %s %s", r.Method, r.URL.Path)
	}))
	_, err := c.CreateMeta(context.Background(), nil)
	if !errors.Is(err, ErrServerCreateMetaScope) {
		t.Fatalf("err = %v, want ErrServerCreateMetaScope", err)
	}
	if u := c.Usage(); u.Requests != 0 {
		t.Errorf("Requests = %d, want 0", u.Requests)
	}
}

// TestServerCreateFieldsParsesValuesEnvelope: Server's createmeta family
// pages a values envelope where Cloud serves fields[].
func TestServerCreateFieldsParsesValuesEnvelope(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issue/createmeta/GDK/issuetypes/10003" {
			t.Errorf("path %s, want /rest/api/2/issue/createmeta/GDK/issuetypes/10003", r.URL.Path)
		}
		w.Write([]byte(`{"maxResults":50,"startAt":0,"total":2,"isLast":true,"values":[` +
			`{"fieldId":"project","name":"Project","required":true,"hasDefaultValue":false,"schema":{"type":"project"}},` +
			`{"fieldId":"summary","name":"Summary","required":true,"hasDefaultValue":false,"schema":{"type":"string"}}]}`))
	}))
	got, err := c.CreateFields(context.Background(), "GDK", "10003")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	if got[0].FieldID != "project" || got[1].FieldID != "summary" || !got[1].Required {
		t.Errorf("fields = %+v", got)
	}
}

// TestServerSearchPagesByStartAt pins the v2 search contract: the route is
// /search, the body never carries nextPageToken (Server has none — a
// token-driven loop would end the walk after one page), and the pages walk
// startAt to the total.
func TestServerSearchPagesByStartAt(t *testing.T) {
	var bodies []map[string]any
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/2/search" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		bodies = append(bodies, body)
		if _, ok := body["nextPageToken"]; ok {
			t.Errorf("body carried nextPageToken: %v", body)
		}
		switch body["startAt"] {
		case float64(0), nil:
			w.Write([]byte(`{"startAt":0,"maxResults":2,"total":3,"issues":[{"id":"1","key":"A-1","fields":{"summary":"one"}},{"id":"2","key":"A-2","fields":{"summary":"two"}}]}`))
		case float64(2):
			w.Write([]byte(`{"startAt":2,"maxResults":2,"total":3,"issues":[{"id":"3","key":"A-3","fields":{"summary":"three"}}]}`))
		default:
			t.Errorf("unexpected startAt %v", body["startAt"])
		}
	}))
	var keys []string
	err := c.Search(context.Background(), "project = A", []string{"summary"}, false, func(issues []Issue) error {
		for _, i := range issues {
			keys = append(keys, i.Key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(keys, ",") != "A-1,A-2,A-3" {
		t.Errorf("keys = %v, want all three — a one-page walk is silent truncation", keys)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	if bodies[0]["startAt"] != float64(0) || bodies[1]["startAt"] != float64(2) {
		t.Errorf("startAt sequence = %v,%v, want 0,2", bodies[0]["startAt"], bodies[1]["startAt"])
	}
	if bodies[0]["jql"] != "project = A" || bodies[0]["maxResults"] != float64(100) {
		t.Errorf("first body = %v, want the same jql/maxResults as Cloud", bodies[0])
	}
}

// TestServerCountReadsSearchTotal: Server has no approximate-count route;
// its count is the total of a zero-row search, and it must not touch the v3
// route (Server would answer 401 and the failure would read as auth).
func TestServerCountReadsSearchTotal(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/search" {
			t.Errorf("path %s, want /rest/api/2/search", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["maxResults"] != float64(0) {
			t.Errorf("maxResults = %v, want 0 — the count asks for no rows", body["maxResults"])
		}
		w.Write([]byte(`{"startAt":0,"maxResults":0,"total":4321,"issues":[]}`))
	}))
	n, err := c.Count(context.Background(), `project = "STD"`)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4321 {
		t.Errorf("count = %d, want 4321", n)
	}
	if u := c.Usage(); u.Requests != 1 {
		t.Errorf("Requests = %d, want 1", u.Requests)
	}
}

// TestServerUploadCarriesNoCheckHeader: both deployments require
// X-Atlassian-Token: no-check on the multipart upload — without it Server
// answers 403 — and the Server credential is the PAT bearer, not basic.
func TestServerUploadCarriesNoCheckHeader(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issue/A-1/attachments" {
			t.Errorf("path %s, want /rest/api/2/issue/A-1/attachments", r.URL.Path)
		}
		if got := r.Header.Get("X-Atlassian-Token"); got != "no-check" {
			t.Errorf("X-Atlassian-Token = %q, want no-check", got)
		}
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ") {
			// Never print the value: the credential stays out of output.
			t.Errorf("Authorization scheme is not Bearer (header length %d)", len(auth))
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.Write([]byte(`[{"id":"101","filename":"shot.png"}]`))
	}))
	atts, err := c.Upload(context.Background(), "A-1", "shot.png", strings.NewReader("png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 || atts[0].Filename != "shot.png" {
		t.Fatalf("attachments = %+v", atts)
	}
}

// TestServerMediaRefRefusedBeforeRequest: the media-id route is Cloud-only;
// the refusal must be named and must not burn a request that would only
// come back dressed as a 401.
func TestServerMediaRefRefusedBeforeRequest(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("refusal must not leave the process: %s %s", r.Method, r.URL.Path)
	}))
	_, _, err := c.MediaRef(context.Background(), "101")
	if !errors.Is(err, ErrServerNoMediaRef) {
		t.Fatalf("err = %v, want ErrServerNoMediaRef", err)
	}
}

// TestAttachmentFilenameCarriesAPIBase pins the fallback's path: born
// without any base, it requested <site>/attachment/{id} — a route no Jira
// deployment serves — so the fallback could only ever fail.
func TestAttachmentFilenameCarriesAPIBase(t *testing.T) {
	var path string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write([]byte(`{"filename":"shot.png"}`))
	}))
	name, err := c.attachmentFilename(context.Background(), "101")
	if err != nil {
		t.Fatal(err)
	}
	if name != "shot.png" {
		t.Errorf("filename = %q", name)
	}
	if path != "/rest/api/3/attachment/101" {
		t.Errorf("path = %s, want /rest/api/3/attachment/101", path)
	}
}
