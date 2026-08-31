package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Contract ↔ assertion (GDK-19 link half):
//
//  1. --type blocks → POST type.id=10000, outward=B, inward=A
//     TestLinkOutwardDescriptionMakesFirstArgumentDisplayOutwardDescription
//  2. --type "is blocked by" → outward=A, inward=B
//     TestLinkInwardDescriptionMakesFirstArgumentDisplayInwardDescription
//  3. unknown token lists the catalog; no POST
//     TestLinkUnknownTokenListsCatalogAndDoesNotPOST
//  4. A==B refused locally; no catalog GET
//     TestLinkSelfRefusedWithoutCatalogGET
//  5. success re-reads both keys (search/jql twice)
//     TestLinkOutwardDescriptionMakesFirstArgumentDisplayOutwardDescription
//     (POST /search/jql count)
//  6. Dispatch + help (blocks / is blocked by; distinct from issue --link)
//     TestLinkIsRegisteredAndHelpShowsDirectionExamples

func postedIssueLink(t *testing.T, f *fakeJira) (typeID, outward, inward string) {
	t.Helper()
	body := f.bodies["POST /issueLink"]
	if body == "" {
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
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
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

// Jira renders type.outward on an issue when that issue appears as
// inwardIssue in its issueLinks response. Therefore the first CLI argument
// must be POSTed as inwardIssue when the caller uses an outward description.
func TestLinkOutwardDescriptionMakesFirstArgumentDisplayOutwardDescription(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "blocks"})
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	id, outward, inward := postedIssueLink(t, f)
	if id != "10000" {
		t.Errorf("type.id = %q, want 10000; body %s", id, f.bodies["POST /issueLink"])
	}
	if outward != "NMB-2" || inward != "NMB-1" {
		t.Errorf("outward=%q inward=%q, want NMB-2 / NMB-1", outward, inward)
	}
	if strings.Contains(out, "NMB-1\t완료\t") == false {
		t.Fatalf("stale line %q", out)
	}
	if n := countTagged(f, "GET /issueLinkType"); n != 1 {
		t.Errorf("catalog GET count %d, want 1; calls %v", n, f.calls)
	}
	if n := countTagged(f, "POST /search/jql"); n != 2 {
		t.Errorf("re-read count %d, want 2 for both keys; calls %v", n, f.calls)
	}
}

// Jira renders type.inward on an issue when that issue appears as
// outwardIssue in its issueLinks response. The first CLI argument therefore
// remains outwardIssue for an inward description.
func TestLinkInwardDescriptionMakesFirstArgumentDisplayInwardDescription(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "is blocked by"})
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	id, outward, inward := postedIssueLink(t, f)
	if id != "10000" {
		t.Errorf("type.id = %q, want 10000", id)
	}
	if outward != "NMB-1" || inward != "NMB-2" {
		t.Errorf("outward=%q inward=%q, want NMB-1 / NMB-2", outward, inward)
	}
}

func TestLinkSplitFromMakesFirstArgumentDisplayInwardDescription(t *testing.T) {
	f := newFakeJira(t)
	f.linkTypesJSON = `{"issueLinkTypes":[{"id":"10001","name":"Issue split","outward":"split to","inward":"split from"}]}`
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "split from"})
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	id, outward, inward := postedIssueLink(t, f)
	if id != "10001" || outward != "NMB-1" || inward != "NMB-2" {
		t.Errorf("id=%q outward=%q inward=%q, want 10001 / NMB-1 / NMB-2", id, outward, inward)
	}
}

func TestLinkUnknownTokenListsCatalogAndDoesNotPOST(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "clones"})
	})
	if err == nil {
		t.Fatal("unknown token must fail")
	}
	msg := err.Error()
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
}

func TestLinkSelfRefusedWithoutCatalogGET(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "nmb-1", "--type", "blocks"})
	})
	if err == nil {
		t.Fatal("A==B must fail")
	}
	if !strings.Contains(err.Error(), "cannot link NMB-1 to itself") {
		t.Errorf("error %q", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("A==B must not call the origin: %v", f.calls)
	}
}

func TestLinkTypeIDUsesOutwardConvention(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "10000"})
	})
	if err != nil {
		t.Fatalf("link --type 10000: %v", err)
	}
	id, outward, inward := postedIssueLink(t, f)
	if id != "10000" || outward != "NMB-2" || inward != "NMB-1" {
		t.Errorf("id=%q outward=%q inward=%q, want 10000 / NMB-2 / NMB-1", id, outward, inward)
	}
}

func TestLinkUnknownIDListsCatalogAndDoesNotPOST(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "99999"})
	})
	if err == nil {
		t.Fatal("unknown id must fail")
	}
	if !strings.Contains(err.Error(), `no link type id "99999"`) {
		t.Errorf("error %q", err)
	}
	if f.called("POST /issueLink") {
		t.Errorf("unknown id must not POST: %v", f.calls)
	}
}

func TestLinkAmbiguousTokenRefuses(t *testing.T) {
	f := newFakeJira(t)
	f.linkTypesJSON = `{"issueLinkTypes":[
		{"id":"10000","name":"Blocks","outward":"blocks","inward":"is blocked by"},
		{"id":"10001","name":"Blockers","outward":"blocks","inward":"is blocked by"}]}`
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "blocks"})
	})
	if err == nil {
		t.Fatal("ambiguous token must fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, `link type "blocks" is ambiguous`) {
		t.Errorf("error %q", msg)
	}
	if !strings.Contains(msg, "10000") || !strings.Contains(msg, "10001") {
		t.Errorf("error should list both ids: %q", msg)
	}
	if f.called("POST /issueLink") {
		t.Errorf("ambiguous must not POST: %v", f.calls)
	}
}

func TestLinkSymmetricTypeIsNotAmbiguous(t *testing.T) {
	f := newFakeJira(t)
	f.linkTypesJSON = `{"issueLinkTypes":[
		{"id":"10003","name":"Relates","outward":"relates to","inward":"relates to"}]}`
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "relates to"})
	})
	if err != nil {
		t.Fatalf("symmetric type must resolve, got %v", err)
	}
	id, outward, inward := postedIssueLink(t, f)
	if id != "10003" {
		t.Errorf("type.id = %q, want 10003", id)
	}
	if outward != "NMB-2" || inward != "NMB-1" {
		t.Errorf("outward=%q inward=%q, want NMB-2 / NMB-1", outward, inward)
	}
}

func TestLinkJSONIncludesBothKeysAndType(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "blocks", "--json"})
	})
	if err != nil {
		t.Fatalf("link --json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json %q: %v", out, err)
	}
	if _, ok := doc["issue"].(map[string]any); !ok {
		t.Errorf("missing issue: %s", out)
	}
	keys, _ := doc["keys"].([]any)
	if len(keys) != 2 || keys[0] != "NMB-1" || keys[1] != "NMB-2" {
		t.Errorf("keys = %#v", doc["keys"])
	}
	typ, _ := doc["type"].(map[string]any)
	if typ["id"] != "10000" || typ["name"] != "Blocks" {
		t.Errorf("type = %#v", doc["type"])
	}
}

func TestLinkMissingArgsIsUsage(t *testing.T) {
	_, err := capture(t, func() error { return cmdLink(nil) })
	if err == nil || !strings.Contains(err.Error(), linkUsage) {
		t.Fatalf("no args: %v", err)
	}
	_, err = capture(t, func() error { return cmdLink([]string{"NMB-1", "NMB-2"}) })
	if err == nil || !strings.Contains(err.Error(), linkUsage) {
		t.Fatalf("missing --type: %v", err)
	}
}

func TestLinkIsRegisteredAndHelpShowsDirectionExamples(t *testing.T) {
	run, ok := commands["link"]
	if !ok || run == nil {
		t.Fatal("link missing from dispatch map")
	}
	h, ok := helps["link"]
	if !ok {
		t.Fatal("link missing from helps")
	}
	if !strings.Contains(h.usage, "gadak [--workspace <name>] link") {
		t.Errorf("usage: %s", h.usage)
	}
	if !strings.Contains(h.summary, "issue --link") {
		t.Errorf("summary must distinguish gadak issue --link: %s", h.summary)
	}
	joined := strings.Join(h.examples, "\n")
	if !strings.Contains(joined, "--type blocks") {
		t.Errorf("examples missing --type blocks:\n%s", joined)
	}
	if !strings.Contains(joined, "is blocked by") {
		t.Errorf("examples missing is blocked by:\n%s", joined)
	}
	if !strings.Contains(usage, "link       create an issue link") {
		t.Errorf("top-level Writing-through block missing link:\n%s", usage)
	}
}
