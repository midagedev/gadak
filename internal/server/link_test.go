package server

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
)

func postedIssueLink(t *testing.T, f *fakeJira) (typeID, outward, inward string) {
	t.Helper()
	body := f.bodies["POST /issueLink"]
	if len(body) == 0 {
		t.Fatalf("no POST /issueLink; calls %v", f.calls)
	}
	var sent struct {
		Type struct {
			ID string `json:"id"`
		} `json:"type"`
		OutwardIssue struct {
			Key string `json:"key"`
		} `json:"outwardIssue"`
		InwardIssue struct {
			Key string `json:"key"`
		} `json:"inwardIssue"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("POST /issueLink body %s: %v", body, err)
	}
	return sent.Type.ID, sent.OutwardIssue.Key, sent.InwardIssue.Key
}

func countTagged(f *fakeJira, tag string) int {
	n := 0
	for _, c := range f.calls {
		if c == tag {
			n++
		}
	}
	return n
}

func detailLinkKeys(t *testing.T, h http.Handler, key string) []string {
	t.Helper()
	rec := get(t, h, apiBase+key+"/detail/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s/detail/ → %d %s", key, rec.Code, rec.Body.String())
	}
	var body struct {
		Linked []struct {
			Key string `json:"key"`
		} `json:"linked_issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	keys := make([]string, 0, len(body.Linked))
	for _, l := range body.Linked {
		keys = append(keys, l.Key)
	}
	return keys
}

func TestLinkTypesREST(t *testing.T) {
	_, h, _ := writable(t)
	rec := get(t, h, apiBase+"NMB-1/linktypes/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		LinkTypes []struct {
			ID, Name, Inward, Outward string
		} `json:"link_types"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	if len(body.LinkTypes) != 1 || body.LinkTypes[0].ID != "10000" ||
		body.LinkTypes[0].Name != "Blocks" || body.LinkTypes[0].Outward != "blocks" ||
		body.LinkTypes[0].Inward != "is blocked by" {
		t.Fatalf("catalog %+v", body.LinkTypes)
	}
}

func TestLinkRESTOutwardDescriptionMakesPathIssueDisplayOutwardDescription(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/link/", `{"type":"blocks","key":"NMB-2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	id, outward, inward := postedIssueLink(t, f)
	if id != "10000" || outward != "NMB-2" || inward != "NMB-1" {
		t.Errorf("id=%q outward=%q inward=%q, want 10000 / NMB-2 / NMB-1", id, outward, inward)
	}
	if n := countTagged(f, "GET /issueLinkType"); n != 1 {
		t.Errorf("catalog GET count %d, want 1; calls %v", n, f.calls)
	}
	if n := countTagged(f, "POST /search/jql"); n != 2 {
		t.Errorf("re-read count %d, want 2 for both keys; calls %v", n, f.calls)
	}
	var body struct {
		Issue struct {
			Status string `json:"status"`
		} `json:"issue"`
		Keys []string `json:"keys"`
		Type struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Outward string `json:"outward"`
			Inward  string `json:"inward"`
		} `json:"type"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	if body.Issue.Status != "완료" {
		t.Errorf("stale row returned: %+v", body.Issue)
	}
	if !reflect.DeepEqual(body.Keys, []string{"NMB-1", "NMB-2"}) {
		t.Errorf("keys %+v", body.Keys)
	}
	if body.Type.ID != "10000" || body.Type.Name != "Blocks" {
		t.Errorf("type %+v", body.Type)
	}
}

func TestLinkRESTInwardDescriptionMakesPathIssueDisplayInwardDescription(t *testing.T) {
	f, h, _ := writable(t)

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/link/",
		`{"type":"is blocked by","key":"NMB-2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	id, outward, inward := postedIssueLink(t, f)
	if id != "10000" || outward != "NMB-1" || inward != "NMB-2" {
		t.Errorf("id=%q outward=%q inward=%q, want 10000 / NMB-1 / NMB-2", id, outward, inward)
	}
}

func TestLinkRESTUnknownTypeDoesNotPOST(t *testing.T) {
	f, h, _ := writable(t)
	before := detailLinkKeys(t, h, "NMB-1")

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/link/", `{"type":"clones","key":"NMB-2"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	msg := decode[map[string]string](t, rec)["error"]
	for _, want := range []string{`no link type matching "clones"`, "available:", "Blocks", "blocks", "is blocked by"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if f.called("POST /issueLink") {
		t.Errorf("unknown token must not POST: %v", f.calls)
	}
	if n := countTagged(f, "GET /issueLinkType"); n != 1 {
		t.Errorf("catalog GET count %d, want 1; calls %v", n, f.calls)
	}
	after := detailLinkKeys(t, h, "NMB-1")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("mirror links changed without origin write: before %v after %v", before, after)
	}
}

func TestLinkRESTSelfRefusedNoOrigin(t *testing.T) {
	f, h, _ := writable(t)
	before := detailLinkKeys(t, h, "NMB-1")

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/link/", `{"type":"blocks","key":"nmb-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cannot link NMB-1 to itself") {
		t.Errorf("error %q", rec.Body.String())
	}
	if len(f.calls) != 0 {
		t.Errorf("A==B must not call the origin: %v", f.calls)
	}
	after := detailLinkKeys(t, h, "NMB-1")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("mirror links changed on self-link: before %v after %v", before, after)
	}
}

func TestLinkRESTOriginFailureLeavesMirrorUnchanged(t *testing.T) {
	f, h, _ := writable(t)
	f.status = http.StatusInternalServerError
	f.errBody = `{"errorMessages":["boom"]}`
	before := detailLinkKeys(t, h, "NMB-1")

	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/link/", `{"type":"blocks","key":"NMB-2"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("origin 500 answered 200: %s", rec.Body.String())
	}
	if !f.called("POST /issueLink") {
		t.Fatalf("origin must be called before any mirror write; calls %v", f.calls)
	}
	if countTagged(f, "POST /search/jql") != 0 {
		t.Errorf("failed origin write must not re-read: %v", f.calls)
	}
	after := detailLinkKeys(t, h, "NMB-1")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("mirror mutated without a landed origin write: before %v after %v", before, after)
	}
}

func TestLinkRESTRequiresCredential(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{Projects: []string{"NMB"}})
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, apiBase + "NMB-1/linktypes/", ""},
		{http.MethodPost, apiBase + "NMB-1/link/", `{"type":"blocks","key":"NMB-2"}`},
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
}

func TestResolveLinkTypeMatchesCLI(t *testing.T) {
	cat := []jira.IssueLinkType{
		{ID: "10000", Name: "Blocks", Outward: "blocks", Inward: "is blocked by"},
	}
	lt, inwardDescription, err := origin.ResolveLinkType("blocks", cat)
	if err != nil || lt.ID != "10000" || inwardDescription {
		t.Fatalf("blocks: id=%q inwardDescription=%v err=%v", lt.ID, inwardDescription, err)
	}
	lt, inwardDescription, err = origin.ResolveLinkType("is blocked by", cat)
	if err != nil || lt.ID != "10000" || !inwardDescription {
		t.Fatalf("inward: id=%q inwardDescription=%v err=%v", lt.ID, inwardDescription, err)
	}
	lt, inwardDescription, err = origin.ResolveLinkType("10000", cat)
	if err != nil || lt.ID != "10000" || inwardDescription {
		t.Fatalf("id: id=%q inwardDescription=%v err=%v", lt.ID, inwardDescription, err)
	}
	_, _, err = origin.ResolveLinkType("clones", cat)
	if err == nil || !strings.Contains(err.Error(), `no link type matching "clones"`) {
		t.Fatalf("unknown: %v", err)
	}

	sym := []jira.IssueLinkType{
		{ID: "10003", Name: "Relates", Outward: "relates to", Inward: "relates to"},
	}
	lt, inwardDescription, err = origin.ResolveLinkType("relates to", sym)
	if err != nil || lt.ID != "10003" || inwardDescription {
		t.Fatalf("symmetric: id=%q inwardDescription=%v err=%v", lt.ID, inwardDescription, err)
	}

	amb := []jira.IssueLinkType{
		{ID: "10000", Name: "Blocks", Outward: "blocks", Inward: "is blocked by"},
		{ID: "10001", Name: "Blockers", Outward: "blocks", Inward: "is blocked by"},
	}
	_, _, err = origin.ResolveLinkType("blocks", amb)
	if err == nil || !strings.Contains(err.Error(), `link type "blocks" is ambiguous`) {
		t.Fatalf("ambiguous: %v", err)
	}
}

// GDK-1983: a phrase two types share is a choice the UI offers and the server
// refuses. Jira lets every type carry free-text inward/outward descriptions
// and enforces no uniqueness across types, so "blocks" naming both a Blocks
// and a Gates type is an ordinary custom catalog — and the phrase fold has
// nothing left to decide between them. Neither site measured on 2026-09-17
// has such a pair today, which is why this arrived as a latent defect rather
// than a report; the catalog below is the one that makes it live.
//
// The fix is not "send the id" — the phrase carries (type, direction) as a
// pair, and internal/server/link.go reads the direction out of it to decide
// which end the requesting issue takes. It is "send the identity and the
// direction separately", which is what the machine path does.
//
// FAIL-first, measured 2026-09-18 with the machine branch taken out of
// internal/server/link.go — which is what the tree looked like before this
// change, since a client had no way to send an identity at all:
//
//	machine path status 400: {"error":"empty link type"}
//
// The first half is red by construction on both sources and stays that way:
// it is the assertion that the phrase fold refuses what it cannot decide,
// which is correct behaviour and the reason the second half has to exist.
func TestLinkRESTAmbiguousPhraseRefusesAndTheChosenIDDoesNot(t *testing.T) {
	ambiguous := `{"issueLinkTypes":[` +
		`{"id":"10000","name":"Blocks","outward":"blocks","inward":"is blocked by"},` +
		`{"id":"10100","name":"Gates","outward":"blocks","inward":"is gated by"}]}`

	// The human path cannot answer, and says so rather than guessing.
	f, h, _ := writable(t)
	f.linkTypesJSON = ambiguous
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/link/", `{"type":"blocks","key":"NMB-2"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("phrase path status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ambiguous") {
		t.Errorf("phrase refusal must name the ambiguity: %s", rec.Body.String())
	}

	// The machine path carries the identity the client was handed, so there
	// is nothing to be ambiguous about.
	f2, h2, _ := writable(t)
	f2.linkTypesJSON = ambiguous
	rec = send(t, h2, http.MethodPost, apiBase+"NMB-1/link/",
		`{"type_id":"10100","direction":"outward","key":"NMB-2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("machine path status %d: %s", rec.Code, rec.Body.String())
	}
	id, outward, inward := postedIssueLink(t, f2)
	if id != "10100" || outward != "NMB-2" || inward != "NMB-1" {
		t.Errorf("id=%q outward=%q inward=%q, want 10100 / NMB-2 / NMB-1", id, outward, inward)
	}

	// The other direction puts the requesting issue on the other end, which
	// is the whole reason the phrase was carrying two things.
	f3, h3, _ := writable(t)
	f3.linkTypesJSON = ambiguous
	rec = send(t, h3, http.MethodPost, apiBase+"NMB-1/link/",
		`{"type_id":"10100","direction":"inward","key":"NMB-2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("inward status %d: %s", rec.Code, rec.Body.String())
	}
	id, outward, inward = postedIssueLink(t, f3)
	if id != "10100" || outward != "NMB-1" || inward != "NMB-2" {
		t.Errorf("id=%q outward=%q inward=%q, want 10100 / NMB-1 / NMB-2", id, outward, inward)
	}
}

// The machine path refuses what it cannot answer exactly, and the refusal
// calls the id by its name — it never falls back to the phrase fold, because
// falling back is the behaviour this separation exists to remove.
func TestLinkRESTMachinePathRefusesAnUnknownIDAndABadDirection(t *testing.T) {
	_, h, _ := writable(t)
	rec := send(t, h, http.MethodPost, apiBase+"NMB-1/link/",
		`{"type_id":"99999","direction":"outward","key":"NMB-2"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "99999") {
		t.Errorf("unknown id → %d %s; want 400 naming the id", rec.Code, rec.Body.String())
	}

	_, h2, _ := writable(t)
	rec = send(t, h2, http.MethodPost, apiBase+"NMB-1/link/",
		`{"type_id":"10000","key":"NMB-2"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "direction") {
		t.Errorf("missing direction → %d %s; want 400 naming the direction", rec.Code, rec.Body.String())
	}

	// And a body with neither spelling still asks for a type.
	_, h3, _ := writable(t)
	rec = send(t, h3, http.MethodPost, apiBase+"NMB-1/link/", `{"key":"NMB-2"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "type_required") {
		t.Errorf("no type → %d %s; want 400 type_required", rec.Code, rec.Body.String())
	}
}
