package adf

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Placeholders (docs/decisions/0012, addendum 1; GDK-1396).
//
// A body can hold nodes markdown has no syntax for — a panel, an inline
// image, a mention, a status lozenge, a coloured run. Before this file the
// only ways to edit such a body from markdown were to refuse or to drop
// them. Now Source() serializes each such node as an HTML-comment
// placeholder the markdown reader sees as a comment and the person sees as
// a labelled marker, and MarkdownDocWith() puts the original node back where
// the placeholder stands, taking it from the body currently on the origin.
// Nothing is stored: the markers exist only in the text an editor opens.
//
//	<!-- adf:3:9f1c2a7b mention @Dana -->              an opaque node, inline or on its own line
//	<!-- adf:1:0be44d10 panel info -->                 a container: its markdown follows…
//	…
//	<!-- /adf:1 -->                                    …until its close
//
// `adf:N` is the node's position in document order among preserved nodes;
// the hash pins its content, so a placeholder from a body that has changed
// since it was read is refused rather than rebuilt over someone else's edit.
// The words after the hash are a hint for the reader and are ignored.
//
// Three classes. Opaque leaves (mention, emoji, status, date, inlineCard,
// media, extension, …) come back exactly as they were. Containers (panel,
// expand, nestedExpand) keep their attrs and take the markdown between their
// two markers as new content. A text run carrying a mark markdown lacks
// (textColor, underline, subsup, …) is opaque as a whole — editable text
// with a preserved mark is a later step.
//
// Deleting a placeholder deletes the node; the caller decides what to do
// when every placeholder is gone (edit -m keeps refusing that, as the plain
// replace it is).

// Kept is one preserved node of a document.
type Kept struct {
	// N is the 1-based position among the document's preserved nodes, in
	// document order — the number the placeholder carries.
	N int
	// Hash pins the node's JSON; a placeholder whose hash differs from the
	// current body's is refused.
	Hash string
	// Type is the ADF node type (panel, mention, …); for a text run kept for
	// its mark it is the mark's type.
	Type string
	// Parent is the type of the node it sat under. A block placeholder may
	// stand under the document root or under a node of this type, nowhere
	// else — Jira rejects a panel inside a list item, and this is the check
	// that says so before the origin does.
	Parent string
	// Container: the node's content is editable markdown between an open
	// and a close marker.
	Container bool
	// Inline: the node sat in inline content (a paragraph, a heading, a cell
	// paragraph) and its placeholder is inline.
	Inline bool
	// Node is the node itself.
	Node node
}

// containerTypes are the preserved block nodes whose content stays editable.
var containerTypes = map[string]bool{"panel": true, "expand": true, "nestedExpand": true}

// Preserved lists the nodes of raw that markdown cannot carry, numbered the
// way Source() numbers their placeholders — the same writer pass produces
// both, so the two cannot disagree.
func Preserved(raw json.RawMessage) []Kept {
	_, kept := sourceWith(raw)
	return kept
}

// Source is the markdown an editor opens with: Markdown() for a body that
// round-trips, and Markdown() with a placeholder standing in for every node
// it cannot carry otherwise. Present.Source is this.
func Source(raw json.RawMessage) string {
	s, _ := sourceWith(raw)
	return s
}

func sourceWith(raw json.RawMessage) (string, []Kept) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var doc node
	if json.Unmarshal(raw, &doc) != nil {
		return PlainText(raw), nil
	}
	if IsSimple(string(raw)) {
		return Markdown(raw), nil
	}
	w := &writer{escape: true, placeholders: true, cur: "doc"}
	out := strings.TrimRight(w.blocks(doc.Content, ""), "\n")
	return out, w.kept
}

// keep records a preserved node and returns its placeholder text (the open
// marker for a container).
func (w *writer) keep(n node, parent string, inline bool) string {
	kind := n.Type
	if n.Type == "text" {
		for _, m := range n.Marks {
			if !markdownMarks[m.Type] {
				kind = m.Type
				break
			}
		}
	}
	k := Kept{
		N: len(w.kept) + 1, Hash: nodeHash(n), Type: kind, Parent: parent,
		Container: containerTypes[n.Type], Inline: inline, Node: n,
	}
	w.kept = append(w.kept, k)
	return openMarker(k)
}

func openMarker(k Kept) string {
	s := "<!-- adf:" + strconv.Itoa(k.N) + ":" + k.Hash + " " + k.Type
	if h := hint(k.Node); h != "" {
		s += " " + h
	}
	return s + " -->"
}

func closeMarker(n int) string { return "<!-- /adf:" + strconv.Itoa(n) + " -->" }

// hint is the few words after the hash that tell a reader what stands here.
// Ignored on the way back in.
func hint(n node) string {
	var s string
	switch n.Type {
	case "text":
		s = n.Text
	case "panel":
		s, _ = n.Attrs["panelType"].(string)
	case "expand", "nestedExpand":
		s, _ = n.Attrs["title"].(string)
	case "mediaSingle", "mediaGroup", "mediaInline", "media":
		s = mediaHint(n)
	default:
		s = inlineLabel(n)
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "--", "‐‐")
	if rs := []rune(s); len(rs) > 40 {
		s = string(rs[:40]) + "…"
	}
	return s
}

func mediaHint(n node) string {
	if alt, _ := n.Attrs["alt"].(string); alt != "" {
		return alt
	}
	for _, c := range n.Content {
		if h := mediaHint(c); h != "" {
			return h
		}
	}
	return ""
}

// nodeHash is FNV-1a over the node's JSON — encoding/json sorts map keys, so
// the same node hashes the same whatever order the origin sent its fields in.
func nodeHash(n node) string {
	b, _ := json.Marshal(n)
	h := fnv.New32a()
	_, _ = h.Write(b)
	return fmt.Sprintf("%08x", h.Sum32())
}

// preservedInline reports whether an inline node has to be kept whole: a
// type outside the subset, or a text run with a mark outside it.
func preservedInline(n node) bool {
	if n.Type == "text" {
		for _, m := range n.Marks {
			if !markdownMarks[m.Type] {
				return true
			}
		}
		return false
	}
	return n.Type != "hardBreak"
}

// preservedBlock reports whether a block node has to be kept whole: a type
// outside the subset, or a subset block carrying a mark (alignment,
// indentation) markdown cannot express.
func preservedBlock(n node) bool {
	if !markdownTypes[n.Type] {
		return true
	}
	for _, m := range n.Marks {
		if !markdownMarks[m.Type] {
			return true
		}
	}
	return false
}

// ── markdown with placeholders → ADF ─────────────────────────────────────

var (
	openRe  = regexp.MustCompile(`^<!--\s*adf:(\d+):([0-9a-f]{8})(?:\s[^>]*?)?\s*-->$`)
	closeRe = regexp.MustCompile(`^<!--\s*/adf:(\d+)\s*-->$`)
)

// HasPlaceholders reports whether src carries at least one placeholder — the
// gate's question when a body has preserved nodes: none in the draft means a
// plain replace, which stays refused without force.
//
// It reads the markdown the way the substituting parser does — a marker is
// an HTML block or a raw-HTML run — so a marker quoted inside a code fence
// or a code span is the text it is, not a placeholder. A line regex called
// this documentation of the markers itself unpublishable (GDK-1398).
func HasPlaceholders(src string) bool {
	source := []byte(strings.ReplaceAll(src, "\r\n", "\n"))
	root := md.Parser().Parse(text.NewReader(source))
	c := &conv{src: source}
	found := false
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var raw string
		switch v := n.(type) {
		case *ast.HTMLBlock:
			raw = c.htmlBlockText(v)
		case *ast.RawHTML:
			var b strings.Builder
			for i := 0; i < v.Segments.Len(); i++ {
				seg := v.Segments.At(i)
				b.Write(seg.Value(source))
			}
			raw = b.String()
		default:
			return ast.WalkContinue, nil
		}
		if inlineOpenRe.MatchString(raw) {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// RefusePlaceholders is the check every text→ADF entry point without a body
// to substitute from makes (create, comment, page create/comment, --append):
// a marker there has nothing behind it and would be stored as visible text.
func RefusePlaceholders(text string) error {
	if !HasPlaceholders(text) {
		return nil
	}
	return &PlaceholderError{Msg: "the text carries placeholders, which stand for nodes of the body they were read from, and this write has no such body — drop the markers, or send that body as ADF with --adf-file"}
}

var inlineOpenRe = regexp.MustCompile(`<!--\s*adf:\d+:[0-9a-f]{8}(?:\s[^>]*?)?\s*-->`)

// PlaceholderError is a placeholder the draft carries that the current body
// cannot honour. Callers print Error(); the fields are for tests and for a
// UI that wants to point at the marker.
type PlaceholderError struct {
	N   int
	Msg string
}

func (e *PlaceholderError) Error() string {
	if e.N > 0 {
		return fmt.Sprintf("placeholder adf:%d: %s", e.N, e.Msg)
	}
	return e.Msg
}

// MarkdownDocWith is MarkdownDoc with the body currently on the origin as
// the source of every placeholder's node. It returns the document, the
// preserved nodes of base that no placeholder referenced (deleted by the
// edit), and an error when a placeholder cannot be honoured: a number base
// does not have, a hash that no longer matches (the body changed since it
// was read), a marker used twice, a close without its open, a block node in
// inline position, or a block node under a parent it did not sit under. A
// nil base with any placeholder in src is an error too — a marker with
// nothing behind it is never text.
func MarkdownDocWith(src string, base json.RawMessage) (map[string]any, []Kept, error) {
	var kept []Kept
	if len(base) > 0 {
		kept = Preserved(base)
	}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	source := []byte(src)
	root := md.Parser().Parse(textReader(source))
	c := &conv{src: source, base: kept, used: map[int]bool{}, substitute: true}
	content, err := c.blocksIn(root, "doc")
	if err != nil {
		return nil, nil, err
	}
	if content == nil {
		content = []any{}
	}
	var dropped []Kept
	for _, k := range kept {
		if !c.used[k.N] && !c.insideUsedContainer(k) {
			dropped = append(dropped, k)
		}
	}
	return map[string]any{"type": "doc", "version": 1, "content": content}, dropped, nil
}

// FromMarkdownWith is MarkdownDocWith marshalled.
func FromMarkdownWith(src string, base json.RawMessage) (json.RawMessage, []Kept, error) {
	doc, dropped, err := MarkdownDocWith(src, base)
	if err != nil {
		return nil, nil, err
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, err
	}
	return b, dropped, nil
}

// insideUsedContainer: a preserved node nested in a container counts as
// referenced when its container was — the container's markdown may or may
// not still carry it, and the marker for it (if kept) was consumed on its
// own. Only a node whose container was itself dropped, or which had no
// container, is reported as dropped when unreferenced.
func (c *conv) insideUsedContainer(k Kept) bool {
	for _, p := range c.base {
		if p.Container && p.N < k.N && c.used[p.N] && containsNode(p.Node, k.Node) {
			return true
		}
	}
	return false
}

func containsNode(parent, want node) bool {
	for _, ch := range parent.Content {
		if ch.Type == want.Type && nodeHash(ch) == nodeHash(want) {
			return true
		}
		if containsNode(ch, want) {
			return true
		}
	}
	return false
}

// lookup resolves a marker against the base.
func (c *conv) lookup(n int, hash string) (Kept, error) {
	if c.base == nil {
		return Kept{}, &PlaceholderError{N: n, Msg: "a placeholder needs the body it came from, and this write has none — drop the marker or edit the body it belongs to"}
	}
	if n < 1 || n > len(c.base) {
		return Kept{}, &PlaceholderError{N: n, Msg: fmt.Sprintf("the current body has %d preserved node(s); re-read it and edit from that", len(c.base))}
	}
	k := c.base[n-1]
	if k.Hash != hash {
		return Kept{}, &PlaceholderError{N: n, Msg: fmt.Sprintf("the %s here changed since the body was read — re-read it and edit from that", k.Type)}
	}
	if c.used[n] {
		return Kept{}, &PlaceholderError{N: n, Msg: "used twice — a preserved node stands in one place"}
	}
	c.used[n] = true
	return k, nil
}

// keptMap is the node as the doc tree holds it: the typed node marshalled
// (omitempty, so an absent attrs stays absent) and read back generic.
func keptMap(k Kept) map[string]any {
	b, _ := json.Marshal(k.Node)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func placeholderLine(s string) (n int, hash string, closing, ok bool) {
	s = strings.TrimSpace(s)
	if m := closeRe.FindStringSubmatch(s); m != nil {
		n, _ = strconv.Atoi(m[1])
		return n, "", true, true
	}
	if m := openRe.FindStringSubmatch(s); m != nil {
		n, _ = strconv.Atoi(m[1])
		return n, m[2], false, true
	}
	return 0, "", false, false
}
