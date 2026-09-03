package adf

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// richKept is a body with one of each placeholder class: a container (panel)
// holding editable markdown and a mention; an opaque block (mediaSingle); an
// inline mention, a status and a coloured run in running text; an expand
// holding a list; a table whose cell carries a mention inline and an image
// as a block beside its paragraph.
const richKept = `{"type":"doc","version":1,"content":[
 {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Repro"}]},
 {"type":"panel","attrs":{"panelType":"info"},"content":[
   {"type":"paragraph","content":[{"type":"text","text":"ask "},{"type":"mention","attrs":{"id":"acc-1","text":"@Dana"}},{"type":"text","text":" first, "},{"type":"text","text":"then","marks":[{"type":"strong"}]}]},
   {"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"step"}]}]}]}]},
 {"type":"paragraph","content":[{"type":"text","text":"cc "},{"type":"mention","attrs":{"id":"acc-2","text":"@Marco"}},{"type":"text","text":" — "},{"type":"status","attrs":{"text":"BLOCKED","color":"red"}},{"type":"text","text":" and "},{"type":"text","text":"red words","marks":[{"type":"textColor","attrs":{"color":"#ff0000"}}]}]},
 {"type":"mediaSingle","attrs":{"layout":"center"},"content":[{"type":"media","attrs":{"type":"file","id":"m-1","collection":"","alt":"shot.png"}}]},
 {"type":"expand","attrs":{"title":"Logs"},"content":[
   {"type":"orderedList","attrs":{"order":1},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]}]}]},
 {"type":"table","content":[
   {"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"who"}]}]},{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"what"}]}]}]},
   {"type":"tableRow","content":[
     {"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"acc-3","text":"@Lee"}}]}]},
     {"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"see"}]},{"type":"mediaSingle","attrs":{"layout":"center"},"content":[{"type":"media","attrs":{"type":"file","id":"m-2","collection":""}}]}]}]}]},
 {"type":"paragraph","content":[{"type":"text","text":"end"}]}
]}`

func TestSourceCarriesAPlaceholderPerPreservedNodeAndRoundTrips(t *testing.T) {
	base := json.RawMessage(richKept)
	src := Source(base)
	kept := Preserved(base)

	wantKinds := []string{"panel", "mention", "mention", "status", "textColor", "mediaSingle", "expand", "mention", "mediaSingle"}
	if len(kept) != len(wantKinds) {
		t.Fatalf("kept %d nodes, want %d:\n%s", len(kept), len(wantKinds), src)
	}
	for i, k := range kept {
		if k.Type != wantKinds[i] || k.N != i+1 {
			t.Fatalf("kept[%d] = %s #%d, want %s #%d", i, k.Type, k.N, wantKinds[i], i+1)
		}
		if !strings.Contains(src, "<!-- adf:"+itoa(k.N)+":"+k.Hash+" "+k.Type) {
			t.Fatalf("source lacks the marker for %s #%d:\n%s", k.Type, k.N, src)
		}
	}
	if !kept[0].Container || kept[0].Parent != "doc" || kept[1].Parent != "paragraph" || !kept[1].Inline {
		t.Fatalf("classes: %+v %+v", kept[0], kept[1])
	}
	if !strings.Contains(src, "<!-- /adf:1 -->") || !strings.Contains(src, "<!-- /adf:7 -->") {
		t.Fatalf("containers need close markers:\n%s", src)
	}
	// The panel's interior is ordinary markdown between its markers.
	if !strings.Contains(src, "first, **then**\n\n- step\n\n<!-- /adf:1 -->") {
		t.Fatalf("container interior is not markdown:\n%s", src)
	}
	// A block marker sits on its own line; an inline one in the text.
	if !strings.Contains(src, "\n<!-- adf:6:"+kept[5].Hash+" mediaSingle shot.png -->\n") {
		t.Fatalf("mediaSingle marker is not a line of its own:\n%s", src)
	}
	if !strings.Contains(src, "cc <!-- adf:3:"+kept[2].Hash+" mention @Marco --> — <!-- adf:4:") {
		t.Fatalf("inline markers are not inline:\n%s", src)
	}

	got, dropped, err := FromMarkdownWith(src, base)
	if err != nil {
		t.Fatalf("substitute: %v\n%s", err, src)
	}
	if len(dropped) != 0 {
		t.Fatalf("nothing was deleted, dropped = %+v", dropped)
	}
	if normalize(t, got) != normalize(t, base) {
		t.Fatalf("source → ADF is not the identity\n--- got ---\n%s\n--- want ---\n%s\n--- src ---\n%s", normalize(t, got), normalize(t, base), src)
	}
}

func TestContainerInteriorIsEditableAndAttrsSurvive(t *testing.T) {
	base := json.RawMessage(richKept)
	src := Source(base)
	edited := strings.Replace(src, "first, **then**", "first, _later_", 1)
	edited = strings.Replace(edited, "- step", "- step one\n- step two", 1)

	got, dropped, err := FromMarkdownWith(edited, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 0 {
		t.Fatalf("dropped %+v", dropped)
	}
	s := string(got)
	if !strings.Contains(s, `"panelType":"info"`) || !strings.Contains(s, `"text":"later"`) || !strings.Contains(s, `"type":"em"`) || !strings.Contains(s, `"text":"step two"`) {
		t.Fatalf("edited panel: %s", s)
	}
	if !strings.Contains(s, `"id":"acc-1"`) {
		t.Fatalf("the mention inside the panel must survive its interior edit: %s", s)
	}
}

func TestDeletingAPlaceholderDeletesTheNodeAndIsReported(t *testing.T) {
	base := json.RawMessage(richKept)
	kept := Preserved(base)
	src := Source(base)
	// Remove the mediaSingle line and the status inline.
	lines := strings.Split(src, "\n")
	var keep []string
	for _, l := range lines {
		if strings.HasPrefix(l, "<!-- adf:6:") {
			continue
		}
		keep = append(keep, l)
	}
	edited := strings.Join(keep, "\n")
	edited = strings.Replace(edited, " — <!-- adf:4:"+kept[3].Hash+" status BLOCKED -->", "", 1)

	got, dropped, err := FromMarkdownWith(edited, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 2 || dropped[0].Type != "status" || dropped[1].Type != "mediaSingle" {
		t.Fatalf("dropped = %+v", dropped)
	}
	if strings.Contains(string(got), `"type":"status"`) || strings.Contains(string(got), `"id":"m-1"`) {
		t.Fatalf("deleted nodes came back: %s", got)
	}
	if !HasPlaceholders(edited) || HasPlaceholders("plain **text**\n\n<!-- a comment -->") {
		t.Fatal("HasPlaceholders must see markers and only markers")
	}
}

func TestHasPlaceholdersIgnoresMarkersQuotedInCode(t *testing.T) {
	// GDK-1398: the documentation of the markers quotes them; a fence or a
	// code span is text, and a body made of it must stay editable.
	quoted := "The source holds `<!-- adf:1:deadbeef panel -->` per node.\n\n```\n<!-- adf:2:deadbeef mention @Dana -->\n<!-- /adf:2 -->\n```\n"
	if HasPlaceholders(quoted) {
		t.Fatalf("a marker inside code is not a placeholder:\n%s", quoted)
	}
	if err := RefusePlaceholders(quoted); err != nil {
		t.Fatalf("RefusePlaceholders shares the parse: %v", err)
	}
	if !HasPlaceholders("- item\n\n  <!-- adf:1:deadbeef mediaSingle -->\n") {
		t.Fatal("a real block marker under a list item is one")
	}
	if !HasPlaceholders("cc <!-- adf:3:deadbeef mention --> today") {
		t.Fatal("a real inline marker is one")
	}
}

func TestPlaceholderRefusals(t *testing.T) {
	base := json.RawMessage(richKept)
	kept := Preserved(base)
	src := Source(base)
	perr := func(t *testing.T, err error, n int, want string) {
		t.Helper()
		var pe *PlaceholderError
		if !errors.As(err, &pe) {
			t.Fatalf("want a PlaceholderError, got %v", err)
		}
		if pe.N != n || !strings.Contains(pe.Msg, want) {
			t.Fatalf("got adf:%d %q, want adf:%d containing %q", pe.N, pe.Msg, n, want)
		}
	}

	t.Run("no base", func(t *testing.T) {
		_, _, err := FromMarkdownWith(src, nil)
		perr(t, err, 1, "needs the body it came from")
	})
	t.Run("changed since read", func(t *testing.T) {
		changed := strings.Replace(richKept, `"panelType":"info"`, `"panelType":"warning"`, 1)
		_, _, err := FromMarkdownWith(src, json.RawMessage(changed))
		perr(t, err, 1, "changed since the body was read")
	})
	t.Run("unknown number", func(t *testing.T) {
		_, _, err := FromMarkdownWith(src+"\n\n<!-- adf:42:deadbeef panel -->", base)
		perr(t, err, 42, "has 9 preserved node(s)")
	})
	t.Run("used twice", func(t *testing.T) {
		m := "<!-- adf:3:" + kept[2].Hash + " mention @Marco -->"
		if !strings.Contains(src, m) {
			t.Fatalf("fixture drift: %s not in\n%s", m, src)
		}
		_, _, err := FromMarkdownWith(strings.Replace(src, m, m+" "+m, 1), base)
		perr(t, err, 3, "used twice")
	})
	t.Run("close without open", func(t *testing.T) {
		_, _, err := FromMarkdownWith("text\n\n<!-- /adf:1 -->", base)
		perr(t, err, 1, "without its open")
	})
	t.Run("open without close", func(t *testing.T) {
		_, _, err := FromMarkdownWith(strings.Replace(src, "<!-- /adf:7 -->", "", 1), base)
		perr(t, err, 7, "without its close")
	})
	t.Run("block marker in running text", func(t *testing.T) {
		m := "<!-- adf:6:" + kept[5].Hash + " mediaSingle shot.png -->"
		_, _, err := FromMarkdownWith("look "+m+" here", base)
		perr(t, err, 6, "stands on a line of its own")
	})
	t.Run("block under a parent it never had", func(t *testing.T) {
		m := "<!-- adf:6:" + kept[5].Hash + " mediaSingle shot.png -->"
		_, _, err := FromMarkdownWith("- item\n\n  "+m+"\n", base)
		perr(t, err, 6, "cannot sit under listItem")
	})
	t.Run("plain MarkdownDoc keeps a marker as the text it is", func(t *testing.T) {
		doc := FromMarkdown("<!-- adf:1:00000000 panel -->")
		if PlainText(doc) != "<!-- adf:1:00000000 panel -->" {
			t.Fatalf("no base, no substitution: %s", doc)
		}
	})
}

func TestInlineNodeAloneOnALineBecomesAParagraph(t *testing.T) {
	base := json.RawMessage(richKept)
	kept := Preserved(base)
	got, _, err := FromMarkdownWith("<!-- adf:3:"+kept[2].Hash+" mention -->\n\nafter", base)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"content":[{"content":[{"attrs":{"id":"acc-2","text":"@Marco"},"type":"mention"}],"type":"paragraph"},{"content":[{"text":"after","type":"text"}],"type":"paragraph"}],"type":"doc","version":1}`
	if normalize(t, got) != want {
		t.Fatalf("got %s", normalize(t, got))
	}
}

func TestSimpleAndSubsetBodiesHaveNoPlaceholders(t *testing.T) {
	if k := Preserved(FromMarkdown(canonical)); len(k) != 0 {
		t.Fatalf("subset body kept %+v", k)
	}
	if s := Source(FromMarkdown(canonical)); s != canonical {
		t.Fatalf("Source on the subset must be Markdown:\n%s", s)
	}
	simple := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"just **typed** text"}]}]}`)
	if Source(simple) != "just **typed** text" {
		t.Fatalf("Source on a simple doc must be the typed text: %q", Source(simple))
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
