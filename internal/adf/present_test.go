package adf

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPresentLinearMarkdownRendersAsBlocks(t *testing.T) {
	p := Present(nil, "## Repro\n\n- one\n- two")
	if p.Source != "## Repro\n\n- one\n- two" {
		t.Fatalf("source must be the markdown as stored: %q", p.Source)
	}
	if k := kinds(p.Display); !strings.Contains(k, "heading") || !strings.Contains(k, "bulletList") {
		t.Fatalf("display must be the parsed markdown:\n%s", k)
	}
	if len(p.Loss) != 0 {
		t.Fatalf("markdown loses nothing: %v", p.Loss)
	}
}

func TestPresentSimpleADFIsReadAsMarkdown(t *testing.T) {
	// One text node with the newlines inside — the migrated shape (GDK-1382).
	wall := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"## Symptom\n\nfirst\n\n- a\n- b"}]}]}`)
	p := Present(wall, "## Symptom\n\nfirst\n\n- a\n- b")
	if k := kinds(p.Display); !strings.Contains(k, "heading") || !strings.Contains(k, "bulletList") {
		t.Fatalf("the wall must come back as blocks:\n%s", k)
	}
	if p.Source != "## Symptom\n\nfirst\n\n- a\n- b" {
		t.Fatalf("source is the typed text: %q", p.Source)
	}
	if len(p.Loss) != 0 {
		t.Fatalf("a simple body loses nothing: %v", p.Loss)
	}
}

func TestPresentRichADFIsDisplayedAsIsAndNamesLoss(t *testing.T) {
	rich := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"panel","attrs":{"panelType":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"note","marks":[{"type":"strong"}]}]}]}]}`)
	p := Present(rich, "note")
	if string(p.Display) != string(rich) {
		t.Fatalf("rich ADF must be displayed verbatim")
	}
	// GDK-1396: the panel stands in the source as a placeholder pair, its
	// interior as markdown.
	if !strings.HasPrefix(p.Source, "<!-- adf:1:") || !strings.HasSuffix(p.Source, " panel info -->\n\n**note**\n\n<!-- /adf:1 -->") {
		t.Fatalf("source serializes the rich body with placeholders: %q", p.Source)
	}
	if len(p.Loss) != 1 || p.Loss[0] != "panel" {
		t.Fatalf("loss must name the panel only (strong is markdown): %v", p.Loss)
	}
}

func TestPresentEmpty(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage(`""`)} {
		p := Present(raw, "")
		if p.Display != nil || p.Source != "" || p.Loss != nil {
			t.Fatalf("empty body presents empty: %+v", p)
		}
	}
}
