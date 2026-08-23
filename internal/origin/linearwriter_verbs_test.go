package origin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
)

// linearRec records which GraphQL documents the adapter sent so a refuse
// case can prove the mutation never left the process.
type linearRec struct {
	queries  []string
	creates  int
	updates  int
	comments int
	lastVars json.RawMessage
}

func linearTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "linear", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func issueQueryFixture(t *testing.T) []byte {
	t.Helper()
	var wrap struct {
		Data struct {
			Issues struct {
				Nodes []json.RawMessage `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(linearTestdata(t, "issues_page1.json"), &wrap); err != nil {
		t.Fatal(err)
	}
	if len(wrap.Data.Issues.Nodes) == 0 {
		t.Fatal("issues_page1.json has no nodes")
	}
	out, err := json.Marshal(map[string]any{
		"data": map[string]json.RawMessage{"issue": wrap.Data.Issues.Nodes[0]},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func linearGQL(t *testing.T, rec *linearRec) http.Handler {
	t.Helper()
	issue := issueQueryFixture(t)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string          `json:"query"`
			Variables json.RawMessage `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode graphql: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rec.queries = append(rec.queries, body.Query)
		rec.lastVars = body.Variables
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body.Query, "query Issue("):
			_, _ = w.Write(issue)
		case strings.Contains(body.Query, "query WorkflowStates"):
			_, _ = w.Write(linearTestdata(t, "workflowstates.json"))
		case strings.Contains(body.Query, "query Teams"):
			_, _ = w.Write(linearTestdata(t, "teams.json"))
		case strings.Contains(body.Query, "query Users"):
			_, _ = w.Write([]byte(`{"data":{"users":{"nodes":[{"id":"lin-u1","name":"Dana","displayName":"Dana","email":"dana@example.com"}]}}}`))
		case strings.Contains(body.Query, "mutation IssueCreate"):
			rec.creates++
			_, _ = w.Write(linearTestdata(t, "issue_create.json"))
		case strings.Contains(body.Query, "mutation IssueUpdate"):
			rec.updates++
			_, _ = w.Write(linearTestdata(t, "issue_update.json"))
		case strings.Contains(body.Query, "mutation CommentCreate"):
			rec.comments++
			_, _ = w.Write(linearTestdata(t, "comment_create.json"))
		default:
			t.Errorf("unexpected graphql document: %s", truncate(body.Query, 80))
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func testLinearWriter(t *testing.T) (*linearWriter, *linearRec) {
	t.Helper()
	rec := &linearRec{}
	srv := httptest.NewServer(linearGQL(t, rec))
	t.Cleanup(srv.Close)
	c := linear.New("linear-test-key-not-a-real-secret")
	c.Endpoint = srv.URL
	c.Retries, c.Backoff = 1, 0
	return &linearWriter{c: c}, rec
}

func TestCreateIssueRefusesUnsupportedFields(t *testing.T) {
	w, rec := testLinearWriter(t)
	ctx := context.Background()
	base := map[string]any{
		"project": map[string]any{"key": "FIX"},
		"summary": "half-apply probe",
	}
	for _, field := range []string{"assignee", "labels", "parent", "issuetype"} {
		fields := map[string]any{
			"project": base["project"],
			"summary": base["summary"],
			field:     "x",
		}
		_, err := w.CreateIssue(ctx, fields)
		if err == nil {
			t.Errorf("%s: CreateIssue succeeded (creates=%d) — unsupported field was silently dropped", field, rec.creates)
			continue
		}
		if !strings.Contains(err.Error(), field) {
			t.Errorf("%s: error %q does not name the field", field, err)
		}
	}
	if rec.creates != 0 {
		t.Errorf("CreateIssue sent %d issueCreate mutations; refuse must happen before the wire", rec.creates)
	}
}

func TestCreateIssueRefusesNonIntegerPriority(t *testing.T) {
	w, rec := testLinearWriter(t)
	_, err := w.CreateIssue(context.Background(), map[string]any{
		"project":  map[string]any{"key": "FIX"},
		"summary":  "prio probe",
		"priority": map[string]any{"id": "Highest"},
	})
	if err == nil {
		t.Fatalf("non-integer priority succeeded (creates=%d)", rec.creates)
	}
	if !strings.Contains(err.Error(), "priority") {
		t.Errorf("error %q does not name priority", err)
	}
	if rec.creates != 0 {
		t.Errorf("issueCreate ran %d times after a bad priority id", rec.creates)
	}
}

func TestUpdateFieldsRejectsLabelsDueClearAndBadPriority(t *testing.T) {
	w, rec := testLinearWriter(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{"labels refused", map[string]any{"labels": []string{"bug"}}, "label"},
		{"due clear refused", map[string]any{"duedate": nil}, "due"},
		{"priority non-integer", map[string]any{"priority": map[string]any{"id": "Highest"}}, "priority"},
		{"priority below scale", map[string]any{"priority": map[string]any{"id": "-1"}}, "0-4"},
		{"priority above scale", map[string]any{"priority": map[string]any{"id": "5"}}, "0-4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := rec.updates
			err := w.UpdateFields(ctx, "FIX-1", tc.fields)
			if err == nil {
				t.Fatalf("succeeded (updates=%d)", rec.updates)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("error %q, want it to mention %q", err, tc.want)
			}
			if rec.updates != before {
				t.Errorf("issueUpdate ran; refuse must stay local")
			}
		})
	}
}

func TestEditIssueRefusesUpdateOps(t *testing.T) {
	w, rec := testLinearWriter(t)
	err := w.EditIssue(context.Background(), "FIX-1",
		map[string]any{"summary": "x"},
		map[string]any{"labels": []any{map[string]any{"add": "bug"}}},
	)
	if err == nil {
		t.Fatal("update-ops were accepted")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "label") {
		t.Errorf("error %q, want it to name the label operations", err)
	}
	if rec.updates != 0 {
		t.Errorf("issueUpdate ran %d times; update-ops must not half-apply", rec.updates)
	}
}

func TestAddCommentPostsMarkdown(t *testing.T) {
	w, rec := testLinearWriter(t)
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hello from linear"}]}]}`)
	got, err := w.AddComment(context.Background(), "FIX-1", adf, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if rec.comments != 1 {
		t.Fatalf("commentCreate calls = %d, want 1", rec.comments)
	}
	if got.ID == "" {
		t.Errorf("comment id empty: %+v", got)
	}
}

func TestAddCommentRefusesVisibilityAndInternal(t *testing.T) {
	w, rec := testLinearWriter(t)
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	vis := &jira.CommentVisibility{Type: "role", Value: "Administrators"}
	_, err := w.AddComment(context.Background(), "FIX-1", adf, vis, false)
	if err == nil {
		t.Fatal("visibility was accepted")
	}
	if !strings.Contains(err.Error(), "visibility") {
		t.Errorf("error %q, want it to name visibility", err)
	}
	_, err = w.AddComment(context.Background(), "FIX-1", adf, nil, true)
	if err == nil {
		t.Fatal("internal was accepted")
	}
	if !strings.Contains(err.Error(), "internal") {
		t.Errorf("error %q, want it to name internal", err)
	}
	if rec.comments != 0 {
		t.Errorf("refuse must stay local; comments=%d", rec.comments)
	}
}

func TestTransitionRefusesScreenFields(t *testing.T) {
	w, rec := testLinearWriter(t)
	ctx := context.Background()
	err := w.Transition(ctx, "FIX-1", "state-1", map[string]any{"resolution": map[string]string{"id": "1"}}, nil)
	if err == nil {
		t.Fatal("fields were accepted")
	}
	if !strings.Contains(err.Error(), "linear transitions do not carry screen fields") {
		t.Errorf("error %q", err)
	}
	err = w.Transition(ctx, "FIX-1", "state-1", nil, json.RawMessage(`{"type":"doc","version":1}`))
	if err == nil {
		t.Fatal("comment was accepted")
	}
	if !strings.Contains(err.Error(), "linear transitions do not carry screen fields") {
		t.Errorf("error %q", err)
	}
	if rec.updates != 0 || len(rec.queries) != 0 {
		t.Errorf("refuse must stay local; updates=%d queries=%d", rec.updates, len(rec.queries))
	}
}

func TestTransitionsMapLinearStates(t *testing.T) {
	w, _ := testLinearWriter(t)
	list, err := w.Transitions(context.Background(), "FIX-1")
	if err != nil {
		t.Fatal(err)
	}
	// Fixture issue is on Todo (unstarted). That state is omitted.
	if len(list) != 5 {
		t.Fatalf("transitions = %d, want 5 (catalog minus current)", len(list))
	}
	byName := map[string]string{}
	for _, tr := range list {
		byName[tr.Name] = tr.To.StatusCategory.Key
		if tr.ID == "" || tr.To.ID == "" {
			t.Errorf("transition missing id: %+v", tr)
		}
	}
	if byName["Todo"] != "" {
		t.Error("current Todo state was offered as a self-transition")
	}
	want := map[string]string{
		"In Progress": "indeterminate",
		"Done":        "done",
		"Canceled":    "done",
		"Duplicate":   "done",
		"Backlog":     "new",
	}
	for name, cat := range want {
		if byName[name] != cat {
			t.Errorf("%s category = %q, want %q", name, byName[name], cat)
		}
	}
}

func TestSearchUsersForwardsQuery(t *testing.T) {
	w, rec := testLinearWriter(t)
	users, err := w.SearchUsers(context.Background(), "dana@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].AccountID != "lin-u1" || users[0].Email != "dana@example.com" {
		t.Fatalf("users = %+v", users)
	}
	if !strings.Contains(string(rec.lastVars), "dana@example.com") {
		t.Errorf("SearchUsers did not forward the email query: %s", rec.lastVars)
	}
}

// GDK-641: linearWriter must not implement the optional faces. FAIL-first
// on the pre-split source: the five stubs made every type-assert succeed
// (measured: TestLinearWriterOmitsOptionalFaces, four t.Error lines).
func TestLinearWriterOmitsOptionalFaces(t *testing.T) {
	w, rec := testLinearWriter(t)
	if _, err := AsVersionCatalog(w); err == nil {
		t.Error("linearWriter satisfies VersionCatalog; optional face must be absent")
	}
	if CreatesVersionsByName(w) {
		t.Error("linearWriter CreatesVersionsByName = true; Linear has no version catalog")
	}
	if _, err := AsIssueLinker(w); err == nil {
		t.Error("linearWriter satisfies IssueLinker; optional face must be absent")
	}
	if _, err := AsCreateFieldCatalog(w); err == nil {
		t.Error("linearWriter satisfies CreateFieldCatalog; optional face must be absent")
	}
	if _, err := AsMediaRef(w); err == nil {
		t.Error("linearWriter satisfies MediaRef; optional face must be absent")
	}
	if len(rec.queries) != 0 {
		t.Errorf("optional-face refuse hit GraphQL %d times; refuse must stay local", len(rec.queries))
	}
}

func TestOptionalFaceErrorsMatchFormerStubs(t *testing.T) {
	cases := []struct {
		name string
		got  error
		want string
	}{
		{"versions", ErrNoVersionCatalog, "linear: project versions are not supported on this origin"},
		{"links", ErrNoIssueLinks, "linear: issue links are not supported on this origin"},
		{"create fields", ErrNoCreateFields, "linear: create-time field metadata is not supported on this origin"},
		{"media", ErrNoMediaRef, "linear: inline comment media is not supported; the file is attached to the issue"},
	}
	for _, tc := range cases {
		if tc.got == nil || tc.got.Error() != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestProjectVersionsUnsupported(t *testing.T) {
	w, rec := testLinearWriter(t)
	got, err := AsVersionCatalog(w)
	if err == nil {
		t.Fatal("ProjectVersions succeeded; Linear has no version catalog")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not supported") {
		t.Errorf("error %q, want it to say not supported", err)
	}
	if got != nil {
		t.Errorf("versions %v, want nil on refuse", got)
	}
	if len(rec.queries) != 0 {
		t.Errorf("ProjectVersions hit GraphQL %d times; refuse must stay local", len(rec.queries))
	}
}

func TestIssueLinksUnsupported(t *testing.T) {
	w, rec := testLinearWriter(t)
	got, err := AsIssueLinker(w)
	if err == nil {
		t.Fatal("IssueLinkTypes succeeded; Linear relations are out of scope")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not supported on this origin") {
		t.Errorf("error %q, want it to say not supported on this origin", err)
	}
	if got != nil {
		t.Errorf("types %v, want nil on refuse", got)
	}
	if got == nil {
		err = ErrNoIssueLinks
	} else {
		err = got.LinkIssues(context.Background(), "10000", "FIX-1", "FIX-2")
	}
	if err == nil {
		t.Fatal("LinkIssues succeeded; Linear relations are out of scope")
	} else if !strings.Contains(strings.ToLower(err.Error()), "not supported on this origin") {
		t.Errorf("LinkIssues error %q, want it to say not supported on this origin", err)
	}
	if len(rec.queries) != 0 {
		t.Errorf("issue-link refuse hit GraphQL %d times; refuse must stay local", len(rec.queries))
	}
}

func TestCreateFieldsUnsupported(t *testing.T) {
	w, rec := testLinearWriter(t)
	got, err := AsCreateFieldCatalog(w)
	if err == nil {
		t.Fatal("CreateFields succeeded; Linear has no create-time field metadata")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not supported") {
		t.Errorf("error %q, want it to say not supported", err)
	}
	if got != nil {
		t.Errorf("fields %v, want nil on refuse", got)
	}
	if len(rec.queries) != 0 {
		t.Errorf("CreateFields hit GraphQL %d times; refuse must stay local", len(rec.queries))
	}
}

func TestMediaRefUnsupported(t *testing.T) {
	w, rec := testLinearWriter(t)
	got, err := AsMediaRef(w)
	if err == nil {
		t.Fatal("MediaRef succeeded; Linear has no inline comment media")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not supported") {
		t.Errorf("error %q, want it to say not supported", err)
	}
	if got != nil {
		t.Errorf("media %v, want nil on refuse", got)
	}
	if len(rec.queries) != 0 {
		t.Errorf("MediaRef hit GraphQL %d times; refuse must stay local", len(rec.queries))
	}
}

func TestCreateIssueKeepsSupportedFields(t *testing.T) {
	w, rec := testLinearWriter(t)
	key, err := w.CreateIssue(context.Background(), map[string]any{
		"project":     map[string]any{"key": "FIX"},
		"summary":     "supported create",
		"description": map[string]any{"type": "doc", "version": 1, "content": []any{}},
		"priority":    map[string]any{"id": "2"},
		"duedate":     "2026-09-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if key != "FIX-11" {
		t.Errorf("key = %q, want FIX-11 from the fixture", key)
	}
	if rec.creates != 1 {
		t.Errorf("creates = %d, want 1", rec.creates)
	}
}

// GDK-643: a wrong any must not become an empty string Linear then stores.
// FAIL-first: on the pre-fix source, summary/duedate swallow the type assert
// and the mutation goes out (UpdateFields) or the create succeeds with the
// field omitted (CreateIssue duedate).
func TestUpdateFieldsRejectsWrongValueTypes(t *testing.T) {
	w, rec := testLinearWriter(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		fields map[string]any
		field  string
	}{
		{"summary float64", map[string]any{"summary": 1.0}, "summary"},
		{"duedate float64", map[string]any{"duedate": 1.0}, "duedate"},
		{"priority id float64", map[string]any{"priority": map[string]any{"id": 2.0}}, "priority"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := rec.updates
			err := w.UpdateFields(ctx, "FIX-1", tc.fields)
			if err == nil {
				t.Fatalf("wrong-type %s succeeded (updates=%d); empty value would have been written", tc.field, rec.updates)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.field) {
				t.Errorf("error %q does not name field %q", msg, tc.field)
			}
			if !strings.Contains(msg, "wants string") {
				t.Errorf("error %q, want it to say wants string", msg)
			}
			if !strings.Contains(msg, "float64") {
				t.Errorf("error %q, want it to name got type float64", msg)
			}
			if rec.updates != before {
				t.Errorf("issueUpdate ran; refuse must stay local")
			}
		})
	}
}

func TestCreateIssueRejectsWrongValueTypes(t *testing.T) {
	w, rec := testLinearWriter(t)
	ctx := context.Background()
	project := map[string]any{"key": "FIX"}
	cases := []struct {
		name   string
		fields map[string]any
		field  string
	}{
		{"summary float64", map[string]any{"project": project, "summary": 1.0}, "summary"},
		{"project.key float64", map[string]any{"project": map[string]any{"key": 1.0}, "summary": "typed"}, "project.key"},
		{"duedate float64", map[string]any{"project": project, "summary": "typed", "duedate": 1.0}, "duedate"},
		{"priority id float64", map[string]any{"project": project, "summary": "typed", "priority": map[string]any{"id": 2.0}}, "priority"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := rec.creates
			_, err := w.CreateIssue(ctx, tc.fields)
			if err == nil {
				t.Fatalf("wrong-type %s succeeded (creates=%d); empty value would have been written", tc.field, rec.creates)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.field) {
				t.Errorf("error %q does not name field %q", msg, tc.field)
			}
			if !strings.Contains(msg, "wants string") {
				t.Errorf("error %q, want it to say wants string", msg)
			}
			if !strings.Contains(msg, "float64") {
				t.Errorf("error %q, want it to name got type float64", msg)
			}
			if rec.creates != before {
				t.Errorf("issueCreate ran; refuse must stay local")
			}
		})
	}
}

// carrierTransport records every request the owning http.Client carries.
// A request that goes out on http.DefaultClient never passes through it.
type carrierTransport struct {
	inner http.RoundTripper
	seen  *[]string
}

func (ct carrierTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	*ct.seen = append(*ct.seen, r.Method+" "+r.URL.String())
	return ct.inner.RoundTrip(r)
}

// GDK-636: the storage PUT must ride w.c.HTTP (60s timeout), not
// http.DefaultClient (no timeout). FAIL-first: on the pre-fix source the PUT
// bypasses the recording transport and this test fails.
func TestUploadStoragePutUsesClientHTTP(t *testing.T) {
	var putSeen bool
	var putSig string
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putSeen = true
			putSig = r.Header.Get("x-sig")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(storage.Close)

	issue := issueQueryFixture(t)
	gql := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode graphql: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body.Query, "query Issue("):
			_, _ = w.Write(issue)
		case strings.Contains(body.Query, "mutation FileUpload"):
			_, _ = w.Write([]byte(`{"data":{"fileUpload":{"success":true,"uploadFile":{"uploadUrl":"` + storage.URL + `/put","assetUrl":"https://uploads.example/asset","headers":[{"key":"x-sig","value":"signed"}]}}}}`))
		case strings.Contains(body.Query, "mutation AttachmentCreate"):
			_, _ = w.Write([]byte(`{"data":{"attachmentCreate":{"success":true,"attachment":{"id":"att-1","title":"a.txt","url":"https://uploads.example/asset"}}}}`))
		default:
			t.Errorf("unexpected graphql document: %s", truncate(body.Query, 80))
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(gql.Close)

	c := linear.New("linear-test-key-not-a-real-secret")
	c.Endpoint = gql.URL
	c.Retries, c.Backoff = 1, 0
	var carried []string
	c.HTTP = &http.Client{Transport: carrierTransport{inner: http.DefaultTransport, seen: &carried}, Timeout: c.HTTP.Timeout}
	w := &linearWriter{c: c}

	atts, err := w.Upload(context.Background(), "FIX-1", "a.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 || atts[0].ID != "att-1" {
		t.Fatalf("attachments = %+v, want the fixture attachment", atts)
	}
	if !putSeen {
		t.Fatal("storage PUT never reached the storage server")
	}
	if putSig != "signed" {
		t.Errorf("PUT x-sig = %q, want the fileUpload header sent verbatim", putSig)
	}
	put := false
	for _, u := range carried {
		if strings.HasPrefix(u, http.MethodPut+" ") {
			put = true
		}
	}
	if !put {
		t.Fatalf("storage PUT bypassed w.c.HTTP (http.DefaultClient?); client carried: %v", carried)
	}
}
