package jira

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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
	if !errors.Is(err, errServerCreateMetaScope) {
		t.Fatalf("err = %v, want errServerCreateMetaScope", err)
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
	if !errors.Is(err, errServerNoMediaRef) {
		t.Fatalf("err = %v, want errServerNoMediaRef", err)
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

// TestServerUserSearchSendsUsernameParam pins the user-search parameter per
// dialect (GDK-1638): Server's /user/search reads username=, and the query=
// Cloud uses is not an error there — it is a silent empty list, so the two
// must never be tried in sequence. The Cloud client keeps query=.
func TestServerUserSearchSendsUsernameParam(t *testing.T) {
	const serverPayload = `[{"self":"https://s.example/jira/rest/api/2/user?username=dkim","key":"dkim","name":"dkim","emailAddress":"dkim@s.example","displayName":"Dana Kim","active":true}]`
	for _, tc := range []struct {
		name      string
		server    bool
		wantParam string
	}{
		{"server", true, "username"},
		{"cloud", false, "query"},
	} {
		var gotPath string
		var gotQuery url.Values
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotQuery = r.URL.Path, r.URL.Query()
			w.Write([]byte(serverPayload))
		}))
		t.Cleanup(srv.Close)
		var c *Client
		if tc.server {
			c = NewServer(srv.URL, "pat-token")
		} else {
			c = New(srv.URL, "a@b.c", "tok")
		}
		c.Retries, c.Backoff = 1, 0
		users, err := c.SearchUsers(context.Background(), "dkim")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if gotPath != c.apiBase+"/user/search" {
			t.Errorf("%s: path = %s", tc.name, gotPath)
		}
		if got := gotQuery.Get(tc.wantParam); got != "dkim" {
			t.Errorf("%s: %s = %q, want dkim", tc.name, tc.wantParam, got)
		}
		other := "query"
		if tc.wantParam == "query" {
			other = "username"
		}
		if gotQuery.Get(other) != "" {
			t.Errorf("%s: %s must not be sent — the dialect branches once, no fallback", tc.name, other)
		}
		if len(users) != 1 || users[0].Name != "dkim" || users[0].Key != "dkim" || users[0].ID() != "dkim" {
			t.Errorf("%s: users = %+v, want the Server user keyed by name", tc.name, users)
		}
	}
}

// TestServerSetAssigneeSendsNameField pins the assignee PUT body per dialect
// (GDK-1638): Server takes {"name": …} where Cloud takes {"accountId": …},
// and unassign is the dialect's own key set to null — sending Cloud's null
// key to Server would leave the assignee untouched, not cleared.
func TestServerSetAssigneeSendsNameField(t *testing.T) {
	for _, tc := range []struct {
		name     string
		server   bool
		id       string
		assign   string
		unassign string
	}{
		{"server", true, "dkim", `{"name":"dkim"}`, `{"name":null}`},
		{"cloud", false, "5b10a284", `{"accountId":"5b10a284"}`, `{"accountId":null}`},
	} {
		var bodies []string
		var reqs []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("%s: read body: %v", tc.name, err)
			}
			bodies = append(bodies, string(b))
			reqs = append(reqs, r.Method+" "+r.URL.Path)
		}))
		t.Cleanup(srv.Close)
		var c *Client
		if tc.server {
			c = NewServer(srv.URL, "pat-token")
		} else {
			c = New(srv.URL, "a@b.c", "tok")
		}
		c.Retries, c.Backoff = 1, 0
		if err := c.SetAssignee(context.Background(), "NMB-1", tc.id); err != nil {
			t.Fatalf("%s: assign: %v", tc.name, err)
		}
		if err := c.SetAssignee(context.Background(), "NMB-1", ""); err != nil {
			t.Fatalf("%s: unassign: %v", tc.name, err)
		}
		wantReq := "PUT " + c.apiBase + "/issue/NMB-1/assignee"
		if reqs[0] != wantReq || reqs[1] != wantReq {
			t.Errorf("%s: requests = %v, want two %s", tc.name, reqs, wantReq)
		}
		if bodies[0] != tc.assign || bodies[1] != tc.unassign {
			t.Errorf("%s: bodies = %v, want %q then %q", tc.name, bodies, tc.assign, tc.unassign)
		}
	}
}

// GDK-1645: Jira Server keys a standard issue's epic in the Epic Link custom
// field; fields.parent there is a sub-task's, and for a standard issue the
// origin answers 204 and drops it (measured). The client rewrites parent
// into the Epic Link field for a non-sub-task — create, edit and the web's
// parent editor all pass through the same three methods.
func TestServerParentBecomesEpicLinkForStandardIssue(t *testing.T) {
	var put map[string]any
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/2/field":
			w.Write([]byte(`[{"id":"summary","name":"Summary","schema":{"type":"string"}},
				{"id":"customfield_10100","name":"Epic Link","custom":true,"schema":{"type":"any","custom":"com.pyxis.greenhopper.jira:gh-epic-link"}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/SCR-3":
			w.Write([]byte(`{"key":"SCR-3","fields":{"issuetype":{"id":"10001","subtask":false}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/SCR-9":
			w.Write([]byte(`{"key":"SCR-9","fields":{"issuetype":{"id":"10003","subtask":true}}}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/"):
			put = nil
			json.NewDecoder(r.Body).Decode(&put)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	ctx := context.Background()
	if err := c.EditIssue(ctx, "SCR-3", map[string]any{"parent": map[string]string{"key": "SCR-7"}}, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := put["fields"].(map[string]any)
	if _, has := f["parent"]; has {
		t.Errorf("a standard issue's PUT still carries fields.parent: %v — Server drops that silently", f)
	}
	if got := f["customfield_10100"]; got != "SCR-7" {
		t.Errorf("Epic Link field = %v, want the epic key string SCR-7", got)
	}
	// Clearing goes to the same field as null.
	if err := c.UpdateFields(ctx, "SCR-3", map[string]any{"parent": nil}); err != nil {
		t.Fatal(err)
	}
	f, _ = put["fields"].(map[string]any)
	if v, has := f["customfield_10100"]; !has || v != nil {
		t.Errorf("clear: fields = %v, want customfield_10100 null", f)
	}
	// A sub-task keeps fields.parent — that is the one place Server honours it.
	if err := c.EditIssue(ctx, "SCR-9", map[string]any{"parent": map[string]string{"key": "SCR-3"}}, nil); err != nil {
		t.Fatal(err)
	}
	f, _ = put["fields"].(map[string]any)
	if p, _ := f["parent"].(map[string]any); p["key"] != "SCR-3" {
		t.Errorf("sub-task PUT fields = %v, want parent.key SCR-3 kept", f)
	}
	if _, has := f["customfield_10100"]; has {
		t.Errorf("sub-task PUT must not touch the Epic Link field: %v", f)
	}
}

// A Server without Jira Software has no Epic Link field; a standard issue's
// parent is refused before any PUT, instead of the 204-and-nothing.
func TestServerParentRefusedWithoutEpicLinkField(t *testing.T) {
	c := serverTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/2/field":
			w.Write([]byte(`[{"id":"summary","name":"Summary","schema":{"type":"string"}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/DCT-2":
			w.Write([]byte(`{"key":"DCT-2","fields":{"issuetype":{"id":"10001","subtask":false}}}`))
		default:
			t.Errorf("unexpected %s %s — the refusal must come before the PUT", r.Method, r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	err := c.EditIssue(context.Background(), "DCT-2", map[string]any{"parent": map[string]string{"key": "DCT-1"}}, nil)
	if !errors.Is(err, errNoEpicLinkField) {
		t.Fatalf("err = %v, want errNoEpicLinkField", err)
	}
}

// Cloud is untouched: parent goes out as parent, and no catalog or issue
// lookup is spent deciding.
func TestCloudParentIsNotRewritten(t *testing.T) {
	var put map[string]any
	var gets int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets++
		}
		put = nil
		json.NewDecoder(r.Body).Decode(&put)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "a@b.c", "tok")
	c.Retries, c.Backoff = 1, 0
	if err := c.EditIssue(context.Background(), "NMB-1", map[string]any{"parent": map[string]string{"key": "NMB-9"}}, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := put["fields"].(map[string]any)
	if p, _ := f["parent"].(map[string]any); p["key"] != "NMB-9" || gets != 0 {
		t.Errorf("Cloud PUT fields = %v (gets=%d), want parent.key NMB-9 and no lookups", f, gets)
	}
}

// GDK-1646: Jira Data Center states its token-bucket budget on every
// authenticated response, so the Server client spaces its requests before
// the 429 instead of after it — one Wait before every attempt, one Observe
// after every response, asserted through an injected Sleep (no real
// waiting). The per-header contract is pinned in internal/httppolicy; this
// test pins the wiring: only NewServer carries a budget. On the unmodified
// tree it does not compile (Client had no budget), which is its FAIL — the
// Cloud path must keep no budget at all. The live FAIL-first — a real 429
// from a DC with a low limit — is the lead's, measured against the lab
// instance.
func TestServerRateBudgetSleepsFromHeaders(t *testing.T) {
	// dcHeaders is the budget DC states, parameterised by what the bucket
	// holds at that moment; limit is always the full bucket so the sleep's
	// optimistic refill can be observed through the next Wait.
	dcHeaders := func(remaining, retryAfter string) http.Header {
		return http.Header{
			"X-RateLimit-Limit":            []string{"100"},
			"X-RateLimit-Remaining":        []string{remaining},
			"X-RateLimit-Interval-Seconds": []string{"7"},
			"X-RateLimit-FillRate":         []string{"10"},
			"Retry-After":                  []string{retryAfter},
		}
	}
	for _, tc := range []struct {
		name   string
		server bool
		// budgetStates is what each successive response states; when nil
		// the responses carry no rate-limit headers at all.
		budgetStates []http.Header
		want         []time.Duration
	}{
		{
			name: "interval when retry-after is zero",
			// Response 1 leaves 1 token (Reserve is 2): the next request
			// sleeps the 7s refill interval; response 2 then states a
			// refilled bucket and request 3 goes straight out.
			server:       true,
			budgetStates: []http.Header{dcHeaders("1", "0"), dcHeaders("100", "0"), dcHeaders("100", "0")},
			want:         []time.Duration{7 * time.Second},
		},
		{
			name:         "retry-after names the sleep",
			server:       true,
			budgetStates: []http.Header{dcHeaders("0", "3"), dcHeaders("100", "0")},
			want:         []time.Duration{3 * time.Second},
		},
		{
			name:         "ample remaining never sleeps",
			server:       true,
			budgetStates: []http.Header{dcHeaders("50", "0"), dcHeaders("50", "0")},
			want:         nil,
		},
		{
			name:   "headers absent never sleeps",
			server: true,
			want:   nil,
		},
		{
			// Cloud publishes none of these headers; even an origin that
			// sent them must not make a Cloud client sleep — its budget is
			// nil, so the request path is the pre-GDK-1646 one byte for
			// byte.
			name:         "cloud never sleeps, even against the headers",
			server:       false,
			budgetStates: []http.Header{dcHeaders("0", "9"), dcHeaders("0", "9"), dcHeaders("0", "9")},
			want:         nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reqs := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := reqs
				reqs++
				if i < len(tc.budgetStates) {
					for k, vs := range tc.budgetStates[i] {
						w.Header()[k] = vs
					}
				}
				w.Write([]byte(`[]`))
			}))
			t.Cleanup(srv.Close)
			var c *Client
			if tc.server {
				c = NewServer(srv.URL, "pat-token")
			} else {
				c = New(srv.URL, "a@b.c", "tok")
			}
			c.Retries, c.Backoff = 1, 0
			var slept []time.Duration
			if c.budget != nil {
				c.budget.Sleep = func(_ context.Context, d time.Duration) error {
					slept = append(slept, d)
					return nil
				}
			}
			n := len(tc.budgetStates)
			if n == 0 {
				n = 2
			}
			for i := 0; i < n; i++ {
				if _, err := c.Statuses(context.Background()); err != nil {
					t.Fatalf("%s: request %d: %v", tc.name, i+1, err)
				}
			}
			if !tc.server && c.budget != nil {
				t.Errorf("%s: a Cloud client carries a budget", tc.name)
			}
			if len(slept) != len(tc.want) {
				t.Fatalf("%s: slept %v, want %v", tc.name, slept, tc.want)
			}
			for i := range slept {
				if slept[i] != tc.want[i] {
					t.Fatalf("%s: slept %v, want %v", tc.name, slept, tc.want)
				}
			}
			if reqs != n {
				t.Errorf("%s: requests = %d, want %d", tc.name, reqs, n)
			}
		})
	}
}
