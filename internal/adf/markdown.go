package adf

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Markdown is the editing source of a body, and ADF is the wire
// (docs/decisions/0012, GDK-1383). FromMarkdown builds the ADF every write
// path sends; Markdown recovers the text a person edits from the ADF an
// origin holds. The two are inverses on the subset below, and the golden
// tests in markdown_test.go pin that both ways — a silent drift there is a
// body that changes on every re-edit.
//
// Subset: paragraph, heading 1–6, hardBreak, rule, bulletList / orderedList
// (nested), listItem, blockquote, codeBlock (language), table with
// single-paragraph cells; marks strong, em, code, strike, link. Line
// semantics: a blank line separates paragraphs, a single newline is a
// hardBreak — what an agent writing markdown into `-m` means, and what
// turns a migrated one-text-node body (GDK-1382) back into its blocks.
// Raw HTML is text. Anything else an ADF carries (panel, media, status,
// mention, textColor, …) has no markdown and FormatLoss names it.

var md = goldmark.New(goldmark.WithExtensions(extension.Strikethrough, extension.Table))

// FromMarkdown parses markdown text into an ADF document.
func FromMarkdown(src string) json.RawMessage {
	b, err := json.Marshal(MarkdownDoc(src))
	if err != nil {
		return json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	}
	return b
}

// MarkdownDoc is FromMarkdown before marshalling: the doc as a tree of
// map[string]any, for a caller that adds nodes (mentions, media) before
// sending it.
func MarkdownDoc(src string) map[string]any {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	source := []byte(src)
	root := md.Parser().Parse(text.NewReader(source))
	c := &conv{src: source}
	content := c.blocks(root)
	if content == nil {
		content = []any{}
	}
	return map[string]any{"type": "doc", "version": 1, "content": content}
}

type conv struct {
	src []byte
	// Placeholder substitution (preserve.go). substitute is on for
	// MarkdownDocWith; base is the current body's preserved nodes, used the
	// markers consumed so far, parent the type of the block being filled,
	// hoisted the block nodes a table cell's inline text referenced (a cell
	// is one paragraph in GFM, so a block marker there is lifted beside it).
	// err is the first placeholder error; the walk stops adding after it.
	substitute bool
	base       []Kept
	used       map[int]bool
	parent     string
	inCell     bool
	hoisted    []any
	err        error
}

func (c *conv) fail(err error) {
	if c.err == nil {
		c.err = err
	}
}

func (c *conv) blocks(parent ast.Node) []any {
	out, _ := c.blocksIn(parent, c.parent)
	return out
}

// blocksIn converts parent's children as blocks under parentType. With
// substitution on, a placeholder line becomes its kept node, and a
// container's open marker swallows the siblings up to its close marker as
// the container's new content.
func (c *conv) blocksIn(parent ast.Node, parentType string) ([]any, error) {
	saved := c.parent
	c.parent = parentType
	defer func() { c.parent = saved }()
	var out []any
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if c.err != nil {
			break
		}
		if hb, ok := n.(*ast.HTMLBlock); ok && c.substitute {
			if num, hash, closing, isMarker := placeholderLine(c.htmlBlockText(hb)); isMarker {
				if closing {
					c.fail(&PlaceholderError{N: num, Msg: "close marker without its open marker"})
					break
				}
				k, err := c.lookup(num, hash)
				if err != nil {
					c.fail(err)
					break
				}
				block, next, err := c.keptBlock(k, n)
				if err != nil {
					c.fail(err)
					break
				}
				out = append(out, block)
				n = next
				continue
			}
		}
		if b := c.block(n); b != nil {
			out = append(out, b...)
		}
	}
	return out, c.err
}

// keptBlock builds the node a block-position marker stands for. For a
// container it converts the siblings after the marker up to the matching
// close marker as the node's content and returns the close marker as the
// node to continue after; for anything else it returns the marker itself.
func (c *conv) keptBlock(k Kept, marker ast.Node) (map[string]any, ast.Node, error) {
	node := keptMap(k)
	if k.Inline {
		// An inline node alone on its line: a paragraph of it.
		return map[string]any{"type": "paragraph", "content": []any{node}}, marker, nil
	}
	if c.parent != "doc" && c.parent != k.Parent {
		return nil, nil, &PlaceholderError{N: k.N, Msg: fmt.Sprintf("a %s sat under %s and cannot sit under %s", k.Type, k.Parent, c.parent)}
	}
	if !k.Container {
		return node, marker, nil
	}
	saved := c.parent
	c.parent = k.Type
	defer func() { c.parent = saved }()
	var content []any
	for n := marker.NextSibling(); n != nil; n = n.NextSibling() {
		if hb, ok := n.(*ast.HTMLBlock); ok {
			if num, hash, closing, isMarker := placeholderLine(c.htmlBlockText(hb)); isMarker {
				if closing {
					if num != k.N {
						return nil, nil, &PlaceholderError{N: num, Msg: fmt.Sprintf("close marker inside adf:%d, which is still open", k.N)}
					}
					if len(content) == 0 {
						content = []any{map[string]any{"type": "paragraph"}}
					}
					node["content"] = content
					return node, n, nil
				}
				inner, err := c.lookup(num, hash)
				if err != nil {
					return nil, nil, err
				}
				block, next, err := c.keptBlock(inner, n)
				if err != nil {
					return nil, nil, err
				}
				content = append(content, block)
				n = next
				continue
			}
		}
		if b := c.block(n); b != nil {
			content = append(content, b...)
		}
		if c.err != nil {
			return nil, nil, c.err
		}
	}
	return nil, nil, &PlaceholderError{N: k.N, Msg: fmt.Sprintf("open marker without its close marker (%s)", closeMarker(k.N))}
}

func (c *conv) htmlBlockText(v *ast.HTMLBlock) string {
	var b strings.Builder
	for i := 0; i < v.Lines().Len(); i++ {
		seg := v.Lines().At(i)
		b.Write(seg.Value(c.src))
	}
	if v.HasClosure() {
		b.Write(v.ClosureLine.Value(c.src))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (c *conv) block(n ast.Node) []any {
	switch v := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return []any{withContent(map[string]any{"type": "paragraph"}, c.inlines(n, nil))}
	case *ast.Heading:
		return []any{withContent(map[string]any{"type": "heading", "attrs": map[string]any{"level": v.Level}}, c.inlines(n, nil))}
	case *ast.ThematicBreak:
		return []any{map[string]any{"type": "rule"}}
	case *ast.FencedCodeBlock:
		node := map[string]any{"type": "codeBlock"}
		if lang := string(v.Language(c.src)); lang != "" {
			node["attrs"] = map[string]any{"language": lang}
		}
		return []any{withContent(node, c.codeText(v))}
	case *ast.CodeBlock:
		return []any{withContent(map[string]any{"type": "codeBlock"}, c.codeText(v))}
	case *ast.Blockquote:
		inner, _ := c.blocksIn(n, "blockquote")
		return []any{withContent(map[string]any{"type": "blockquote"}, inner)}
	case *ast.List:
		node := map[string]any{"type": "bulletList"}
		if v.IsOrdered() {
			node = map[string]any{"type": "orderedList", "attrs": map[string]any{"order": v.Start}}
		}
		return []any{withContent(node, c.blocks(n))}
	case *ast.ListItem:
		inner, _ := c.blocksIn(n, "listItem")
		return []any{withContent(map[string]any{"type": "listItem"}, inner)}
	case *ast.HTMLBlock:
		// Raw HTML is text: nothing this package emits is rendered as HTML.
		// (A placeholder line was taken by blocksIn before reaching here.)
		return []any{withContent(map[string]any{"type": "paragraph"}, textNode(c.htmlBlockText(v), nil))}
	case *extast.Table:
		return []any{withContent(map[string]any{"type": "table"}, c.blocks(n))}
	case *extast.TableHeader:
		return []any{withContent(map[string]any{"type": "tableRow"}, c.cells(n, "tableHeader"))}
	case *extast.TableRow:
		return []any{withContent(map[string]any{"type": "tableRow"}, c.cells(n, "tableCell"))}
	}
	return nil
}

func (c *conv) cells(row ast.Node, kind string) []any {
	var out []any
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		c.inCell, c.hoisted = true, nil
		saved := c.parent
		c.parent = kind
		inl := c.inlines(cell, nil)
		if len(c.hoisted) > 0 {
			// ponytail: a hoisted block lands after the cell's paragraph
			// whatever its original position; a cell with a block before its
			// text comes back reordered. Positional markers if it matters.
			// The writer joined the cell's paragraph and its block markers
			// with spaces; the block is a sibling again, so the spaces go.
			inl = trimEdgeSpace(inl)
		}
		para := withContent(map[string]any{"type": "paragraph"}, inl)
		content := append([]any{para}, c.hoisted...)
		c.parent = saved
		c.inCell, c.hoisted = false, nil
		out = append(out, map[string]any{"type": kind, "content": content})
	}
	return out
}

func (c *conv) codeText(n ast.Node) []any {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(c.src))
	}
	s := strings.TrimSuffix(b.String(), "\n")
	if s == "" {
		return nil
	}
	return []any{map[string]any{"type": "text", "text": s}}
}

// inlines flattens an inline subtree into ADF text/hardBreak nodes, marks
// accumulating down the tree.
func (c *conv) inlines(parent ast.Node, marks []any) []any {
	var out []any
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch v := n.(type) {
		case *ast.Text:
			seg := string(v.Segment.Value(c.src))
			if !v.IsRaw() {
				seg = resolveEscapes(seg)
			}
			out = append(out, textNode(seg, marks)...)
			if v.HardLineBreak() || v.SoftLineBreak() {
				out = append(out, map[string]any{"type": "hardBreak"})
			}
		case *ast.String:
			out = append(out, textNode(string(v.Value), marks)...)
		case *ast.CodeSpan:
			var b strings.Builder
			for t := v.FirstChild(); t != nil; t = t.NextSibling() {
				if tt, ok := t.(*ast.Text); ok {
					b.Write(tt.Segment.Value(c.src))
				}
			}
			out = append(out, textNode(b.String(), append(cloneMarks(marks), mark("code")))...)
		case *ast.Emphasis:
			m := "em"
			if v.Level >= 2 {
				m = "strong"
			}
			out = append(out, c.inlines(n, append(cloneMarks(marks), mark(m)))...)
		case *extast.Strikethrough:
			out = append(out, c.inlines(n, append(cloneMarks(marks), mark("strike")))...)
		case *ast.Link:
			attrs := map[string]any{"href": string(v.Destination)}
			if len(v.Title) > 0 {
				attrs["title"] = string(v.Title)
			}
			out = append(out, c.inlines(n, append(cloneMarks(marks), map[string]any{"type": "link", "attrs": attrs}))...)
		case *ast.AutoLink:
			url := string(v.URL(c.src))
			label := string(v.Label(c.src))
			href := url
			if v.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(href), "mailto:") {
				href = "mailto:" + href
			}
			out = append(out, textNode(label, append(cloneMarks(marks), map[string]any{"type": "link", "attrs": map[string]any{"href": href}}))...)
		case *ast.Image:
			// ADF has no image-by-URL; keep the author's markdown as text so
			// it survives a round trip untouched.
			var alt strings.Builder
			for t := v.FirstChild(); t != nil; t = t.NextSibling() {
				if tt, ok := t.(*ast.Text); ok {
					alt.Write(tt.Segment.Value(c.src))
				}
			}
			out = append(out, textNode("!["+alt.String()+"]("+string(v.Destination)+")", marks)...)
		case *ast.RawHTML:
			var b strings.Builder
			for i := 0; i < v.Segments.Len(); i++ {
				seg := v.Segments.At(i)
				b.Write(seg.Value(c.src))
			}
			if c.substitute {
				if num, hash, closing, isMarker := placeholderLine(b.String()); isMarker {
					if closing {
						c.fail(&PlaceholderError{N: num, Msg: "close marker in running text — a container's markers stand on their own lines"})
						continue
					}
					k, err := c.lookup(num, hash)
					if err != nil {
						c.fail(err)
						continue
					}
					if !k.Inline {
						if c.inCell {
							c.hoisted = append(c.hoisted, keptMap(k))
							continue
						}
						c.fail(&PlaceholderError{N: k.N, Msg: fmt.Sprintf("a %s is a block — its marker stands on a line of its own", k.Type)})
						continue
					}
					out = append(out, keptMap(k))
					continue
				}
			}
			out = append(out, textNode(b.String(), marks)...)
		default:
			out = append(out, c.inlines(n, marks)...)
		}
	}
	return mergeText(out)
}

// resolveEscapes does for a text segment what goldmark's HTML writer would:
// a backslash before ASCII punctuation is dropped, an entity reference is
// decoded. The parser leaves both in the segment.
func resolveEscapes(s string) string {
	if !strings.ContainsAny(s, "\\&") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && i+1 < len(s) && util.IsPunct(s[i+1]) {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if ch == '&' {
			if end := strings.IndexByte(s[i:], ';'); end > 1 && end <= 32 {
				ent := s[i : i+end+1]
				if dec := html.UnescapeString(ent); dec != ent {
					b.WriteString(dec)
					i += end
					continue
				}
			}
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func mark(kind string) map[string]any { return map[string]any{"type": kind} }

func cloneMarks(m []any) []any { return append([]any(nil), m...) }

func textNode(s string, marks []any) []any {
	if s == "" {
		return nil
	}
	n := map[string]any{"type": "text", "text": s}
	if len(marks) > 0 {
		n["marks"] = marks
	}
	return []any{n}
}

func withContent(node map[string]any, content []any) map[string]any {
	if len(content) > 0 {
		node["content"] = content
	}
	return node
}

// mergeText joins adjacent text nodes with identical marks — goldmark splits
// text at every delimiter it considered, and one node per word is not a
// document anyone wants to diff.
func mergeText(nodes []any) []any {
	var out []any
	for _, n := range nodes {
		m, _ := n.(map[string]any)
		if len(out) > 0 && m != nil && m["type"] == "text" {
			if prev, _ := out[len(out)-1].(map[string]any); prev != nil && prev["type"] == "text" && sameMarks(prev["marks"], m["marks"]) {
				prev["text"] = prev["text"].(string) + m["text"].(string)
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

func sameMarks(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// ── ADF → markdown ────────────────────────────────────────────────────────

// Markdown recovers the editing source of an ADF document. A simple document
// (IsSimple: paragraphs, text, hardBreak, codeBlock, no marks) is what an
// older jira.Doc or a migration made of text a person typed — that text is
// returned as it was typed, markdown syntax included, so what looked like a
// wall of `**` and `##` edits as the markdown it always was. Anything richer
// is serialized with escaping, so a literal `*` in a Jira-authored body stays
// literal when it comes back through FromMarkdown. A bare string body is
// returned as-is.
func Markdown(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var doc node
	if json.Unmarshal(raw, &doc) != nil {
		return PlainText(raw)
	}
	simple := IsSimple(string(raw))
	w := &writer{escape: !simple, lines: simple && hasEmptyParagraph(doc.Content)}
	return strings.TrimRight(w.blocks(doc.Content, ""), "\n")
}

func hasEmptyParagraph(ns []node) bool {
	for _, n := range ns {
		if n.Type == "paragraph" && len(n.Content) == 0 {
			return true
		}
	}
	return false
}

// node is the typed walk shape. omitempty on every field but type: a kept
// node (preserve.go) is re-marshalled from this and must come out as the
// origin sent it — no "text":"" on a panel, no "attrs":null on a paragraph.
type node struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Marks   []node         `json:"marks,omitempty"`
	Content []node         `json:"content,omitempty"`
}

func textReader(b []byte) text.Reader { return text.NewReader(b) }

type writer struct {
	escape bool
	inItem int // >0 while rendering a listItem's blocks
	// placeholders: a node markdown cannot carry becomes a marker
	// (preserve.go) instead of its flattened label; kept collects them in
	// the order the markers are numbered. cur is the type of the block whose
	// content is being rendered — a kept node's Parent.
	placeholders bool
	kept         []Kept
	cur          string
	// lines: the document is an older jira.Doc — one paragraph per typed
	// line, an empty paragraph where the line was blank — so blocks are
	// separated by a newline, not a blank line, and the typed text comes
	// back exactly.
	lines bool
}

// blocks renders block nodes separated by blank lines, every line of the
// result prefixed (blockquote and list continuation are prefixes).
func (w *writer) blocks(ns []node, prefix string) string {
	var b strings.Builder
	var prev string
	for i, n := range ns {
		s := w.block(n, prefix, i)
		if s == "" && n.Type != "paragraph" {
			continue
		}
		if prev != "" {
			// A list nested under an item's paragraph stays tight: no blank
			// line, which is also the only way md→ADF→md is the identity for
			// "- b\n  - b2".
			// An ordered list may interrupt a paragraph only when it starts at
			// 1 (CommonMark), so any other start keeps the blank line.
			tight := w.inItem > 0 && prev == "paragraph" &&
				(n.Type == "bulletList" || (n.Type == "orderedList" && attrInt(n.Attrs, "order", 1) == 1))
			if !tight && !w.lines {
				b.WriteString(strings.TrimRight(prefix, " ") + "\n")
			}
		}
		b.WriteString(s)
		if !strings.HasSuffix(s, "\n") {
			b.WriteString("\n")
		}
		prev = n.Type
	}
	return b.String()
}

// blocksUnder is blocks with the parent type recorded for kept nodes.
func (w *writer) blocksUnder(parent string, ns []node, prefix string) string {
	saved := w.cur
	w.cur = parent
	defer func() { w.cur = saved }()
	return w.blocks(ns, prefix)
}

func (w *writer) block(n node, prefix string, index int) string {
	if w.placeholders && preservedBlock(n) {
		return w.keepBlock(n, prefix)
	}
	switch n.Type {
	case "paragraph":
		return prefixLines(w.inlinesUnder("paragraph", n.Content), prefix)
	case "heading":
		level := attrInt(n.Attrs, "level", 1)
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		return prefixLines(strings.Repeat("#", level)+" "+strings.ReplaceAll(w.inlinesUnder("heading", n.Content), "\n", " "), prefix)
	case "rule":
		return prefix + "---"
	case "codeBlock":
		body := ""
		for _, c := range n.Content {
			body += c.Text
		}
		fence := "```"
		for strings.Contains(body, fence) {
			fence += "`"
		}
		lang, _ := n.Attrs["language"].(string)
		return prefixLines(fence+lang+"\n"+body+"\n"+fence, prefix)
	case "blockquote":
		return w.blocksUnder("blockquote", n.Content, prefix+"> ")
	case "bulletList":
		var items []string
		for _, li := range n.Content {
			items = append(items, w.listItem(li, "- ", prefix))
		}
		return strings.Join(items, "")
	case "orderedList":
		start := attrInt(n.Attrs, "order", 1)
		var items []string
		for i, li := range n.Content {
			marker := strconv.Itoa(start+i) + ". "
			items = append(items, w.listItem(li, marker, prefix))
		}
		return strings.Join(items, "")
	case "table":
		return w.table(n, prefix)
	case "mediaSingle", "mediaGroup", "media", "mediaInline":
		return prefix + w.inlines([]node{n})
	default:
		// Unknown block (panel, expand, extension, …): its blocks, flattened.
		// (Only without placeholders — with them, preservedBlock caught it.)
		if len(n.Content) > 0 && isBlock(n.Content[0].Type) {
			return w.blocksUnder(n.Type, n.Content, prefix)
		}
		return prefixLines(w.inlinesUnder(n.Type, n.Content), prefix)
	}
}

// keepBlock renders a preserved block: a marker line for an opaque node, an
// open marker, its content as markdown and a close marker for a container.
func (w *writer) keepBlock(n node, prefix string) string {
	open := w.keep(n, w.cur, false)
	k := w.kept[len(w.kept)-1]
	if !k.Container {
		return prefix + open
	}
	inner := strings.TrimRight(w.blocksUnder(n.Type, n.Content, prefix), "\n")
	sep := strings.TrimRight(prefix, " ")
	if inner == "" {
		return prefix + open + "\n" + sep + "\n" + prefix + closeMarker(k.N)
	}
	return prefix + open + "\n" + sep + "\n" + inner + "\n" + sep + "\n" + prefix + closeMarker(k.N)
}

func isBlock(t string) bool {
	switch t {
	case "paragraph", "heading", "rule", "codeBlock", "blockquote", "bulletList", "orderedList", "table", "panel", "expand", "mediaSingle", "mediaGroup":
		return true
	}
	return false
}

// listItem renders one item: the marker on its first line, continuation
// lines and nested blocks indented by the marker's width.
func (w *writer) listItem(li node, marker, prefix string) string {
	indent := strings.Repeat(" ", len(marker))
	w.inItem++
	body := w.blocksUnder("listItem", li.Content, prefix+indent)
	w.inItem--
	if body == "" {
		return prefix + marker + "\n"
	}
	// Replace the first line's indent with the marker.
	first := prefix + indent
	if strings.HasPrefix(body, first) {
		body = prefix + marker + body[len(first):]
	}
	return body
}

func (w *writer) table(n node, prefix string) string {
	var rows [][]string
	header := false
	for ri, row := range n.Content {
		var cells []string
		for _, cell := range row.Content {
			if ri == 0 && cell.Type == "tableHeader" {
				header = true
			}
			var parts []string
			saved := w.cur
			w.cur = cell.Type
			for _, c := range cell.Content {
				parts = append(parts, strings.ReplaceAll(w.block(c, "", 0), "\n", " "))
			}
			w.cur = saved
			cells = append(cells, strings.ReplaceAll(strings.Join(parts, " "), "|", "\\|"))
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	line := func(cells []string) string {
		for len(cells) < width {
			cells = append(cells, "")
		}
		return prefix + "| " + strings.Join(cells, " | ") + " |"
	}
	var b strings.Builder
	if !header {
		// GFM needs a header row; an ADF table without one gets an empty one.
		b.WriteString(line(make([]string, width)) + "\n")
	} else {
		b.WriteString(line(rows[0]) + "\n")
		rows = rows[1:]
	}
	seps := make([]string, width)
	for i := range seps {
		seps[i] = "---"
	}
	b.WriteString(line(seps) + "\n")
	for _, r := range rows {
		b.WriteString(line(r) + "\n")
	}
	return b.String()
}

func prefixLines(s, prefix string) string {
	if prefix == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// inlines renders inline nodes: marks become delimiters, hardBreak a newline,
// a mention its @label, everything else its visible text.
// inlinesUnder is inlines with the parent type recorded for kept nodes.
func (w *writer) inlinesUnder(parent string, ns []node) string {
	saved := w.cur
	w.cur = parent
	defer func() { w.cur = saved }()
	return w.inlines(ns)
}

func (w *writer) inlines(ns []node) string {
	var b strings.Builder
	for _, n := range ns {
		if w.placeholders && preservedInline(n) {
			b.WriteString(w.keep(n, w.cur, true))
			continue
		}
		switch n.Type {
		case "text":
			b.WriteString(w.marked(n))
		case "hardBreak":
			b.WriteString("\n")
		case "mention", "emoji", "status", "date", "inlineCard":
			b.WriteString(inlineLabel(n))
		case "media", "mediaInline":
			if alt, _ := n.Attrs["alt"].(string); alt != "" {
				b.WriteString(alt)
			}
		default:
			b.WriteString(w.inlines(n.Content))
		}
	}
	return b.String()
}

func inlineLabel(n node) string {
	if s, _ := n.Attrs["text"].(string); s != "" {
		return s
	}
	switch n.Type {
	case "emoji":
		if s, _ := n.Attrs["shortName"].(string); s != "" {
			return s
		}
	case "date":
		if ts, _ := n.Attrs["timestamp"].(string); ts != "" {
			return ts
		}
	case "inlineCard":
		if u, _ := n.Attrs["url"].(string); u != "" {
			return u
		}
	}
	return ""
}

func (w *writer) marked(n node) string {
	s := n.Text
	code := false
	for _, m := range n.Marks {
		if m.Type == "code" {
			code = true
		}
	}
	if code {
		s = codeSpan(s)
	} else if w.escape {
		s = escapeInline(s)
	}
	var href, title string
	for _, m := range n.Marks {
		switch m.Type {
		case "strong":
			s = "**" + s + "**"
		case "em":
			s = "_" + s + "_"
		case "strike":
			s = "~~" + s + "~~"
		case "link":
			href, _ = m.Attrs["href"].(string)
			title, _ = m.Attrs["title"].(string)
		}
	}
	if href != "" {
		if title != "" {
			return "[" + s + "](" + href + " \"" + strings.ReplaceAll(title, `"`, `\"`) + "\")"
		}
		return "[" + s + "](" + href + ")"
	}
	return s
}

func codeSpan(s string) string {
	run := "`"
	for strings.Contains(s, run) {
		run += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return run + s + run
}

// Line-leading block markers, which would start a heading, quote, list or
// rule if left unescaped; digits before `.`/`)` too.
var leadingEscape = regexp.MustCompile(`(?m)^(\s*)([#>+\-=]|\d+[.)])`)

// escapeInline neutralizes the markdown punctuation in literal text that
// FromMarkdown would otherwise read as syntax, so a Jira-authored "2 * 3"
// comes back as "2 * 3" and "[x]" stays brackets. Only positions where the
// character could open or close something are escaped — a_b and 2 * 3 are
// left alone, because CommonMark leaves them alone too.
func escapeInline(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		prev, next := ' ', ' '
		if i > 0 {
			prev = rs[i-1]
		}
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		switch r {
		case '\\', '`', '[':
			b.WriteRune('\\')
		case '*':
			if !isSpace(prev) || !isSpace(next) {
				b.WriteRune('\\')
			}
		case '_':
			if !(isWord(prev) && isWord(next)) && !(isSpace(prev) && isSpace(next)) {
				b.WriteRune('\\')
			}
		case '~':
			if next == '~' || prev == '~' {
				b.WriteRune('\\')
			}
		case '<':
			if isWord(next) || next == '/' || next == '!' || next == '?' {
				b.WriteRune('\\')
			}
		}
		b.WriteRune(r)
	}
	out := b.String()
	return leadingEscape.ReplaceAllStringFunc(out, func(m string) string {
		sub := leadingEscape.FindStringSubmatch(m)
		return sub[1] + `\` + sub[2]
	})
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }

func isWord(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 0x7f
}

func attrInt(attrs map[string]any, key string, def int) int {
	switch v := attrs[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// String is a debugging aid: the doc's node kinds in document order.
func kinds(raw json.RawMessage) string {
	var d node
	if json.Unmarshal(raw, &d) != nil {
		return "?"
	}
	var out []string
	var walk func(n node, depth int)
	walk = func(n node, depth int) {
		out = append(out, fmt.Sprintf("%s%s", strings.Repeat(" ", depth), n.Type))
		for _, c := range n.Content {
			walk(c, depth+1)
		}
	}
	walk(d, 0)
	return strings.Join(out, "\n")
}

// trimEdgeSpace strips leading space from the first text node and trailing
// space from the last, dropping either if that empties it.
func trimEdgeSpace(nodes []any) []any {
	if len(nodes) == 0 {
		return nodes
	}
	if m, ok := nodes[0].(map[string]any); ok && m["type"] == "text" {
		m["text"] = strings.TrimLeft(m["text"].(string), " ")
		if m["text"] == "" {
			nodes = nodes[1:]
		}
	}
	if len(nodes) == 0 {
		return nil
	}
	if m, ok := nodes[len(nodes)-1].(map[string]any); ok && m["type"] == "text" {
		m["text"] = strings.TrimRight(m["text"].(string), " ")
		if m["text"] == "" {
			nodes = nodes[:len(nodes)-1]
		}
	}
	return nodes
}
