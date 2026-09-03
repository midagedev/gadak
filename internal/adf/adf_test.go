package adf

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

func TestIsSimpleEmptyAndParagraphs(t *testing.T) {
	if !IsSimple("") || !IsSimple("   ") {
		t.Fatal("empty/whitespace must be simple")
	}
	simple := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"plain"}]}]}`
	if !IsSimple(simple) {
		t.Fatal("paragraph-only doc must be simple")
	}
	hardBreak := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]}]}`
	if !IsSimple(hardBreak) {
		t.Fatal("hardBreak inside a paragraph must be simple")
	}
}

func TestIsSimpleRejectsMarksListsHeadings(t *testing.T) {
	cases := map[string]string{
		"strong":  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"strong"}]}]}]}`,
		"heading": `{"type":"doc","version":1,"content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Steps"}]}]}`,
		"list":    `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"item"}]}]}]}]}`,
		"mention": `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"acc-1","text":"@Dana"}}]}]}`,
		"garbage": `not json`,
	}
	for name, raw := range cases {
		if IsSimple(raw) {
			t.Errorf("%s: want not simple", name)
		}
	}
}

func TestFormatLoss(t *testing.T) {
	// GDK-1384: the edit surface is markdown, so the subset markdown carries
	// (marks strong/em/code/strike/link, headings, lists, tables with
	// single-paragraph cells) is no longer loss. Loss is what has no
	// markdown: panel, media, mention, status, textColor, …
	table := `{"type":"doc","version":1,"content":[{"type":"table","content":[{"type":"tableRow","content":[` +
		`{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"c"}]}]}]}]}]}`
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"null description", `null`, nil},
		{"plain paragraphs", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}`, nil},
		{"hardBreak is plain", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"}]}]}`, nil},
		{"bare string is already plain", `"wiki markup"`, nil},
		{"strong is markdown", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"b","marks":[{"type":"strong"}]}]}]}`, nil},
		{"link is markdown", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"l","marks":[{"type":"link","attrs":{"href":"https://example.com"}}]}]}]}`, nil},
		{"heading and list are markdown", `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"h"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"i"}]}]}]}]}`, nil},
		{"table is markdown", table, nil},
		{"textColor names the mark", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"b","marks":[{"type":"textColor","attrs":{"color":"#ff0000"}}]}]}]}`, []string{"textColor"}},
		{"panel walks into its paragraph", `{"type":"doc","content":[{"type":"panel","attrs":{"panelType":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"p"}]}]}]}`, []string{"panel"}},
		{"mention", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"a","text":"@D"}}]}]}`, []string{"mention"}},
		{"media is loss down to the leaf", `{"type":"doc","content":[{"type":"mediaSingle","content":[{"type":"media","attrs":{"id":"x"}}]}]}`, []string{"mediaSingle", "media"}},
		// Two panels in one doc: each kind appears once.
		{"dedup keeps one of each kind", `{"type":"doc","content":[{"type":"panel","content":[{"type":"paragraph"}]},{"type":"panel","content":[{"type":"paragraph"}]}]}`, []string{"panel"}},
		{"garbage is reported, never waved through", `not json`, []string{unreadableDescription}},
	}

	for _, tc := range cases {
		got := FormatLoss(tc.raw)
		if len(got) != len(tc.want) {
			t.Errorf("%s: FormatLoss = %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: FormatLoss = %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}
