package adf

import (
	"encoding/json"
	"strings"
	"testing"
)

// textMarks walks a converted document and returns, per text node, the text
// and its mark types in order.
func textMarks(t *testing.T, raw json.RawMessage) map[string][]string {
	t.Helper()
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["type"] == "text" {
				var ms []string
				raw, _ := x["marks"].([]any)
				for _, m := range raw {
					ms = append(ms, m.(map[string]any)["type"].(string))
				}
				out[x["text"].(string)] = ms
				return
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(doc)
	return out
}

// GDK-1892: Jira rejects (400 INVALID_INPUT) a text node that carries `code`
// together with strong/em/strike. Only `link` may accompany `code`. Every
// emitted text node must obey that, whatever the nesting.
func TestCodeMarkNeverCombinesWithTextStyleMarks(t *testing.T) {
	cases := []struct{ name, md string }{
		{"strong", "x **legacy `IMPORTED` 축 단독**이다."},
		{"em-strong", "*_x_ `code`*"},
		{"strike", "~~`code`~~"},
		{"strong-em-strike", "**_~~`c`~~_**"},
		{"in-link", "[**`c`** t](https://x.y)"},
		{"heading", "## **a `b` c**"},
		{"list", "- **a `b` c**"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := FromMarkdown(c.md)
			var sawCode bool
			for text, ms := range textMarks(t, raw) {
				hasCode := false
				for _, m := range ms {
					if m == "code" {
						hasCode = true
					}
				}
				if !hasCode {
					continue
				}
				sawCode = true
				for _, m := range ms {
					if m != "code" && m != "link" {
						t.Errorf("text %q carries marks %v: code may only combine with link\n%s", text, ms, raw)
					}
				}
			}
			if !sawCode {
				t.Fatalf("no code text node emitted\n%s", raw)
			}
		})
	}
}

// The text around the code span keeps its bold.
func TestCodeInsideBoldKeepsSurroundingBold(t *testing.T) {
	got := textMarks(t, FromMarkdown("x **legacy `IMPORTED` 축 단독**이다."))
	if ms := got["legacy "]; len(ms) != 1 || ms[0] != "strong" {
		t.Errorf("leading text marks = %v, want [strong]", ms)
	}
	if ms := got["IMPORTED"]; len(ms) != 1 || ms[0] != "code" {
		t.Errorf("code text marks = %v, want [code]", ms)
	}
	if ms := got[" 축 단독"]; len(ms) != 1 || ms[0] != "strong" {
		t.Errorf("trailing text marks = %v, want [strong]", ms)
	}
}

// GDK-1893: a heading that starts with a number must not store or show a
// backslash, and the shown text must come back unchanged on edit.
func TestNumberedHeadingHasNoBackslash(t *testing.T) {
	src := "## 1. 무슨 일이 있었나"
	raw := FromMarkdown(src)
	if strings.Contains(string(raw), `\\`) {
		t.Fatalf("stored ADF contains a backslash (converter keeps an escape)\n%s", raw)
	}
	if got := Markdown(raw); got != src {
		t.Fatalf("presenter shows %q, want %q", got, src)
	}
	if got := Source(raw); got != src {
		t.Fatalf("Source shows %q, want %q", got, src)
	}
}

// A paragraph that starts with `1. ` as text (not a list) is still escaped,
// and the round trip is stable: no backslash grows or disappears.
func TestNumberedParagraphEscapeRoundTrips(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","version":1,"content":[` +
		`{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"1. head"}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"1. not a list"}]}]}`)
	shown := Markdown(raw)
	want := "## 1. head\n\n1\\. not a list"
	if shown != want {
		t.Fatalf("shown = %q, want %q", shown, want)
	}
	back := FromMarkdown(shown)
	if got := Markdown(back); got != shown {
		t.Fatalf("second show = %q, want %q", got, shown)
	}
	if string(back) != string(raw) && !jsonEqual(back, raw) {
		t.Fatalf("round trip changed the ADF\n%s\nvs\n%s", back, raw)
	}
	b2, _, err := FromMarkdownWith(shown, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := Markdown(b2); got != shown {
		t.Fatalf("with-base show = %q, want %q", got, shown)
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ab, _ := json.Marshal(x)
	bb, _ := json.Marshal(y)
	return string(ab) == string(bb)
}
