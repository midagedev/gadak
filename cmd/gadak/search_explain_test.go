package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

func TestSearchExplainJSON(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSearch([]string{"--json", "--explain", "--limit", "5", "NMB-140"})
	})
	if err != nil {
		t.Fatalf("search --explain: %v\n%s", err, out)
	}
	var body struct {
		Issues  []store.IssueLite     `json:"issues"`
		Explain []store.SearchExplain `json:"explain"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(body.Issues) == 0 || body.Issues[0].IssueKey != "NMB-140" {
		t.Fatalf("issues = %+v, want NMB-140 first", body.Issues)
	}
	if len(body.Explain) == 0 || body.Explain[0].Key != "NMB-140" || body.Explain[0].Reason != "key-exact" {
		t.Fatalf("explain = %+v, want NMB-140 key-exact first", body.Explain)
	}
}

func TestSearchHelpDocumentsExplain(t *testing.T) {
	out, err := capture(t, func() error { return cmdSearch([]string{"--help"}) })
	if err != nil {
		t.Fatalf("search --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--explain") {
		t.Fatalf("search --help missing --explain:\n%s", out)
	}
	if !strings.Contains(out, "key-exact") || !strings.Contains(out, "key-prefix") || !strings.Contains(out, "fts") {
		t.Fatalf("search --help --explain text does not name the reasons:\n%s", out)
	}
}
