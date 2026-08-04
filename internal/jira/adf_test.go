package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlainTextNestedDocument(t *testing.T) {
	doc := json.RawMessage(`{"type":"doc","version":1,"content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Steps"}]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[
				{"type":"text","text":"open "},
				{"type":"text","text":"the editor","marks":[{"type":"strong"}]}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"press save"}]}]}]},
		{"type":"paragraph","content":[
			{"type":"mention","attrs":{"id":"acc-1","text":"@Dana"}},
			{"type":"text","text":" it crashes"},
			{"type":"hardBreak"},
			{"type":"text","text":"every time"}]},
		{"type":"codeBlock","content":[{"type":"text","text":"panic: nil map"}]}]}`)

	got := PlainText(doc)
	for _, want := range []string{"Steps", "open the editor", "press save", "@Dana it crashes", "every time", "panic: nil map"} {
		if !strings.Contains(got, want) {
			t.Errorf("flattened text missing %q\ngot:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{") || strings.Contains(got, "doc") {
		t.Errorf("flattened text leaked ADF structure:\n%s", got)
	}
	// Block nodes end a line, inline marks do not.
	if lines := strings.Split(got, "\n"); len(lines) < 4 {
		t.Errorf("expected one line per block, got %d:\n%s", len(lines), got)
	}
}

func TestPlainTextEdgeCases(t *testing.T) {
	if got := PlainText(nil); got != "" {
		t.Errorf("empty raw: %q", got)
	}
	// A field holding a bare string (older wiki markup) passes through.
	if got := PlainText(json.RawMessage(`"just text"`)); got != "just text" {
		t.Errorf("string field: %q", got)
	}
	if got := PlainText(json.RawMessage(`not json`)); got != "" {
		t.Errorf("garbage: %q", got)
	}
}

func TestISOTimeNormalizesToUTC(t *testing.T) {
	if got := ISOTime("2026-08-04T18:15:00.482+0900"); got != "2026-08-04T09:15:00.482Z" {
		t.Errorf("offset timestamp: %q", got)
	}
	if got := ISOTime("2026-08-04T09:15:00Z"); got != "2026-08-04T09:15:00.000Z" {
		t.Errorf("rfc3339: %q", got)
	}
	if got := ISOTime("whenever"); got != "whenever" {
		t.Errorf("unparseable should pass through: %q", got)
	}
}

func TestCategoryNormalizesJiraKeys(t *testing.T) {
	for key, want := range map[string]string{
		"new": "new", "indeterminate": "inprogress", "done": "done", "undefined": "new", "": "new",
	} {
		if got := Category(key); got != want {
			t.Errorf("Category(%q) = %q, want %q", key, got, want)
		}
	}
}
