package main

// GDK-45: symbolic refs across the two seed halves. The dataset pair cannot
// hardcode issue keys (the site numbers them), so the --data run hands a
// ref → key map to the --docs run. Everything here is offline — the same
// guarantee the seeder itself gives dry runs.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefKeyMapPairsOrderWithKeys(t *testing.T) {
	order := []SeedIssue{
		{Summary: "no ref"},
		{Summary: "sso", Ref: "auth-sso-timeout"},
		{Summary: "also no ref"},
		{Summary: "upload budget", Ref: "upload-budget"},
	}
	keys := []string{"NMB-1", "NMB-2", "NMB-3", "NMB-4"}
	m, err := refKeyMap(order, keys)
	if err != nil {
		t.Fatalf("refKeyMap: %v", err)
	}
	want := map[string]string{"auth-sso-timeout": "NMB-2", "upload-budget": "NMB-4"}
	if len(m) != len(want) {
		t.Fatalf("map = %v, want %v", m, want)
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("ref %q → %q, want %q", k, m[k], v)
		}
	}
}

func TestRefKeyMapRejectsDuplicateRef(t *testing.T) {
	order := []SeedIssue{{Ref: "dup"}, {Ref: "dup"}}
	if _, err := refKeyMap(order, []string{"NMB-1", "NMB-2"}); err == nil {
		t.Fatal("duplicate ref accepted — half the doc links would resolve to the wrong issue")
	}
}

func TestRefKeyMapRejectsPartialCreation(t *testing.T) {
	order := []SeedIssue{{Ref: "a"}, {Ref: "b"}}
	if _, err := refKeyMap(order, []string{"NMB-1"}); err == nil {
		t.Fatal("length mismatch accepted — index pairing is unreliable after partial creation")
	}
}

func TestResolveDocsRefsSubstitutesBodiesAndComments(t *testing.T) {
	data := &DocsDataset{Pages: []DocsPage{{
		Space:       "ENG",
		Title:       "Incident review",
		BodyStorage: "<p>Follow-up is {{ref:auth-sso-timeout}}.</p>",
		Comments: []DocsComment{
			{BodyStorage: "Tracking {{ref:upload-budget}} too."},
		},
	}}}
	resolved, err := resolveDocsRefs(data, map[string]string{
		"auth-sso-timeout": "NMB-42",
		"upload-budget":    "NMB-7",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != 2 {
		t.Errorf("resolved %d tokens, want 2", resolved)
	}
	if got := data.Pages[0].BodyStorage; !strings.Contains(got, "NMB-42") || strings.Contains(got, "{{ref:") {
		t.Errorf("body = %q, want NMB-42 substituted with no leftover", got)
	}
	if got := data.Pages[0].Comments[0].BodyStorage; !strings.Contains(got, "NMB-7") {
		t.Errorf("comment = %q, want NMB-7", got)
	}
}

func TestResolveDocsRefsErrorsWithoutMap(t *testing.T) {
	data := &DocsDataset{Pages: []DocsPage{{Title: "Runbook", BodyStorage: "{{ref:auth-sso-timeout}}"}}}
	if _, err := resolveDocsRefs(data, nil); err == nil {
		t.Fatal("token with nil map accepted — the doc would ship with the literal token")
	}
}

func TestResolveDocsRefsErrorsOnUnknownRef(t *testing.T) {
	data := &DocsDataset{Pages: []DocsPage{{Title: "Runbook", BodyStorage: "{{ref:nope}}"}}}
	_, err := resolveDocsRefs(data, map[string]string{"other": "NMB-1"})
	if err == nil {
		t.Fatal("unknown ref accepted")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error names the offending symbol: %v", err)
	}
}

func TestResolveDocsRefsErrorsOnMalformedLeftover(t *testing.T) {
	// Unclosed token: the regexp cannot consume it, so it would ride into
	// the page body verbatim. The leftover scan must catch it.
	data := &DocsDataset{Pages: []DocsPage{{Title: "Runbook", BodyStorage: "see {{ref: auth-sso-timeout }} (spaces!)"}}}
	if _, err := resolveDocsRefs(data, map[string]string{"auth-sso-timeout": "NMB-42"}); err == nil {
		t.Fatal("malformed token accepted")
	}
}

func TestResolveDocsRefsCleanDatasetNeedsNoMap(t *testing.T) {
	// Backward compatibility: the committed examples/demo-docs.json carries
	// no tokens, and a run without --refmap must behave exactly as before.
	data := &DocsDataset{Pages: []DocsPage{{Title: "Architecture", BodyStorage: "<p>plain</p>"}}}
	if _, err := resolveDocsRefs(data, nil); err != nil {
		t.Fatalf("clean dataset without map: %v", err)
	}
}

func TestRefMapRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refmap.json")
	m := map[string]string{"b": "NMB-2", "a": "NMB-1"}
	if err := writeRefMap(path, m); err != nil {
		t.Fatalf("write: %v", err)
	}
	back, err := loadRefMap(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(back) != 2 || back["a"] != "NMB-1" || back["b"] != "NMB-2" {
		t.Fatalf("round trip = %v", back)
	}
	// Marshal sorts map keys — a stable shape for review diffs.
	raw, _ := os.ReadFile(path)
	if strings.Index(string(raw), `"a"`) > strings.Index(string(raw), `"b"`) {
		t.Errorf("refmap not key-sorted:\n%s", raw)
	}
}

func TestWriteRefMapRefusesEmpty(t *testing.T) {
	// An empty map means the dataset carries no refs — --refmap was pointed
	// at a dataset that has none, which is almost certainly the wrong file.
	if err := writeRefMap(filepath.Join(t.TempDir(), "refmap.json"), nil); err == nil {
		t.Fatal("empty refmap accepted")
	}
}
