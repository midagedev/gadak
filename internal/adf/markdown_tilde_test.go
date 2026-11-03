package adf

import (
	"encoding/json"
	"testing"
)

// GDK-2062: goldmark's Strikethrough extension accepts a single `~` as a
// delimiter, so two unrelated ranges in one paragraph — "61~69% … 36~37%"
// — became one strike: the tildes vanished and the text between them was
// struck on the origin. A run of exactly one `~` is a literal everywhere;
// `~~text~~` is strikethrough. The wants below are ADF blocks; the test
// wraps them in a doc.
var tildeCases = []struct {
	name string
	src  string
	want string
}{
	{"range", "61~69%",
		`{"type":"paragraph","content":[{"type":"text","text":"61~69%"}]}`},
	{"two ranges in one paragraph", "a 61~69% b 36~37%.",
		`{"type":"paragraph","content":[{"type":"text","text":"a 61~69% b 36~37%."}]}`},
	{"word-wrapped pair", "~a~",
		`{"type":"paragraph","content":[{"type":"text","text":"~a~"}]}`},
	{"space-flanked pair", "a ~ b ~ c",
		`{"type":"paragraph","content":[{"type":"text","text":"a ~ b ~ c"}]}`},
	{"double tilde still strikes", "~~a~~",
		`{"type":"paragraph","content":[{"type":"text","text":"a","marks":[{"type":"strike"}]}]}`},
	{"strike beside a literal range", "~~a~~ and 1~2",
		`{"type":"paragraph","content":[{"type":"text","text":"a","marks":[{"type":"strike"}]},{"type":"text","text":" and 1~2"}]}`},
	{"backslash-escaped", `\~a\~`,
		`{"type":"paragraph","content":[{"type":"text","text":"~a~"}]}`},
	{"code span", "`~a~`",
		`{"type":"paragraph","content":[{"type":"text","text":"~a~","marks":[{"type":"code"}]}]}`},
	{"tilde fence still opens a code block", "~~~\n1~2\n~~~",
		`{"type":"codeBlock","content":[{"type":"text","text":"1~2"}]}`},
	{"link text and destination", "[1~2](https://example.com/a~b)",
		`{"type":"paragraph","content":[{"type":"text","text":"1~2","marks":[{"type":"link","attrs":{"href":"https://example.com/a~b"}}]}]}`},
	{"korean sentence", "범위는 3~5일, 비용은 10~20만 원",
		`{"type":"paragraph","content":[{"type":"text","text":"범위는 3~5일, 비용은 10~20만 원"}]}`},
	{"lone tilde line", "~",
		`{"type":"paragraph","content":[{"type":"text","text":"~"}]}`},
	// goldmark's strikethrough parser rejects runs longer than two
	// (OriginalLength > 2), so three tildes are text today; this pins that
	// the replacement parser keeps it.
	{"inline run of three stays literal", "x ~~~y~~~ z",
		`{"type":"paragraph","content":[{"type":"text","text":"x ~~~y~~~ z"}]}`},
	{"unmatched double tilde is text", "a ~~ b",
		`{"type":"paragraph","content":[{"type":"text","text":"a ~~ b"}]}`},
}

func TestSingleTildeIsLiteral(t *testing.T) {
	for _, tc := range tildeCases {
		t.Run(tc.name, func(t *testing.T) {
			want := normalize(t, json.RawMessage(`{"type":"doc","version":1,"content":[`+tc.want+`]}`))
			if got := normalize(t, FromMarkdown(tc.src)); got != want {
				t.Errorf("FromMarkdown(%q):\n got %s\nwant %s", tc.src, got, want)
			}
		})
	}
	// The report case comes back for editing as it was typed: a simple
	// document is returned verbatim, tildes included.
	if got := Markdown(FromMarkdown("a 61~69% b 36~37%.")); got != "a 61~69% b 36~37%." {
		t.Errorf("Markdown of the report case = %q, want the typed line", got)
	}
}

// The round trip GDK-2062 buys: the markdown the writer recovers from each
// case's ADF must parse back to the same ADF. Before the fix the writer
// emitted a lone `~` unescaped (escapeInline escapes `~` only next to
// another `~`) while the parser read one `~` as markup, so a body with two
// ranges changed on every re-edit.
func TestSingleTildeRoundTrip(t *testing.T) {
	for _, tc := range tildeCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := FromMarkdown(tc.src)
			md := Markdown(doc)
			if got := normalize(t, FromMarkdown(md)); got != normalize(t, doc) {
				t.Errorf("round trip of %q via %q:\n got %s\nwant %s", tc.src, md, got, normalize(t, doc))
			}
		})
	}
}
