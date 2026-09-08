package adf

import (
	"encoding/json"
	"strings"
	"testing"
)

// The Jira Server body from the GDK-1637 measurement: what a live Server
// 11.3.11 sends back — and what went in — byte for byte.
const wikiBody = "h2. Heading\n\n*bold* and {{code}}\n\n* one\n* two\n\n{code:java}\nSystem.out.println(1);\n{code}\n"

// kindsOf lists a display document's top-level node types.
func kindsOf(raw json.RawMessage) string {
	var n struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	_ = json.Unmarshal(raw, &n)
	parts := make([]string, 0, len(n.Content))
	for _, c := range n.Content {
		parts = append(parts, c.Type)
	}
	return strings.Join(parts, ",")
}

// GDK-1637: on a Jira Server origin the body is wiki markup carried verbatim.
// Source is the input unchanged — the editor opens those same characters — and
// Display is a single codeBlock holding them, never the markdown parse (h2.
// must not become a heading, *bold* must not become emphasis).
func TestPresentWikiMarkupCarriedVerbatim(t *testing.T) {
	p := Present(nil, wikiBody, DialectWiki)
	if p.Source != wikiBody {
		t.Fatalf("source must be the wiki markup unchanged:\n%q", p.Source)
	}
	if got := kindsOf(p.Display); got != "codeBlock" {
		t.Fatalf("display must be one codeBlock, got %q:\n%s", got, p.Display)
	}
	var block struct {
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(p.Display, &block); err != nil {
		t.Fatalf("display is not a document: %v", err)
	}
	if len(block.Content) != 1 || len(block.Content[0].Content) != 1 || block.Content[0].Content[0].Text != wikiBody {
		t.Fatalf("the codeBlock must hold the body verbatim: %s", p.Display)
	}
	if len(p.Loss) != 0 {
		t.Fatalf("a verbatim carry loses nothing: %v", p.Loss)
	}
}

// The sync copies fields.description as it arrives (sync.go), so a Server
// mirror's ADF column holds the origin's JSON string, not nothing. That is
// still "no document" on a wiki dialect: the same verbatim presentation as an
// empty column, not the rich-ADF branch a non-simple raw would otherwise take.
func TestPresentWikiStringColumnPresentsAsVerbatim(t *testing.T) {
	quoted, err := json.Marshal(wikiBody)
	if err != nil {
		t.Fatal(err)
	}
	p := Present(quoted, wikiBody, DialectWiki)
	if p.Source != wikiBody {
		t.Fatalf("source must be the wiki markup unchanged:\n%q", p.Source)
	}
	if got := kindsOf(p.Display); got != "codeBlock" {
		t.Fatalf("display must be one codeBlock, got %q:\n%s", got, p.Display)
	}
}

// An empty wiki body presents empty, exactly as the markdown dialect does.
func TestPresentWikiEmpty(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage(`""`)} {
		p := Present(raw, "", DialectWiki)
		if p.Display != nil || p.Source != "" || p.Loss != nil {
			t.Fatalf("empty wiki body presents empty: %+v", p)
		}
	}
}

// The markdown dialect keeps today's behaviour on the same input — which is
// the misreading DialectWiki exists to avoid: `h2.` stays a literal paragraph
// (it is not a markdown heading), while `*bold*`, a construct the two syntaxes
// share by accident, becomes emphasis the origin did not mean.
func TestPresentMarkdownDialectUnchangedOnWikiSample(t *testing.T) {
	p := Present(nil, wikiBody, DialectMarkdown)
	if p.Source != wikiBody {
		t.Fatalf("markdown source: %q", p.Source)
	}
	if k := kindsOf(p.Display); !strings.HasPrefix(k, "paragraph") || !strings.Contains(string(p.Display), `"em"`) {
		t.Fatalf("markdown dialect must keep its own parse:\n%s\n%s", k, p.Display)
	}
}

// DialectWiki with a real ADF document cannot happen on a Server origin, so
// it keeps the markdown behaviour rather than growing a branch nobody reaches.
func TestPresentWikiWithRealDocKeepsMarkdownBehaviour(t *testing.T) {
	wall := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"## Symptom\n\nfirst"}]}]}`)
	p := Present(wall, "## Symptom\n\nfirst", DialectWiki)
	if k := kindsOf(p.Display); !strings.Contains(k, "heading") {
		t.Fatalf("a document on a wiki origin still reads as blocks:\n%s", k)
	}
}
