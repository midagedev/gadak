package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDevPRStatus(t *testing.T) {
	for in, want := range map[string]DevPRStatus{
		"OPEN": DevPROpen, "open": DevPROpen, " Open ": DevPROpen,
		"MERGED": DevPRMerged, "merged": DevPRMerged,
		"DECLINED": DevPRDeclined, "declined": DevPRDeclined,
	} {
		got, ok := ParseDevPRStatus(in)
		if !ok || got != want {
			t.Errorf("ParseDevPRStatus(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	if _, ok := ParseDevPRStatus("CLOSED"); ok {
		t.Error("CLOSED is a GitHub token, not an origin status")
	}
	if _, ok := ParseDevPRStatus(""); ok {
		t.Error("empty parsed as a status")
	}
}

func TestDevPRStatusFromGitHub(t *testing.T) {
	for in, want := range map[string]DevPRStatus{
		"OPEN": DevPROpen, "open": DevPROpen, "weird": DevPROpen,
		"MERGED": DevPRMerged, "merged": DevPRMerged,
		"CLOSED": DevPRDeclined, "closed": DevPRDeclined,
	} {
		if got := DevPRStatusFromGitHub(in); got != want {
			t.Errorf("DevPRStatusFromGitHub(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDevPRStatusStored(t *testing.T) {
	if got := DevPROpen.Stored(); got != "open" {
		t.Errorf("OPEN.Stored() = %q, want open", got)
	}
	if got := DevPRMerged.Stored(); got != "merged" {
		t.Errorf("MERGED.Stored() = %q, want merged", got)
	}
	if got := DevPRDeclined.Stored(); got != "declined" {
		t.Errorf("DECLINED.Stored() = %q, want declined", got)
	}
}

// devDetailContract is the detail payload both origins serve once GDK-589
// lands: Cloud's own vocabulary (author{name}, source{branch}) plus
// issuetap's actor extension (accountId/displayName — Cloud has none, the
// fields stay empty there). The test round-trips DevStatusPRs's answer
// through JSON so the assertion is on the wire vocabulary, not on Go field
// names that could exist and still never carry the payload.
func TestDevStatusPRsParsesAuthorBranchActor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
		  "errors": [],
		  "detail": [{
		    "_instance": {"name": "GitHub", "type": "GitHub", "id": "com.atlassian.github", "singleInstance": true},
		    "pullRequests": [{
		      "id": "pr-1",
		      "url": "https://github.com/o/r/pull/1",
		      "name": "GDK-589 carry author",
		      "status": "OPEN",
		      "author": {"name": "midagedev"},
		      "source": {"branch": "gdk-589-dev-link-actor"},
		      "actor": {"accountId": "claude:354bff2b", "displayName": "Claude (build 1)"}
		    }]
		  }]
		}`))
	}))
	t.Cleanup(ts.Close)

	prs, err := New(ts.URL, "e@xample.test", "t").DevStatusPRs(t.Context(), "10001")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	raw, err := json.Marshal(prs[0])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if author, _ := got["author"].(map[string]any); author["name"] != "midagedev" {
		t.Errorf("author = %v, want name midagedev (full PR: %s)", got["author"], raw)
	}
	if source, _ := got["source"].(map[string]any); source["branch"] != "gdk-589-dev-link-actor" {
		t.Errorf("source = %v, want branch gdk-589-dev-link-actor (full PR: %s)", got["source"], raw)
	}
	actor, _ := got["actor"].(map[string]any)
	if actor["accountId"] != "claude:354bff2b" || actor["displayName"] != "Claude (build 1)" {
		t.Errorf("actor = %v, want accountId claude:354bff2b / displayName Claude (build 1)", got["actor"])
	}
}

// TestLinkDevPRCarriesAuthorBranch pins the POST half of GDK-589: author and
// branch ride the body only when non-empty (issuetap's keep-rule — a re-link
// without them preserves what the origin holds; an older origin ignores both
// keys), and the 201 answer's nested blocks parse back in.
func TestLinkDevPRCarriesAuthorBranch(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/issue/link") {
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = map[string]any{} // per-request: Unmarshal into a used map merges
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("body is not JSON: %v (%s)", err, raw)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
		  "id": "pr-7", "url": "https://github.com/o/r/pull/7", "name": "GDK-589",
		  "status": "OPEN",
		  "author": {"name": "midagedev"},
		  "source": {"branch": "gdk-589-dev-link-actor"},
		  "actor": {"accountId": "claude:354bff2b", "displayName": "Claude (build 1)"}
		}`))
	}))
	t.Cleanup(ts.Close)
	c := New(ts.URL, "e@xample.test", "t")

	created, err := c.LinkDevPR(t.Context(), "10001", "https://github.com/o/r/pull/7", "GDK-589",
		"midagedev", "gdk-589-dev-link-actor", DevPROpen)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gotBody["author"]; !ok || gotBody["author"] != "midagedev" {
		t.Errorf("POST author = %v, want the flat string midagedev", gotBody["author"])
	}
	if _, ok := gotBody["branch"]; !ok || gotBody["branch"] != "gdk-589-dev-link-actor" {
		t.Errorf("POST branch = %v, want gdk-589-dev-link-actor", gotBody["branch"])
	}
	if created.Author.Name != "midagedev" || created.Source.Branch != "gdk-589-dev-link-actor" ||
		created.Actor.AccountID != "claude:354bff2b" || created.Actor.DisplayName != "Claude (build 1)" {
		t.Errorf("201 decode = %+v, want author/source/actor blocks parsed", created)
	}

	if _, err := c.LinkDevPR(t.Context(), "10001", "https://github.com/o/r/pull/8", "plain",
		"", "", DevPROpen); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotBody["author"]; ok {
		t.Errorf("empty author still posted: %v", gotBody["author"])
	}
	if _, ok := gotBody["branch"]; ok {
		t.Errorf("empty branch still posted: %v", gotBody["branch"])
	}
}

// TestParseDevBuildState pins the build state vocabulary (GDK-592): the
// three buckets the dev-status summary counts. Deployment states are
// deliberately free-form — only "successful" is load-bearing there — so
// they get no closed parser.
func TestParseDevBuildState(t *testing.T) {
	for in, want := range map[string]DevBuildState{
		"successful": DevBuildSuccessful, "SUCCESSFUL": DevBuildSuccessful,
		"failed": DevBuildFailed, "Failed": DevBuildFailed,
		"unknown": DevBuildUnknown,
	} {
		got, ok := ParseDevBuildState(in)
		if !ok || got != want {
			t.Errorf("ParseDevBuildState(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"pending", "in progress", ""} {
		if _, ok := ParseDevBuildState(in); ok {
			t.Errorf("ParseDevBuildState(%q) accepted a non-bucket state", in)
		}
	}
}

// TestLinkDevDeploymentAndBuildBody pins the write side of GDK-592: the
// link POST carries kind=deployment/build with the per-kind fields, and
// omits the optional url/number keys when empty so the origin keeps what
// it holds (same rule as LinkDevPR's author/branch).
func TestLinkDevDeploymentAndBuildBody(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/issue/link") {
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = map[string]any{}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("body is not JSON: %v (%s)", err, raw)
		}
		w.WriteHeader(http.StatusCreated)
		kind := gotBody["kind"]
		switch kind {
		case "deployment":
			_, _ = w.Write([]byte(`{"id":"environment:production","environment":"production","state":"successful","lastUpdate":"2026-08-22T00:00:00.000+0000"}`))
		case "build":
			_, _ = w.Write([]byte(`{"id":"https://ci.example/gadak/build/592","url":"https://ci.example/gadak/build/592","number":"592","state":"failed","lastUpdate":"2026-08-22T00:00:00.000+0000"}`))
		default:
			t.Errorf("unexpected kind %v", kind)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(ts.Close)
	c := New(ts.URL, "e@xample.test", "t")

	dep, err := c.LinkDevDeployment(t.Context(), "10001", "production", "successful", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["kind"] != "deployment" || gotBody["environment"] != "production" || gotBody["state"] != "successful" {
		t.Errorf("deployment body = %v", gotBody)
	}
	if _, ok := gotBody["url"]; ok {
		t.Errorf("empty url still posted: %v", gotBody["url"])
	}
	if dep.ID != "environment:production" || dep.Environment != "production" || dep.State != "successful" {
		t.Errorf("deployment 201 decode = %+v", dep)
	}

	b, err := c.LinkDevBuild(t.Context(), "10001", DevBuildFailed, "592", "https://ci.example/gadak/build/592")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["kind"] != "build" || gotBody["state"] != "failed" || gotBody["number"] != "592" {
		t.Errorf("build body = %v", gotBody)
	}
	if b.Number != "592" || b.State != "failed" || b.URL != "https://ci.example/gadak/build/592" {
		t.Errorf("build 201 decode = %+v", b)
	}

	// A number-less build posts no number key; the url is the key then.
	if _, err := c.LinkDevBuild(t.Context(), "10001", DevBuildSuccessful, "", "https://ci.example/x/7"); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotBody["number"]; ok {
		t.Errorf("empty number still posted: %v", gotBody["number"])
	}
}
