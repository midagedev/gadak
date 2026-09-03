package adf

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// canonical is the markdown subset in the form Markdown() emits: tight lists,
// `**`/`_`/`~~` marks, fenced code, GFM table. md → ADF → md must be the
// identity on it — every re-edit of a body written this way otherwise drifts.
const canonical = `# Title

line one
line two **bold** and _em_ and ` + "`code`" + ` and ~~gone~~ [link](https://x.y "T")

## Second

- a
- b
  - b2
  - b3
- c

1. one
2. two

> quoted
> more

` + "```sh" + `
gadak sql "x"
` + "```" + `

---

| h1 | h2 |
| --- | --- |
| c1 | c2 |

plain end`

func TestMarkdownRoundTripIsIdentityOnTheSubset(t *testing.T) {
	adf := FromMarkdown(canonical)
	back := Markdown(adf)
	if back != canonical {
		t.Fatalf("md → ADF → md changed the text\n--- got ---\n%s\n--- want ---\n%s\n--- adf ---\n%s", back, canonical, kinds(adf))
	}
	again := FromMarkdown(back)
	if string(again) != string(adf) {
		t.Fatalf("second parse differs from first:\n%s\nvs\n%s", again, adf)
	}
}

// A rich document as a Jira editor writes it, including literals that would
// read as syntax if not escaped. ADF → md → ADF must be the identity: the
// escaped form parses back to the same nodes and the same text.
func TestADFRoundTripIsIdentityOnTheSubset(t *testing.T) {
	rich := `{"type":"doc","version":1,"content":[
	 {"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Repro 2 * 3"}]},
	 {"type":"paragraph","content":[
	   {"type":"text","text":"a_b and [x] and <b> and "},
	   {"type":"text","text":"bold","marks":[{"type":"strong"}]},
	   {"type":"text","text":" then "},
	   {"type":"text","text":"a|b","marks":[{"type":"code"}]},
	   {"type":"hardBreak"},
	   {"type":"text","text":"second line with ","marks":[]},
	   {"type":"text","text":"site","marks":[{"type":"link","attrs":{"href":"https://example.com/a?b=1"}}]}]},
	 {"type":"bulletList","content":[
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"- not a nested marker"}]}]},
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"item"}]},
	     {"type":"orderedList","attrs":{"order":3},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"three"}]}]}]}]}]},
	 {"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"# not a heading"}]}]},
	 {"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"fmt.Println(\"` + "```" + `\")"}]},
	 {"type":"rule"},
	 {"type":"table","content":[
	   {"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"k"}]}]},{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"v"}]}]}]},
	   {"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]}
	]}`
	want := normalize(t, json.RawMessage(rich))
	md := Markdown(json.RawMessage(rich))
	got := normalize(t, FromMarkdown(md))
	if got != want {
		t.Fatalf("ADF → md → ADF changed the document\n--- md ---\n%s\n--- got ---\n%s\n--- want ---\n%s", md, got, want)
	}
}

// normalize drops what the round trip is allowed to forget: empty marks
// arrays and key order.
func normalize(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	var strip func(any) any
	strip = func(x any) any {
		switch m := x.(type) {
		case map[string]any:
			if marks, ok := m["marks"].([]any); ok && len(marks) == 0 {
				delete(m, "marks")
			}
			for k, c := range m {
				m[k] = strip(c)
			}
		case []any:
			for i, c := range m {
				m[i] = strip(c)
			}
		}
		return x
	}
	b, _ := json.Marshal(strip(v))
	return string(b)
}

// Line semantics (GDK-1384): a blank line is a paragraph break, a single
// newline a hardBreak. The older jira.Doc made a paragraph per line; the
// migration made one text node with the newlines inside — both recover
// through Markdown() to the text the author typed.
func TestBlankLineIsParagraphSingleNewlineIsHardBreak(t *testing.T) {
	adf := FromMarkdown("a\nb\n\nc")
	if k := kinds(adf); k != "doc\n paragraph\n  text\n  hardBreak\n  text\n paragraph\n  text" {
		t.Fatalf("kinds:\n%s", k)
	}
	if Markdown(adf) != "a\nb\n\nc" {
		t.Fatalf("source not recovered: %q", Markdown(adf))
	}
}

// Markdown() on a simple document returns the typed text unescaped — the
// `**` in a paragraph an older jira.Doc wrote is the author's markdown, not
// a literal to protect.
func TestSimpleDocumentIsReturnedAsTypedText(t *testing.T) {
	simple := json.RawMessage(`{"type":"doc","version":1,"content":[
	  {"type":"paragraph","content":[{"type":"text","text":"## Heading"}]},
	  {"type":"paragraph","content":[{"type":"text","text":"**bold** and _em_"}]},
	  {"type":"paragraph"},
	  {"type":"codeBlock","attrs":{"language":"sh"},"content":[{"type":"text","text":"ls"}]}]}`)
	got := Markdown(simple)
	want := "## Heading\n**bold** and _em_\n\n```sh\nls\n```"
	if got != want {
		t.Fatalf("simple source:\n%q\nwant\n%q", got, want)
	}
	if k := kinds(FromMarkdown(got)); !strings.Contains(k, "heading") || !strings.Contains(k, "codeBlock") {
		t.Fatalf("re-reading the recovered source must yield the formatting the author meant:\n%s", k)
	}
}

// Raw HTML never passes through as HTML.
func TestRawHTMLIsText(t *testing.T) {
	adf := FromMarkdown("<script>alert(1)</script>\n\ninline <b>x</b> here")
	if !strings.Contains(PlainText(adf), "<script>alert(1)</script>") {
		t.Fatalf("html block must be a text node: %s", adf)
	}
	if strings.Contains(kinds(adf), "html") {
		t.Fatalf("no html node kinds: %s", kinds(adf))
	}
}

// GDK-1161 as the two mirrors hold it: the Jira original (an older jira.Doc,
// one paragraph per line, 49 of them) and the migrated copy (one paragraph,
// one text node, the newlines inside). Deriving a display document from
// either must find the same headings and lists — the "wall" and the
// original are the same markdown.
func TestGDK1161DerivesTheSameBlocksFromWallAndOriginal(t *testing.T) {
	jira, err := os.ReadFile("testdata/gdk-1161.jira.json")
	if err != nil {
		t.Skip("fixture missing")
	}
	flat, err := os.ReadFile("testdata/gdk-1161.flat.json")
	if err != nil {
		t.Skip("fixture missing")
	}
	if !IsSimple(string(jira)) || !IsSimple(string(flat)) {
		t.Fatal("both fixtures are simple documents by construction")
	}
	fromJira := FromMarkdown(Markdown(json.RawMessage(strings.TrimSpace(string(jira)))))
	fromFlat := FromMarkdown(Markdown(json.RawMessage(strings.TrimSpace(string(flat)))))
	count := func(raw json.RawMessage, kind string) int {
		return strings.Count(kinds(raw), "\n "+kind) + strings.Count(kinds(raw), "\n  "+kind)
	}
	for _, kind := range []string{"heading", "bulletList", "orderedList", "codeBlock"} {
		a, b := count(fromJira, kind), count(fromFlat, kind)
		if a != b {
			t.Errorf("%s: original %d vs wall %d", kind, a, b)
		}
	}
	if count(fromFlat, "heading") == 0 {
		t.Fatalf("the migrated wall must come back with its headings:\n%s", kinds(fromFlat))
	}
	if strings.Contains(string(fromFlat), `"text":"## `) {
		t.Fatalf("a heading marker survived as literal text: %s", fromFlat)
	}
}
