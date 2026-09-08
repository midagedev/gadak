package adf

import (
	"encoding/json"
	"strings"
)

// Presented is one body as a reader and an editor need it, derived from what
// the mirror holds (GDK-1385). The mirror keeps the origin's shape — ADF for
// Jira, Confluence and the Built-in tracker, markdown text for Linear — and
// never a converted copy; the conversion happens here, on the way out.
type Presented struct {
	// Display is the document to render. For a body whose ADF is simple
	// (typed text — an older jira.Doc, a migration) and for a markdown-only
	// body it is FromMarkdown of the text, so the `##` and `**` a person or
	// an agent typed become headings and emphasis. A rich ADF is displayed
	// as it is.
	Display json.RawMessage
	// Source is the markdown an editor opens with: the typed text for a
	// simple body, an escaped serialization for a rich one — with a
	// placeholder standing in for each node markdown cannot carry.
	Source string
	// Loss names what a markdown edit of this body would destroy — the
	// nodes and marks outside the markdown subset. Empty when the body
	// round-trips.
	Loss []string
}

// Dialect is the markup a body's text column holds (GDK-1637). The dialect
// comes from the origin type, never from sniffing the string.
type Dialect int

const (
	// DialectMarkdown: the text is markdown — Cloud Jira, Linear, and the
	// built-in tracker all store bodies that convert to and from it.
	DialectMarkdown Dialect = iota
	// DialectWiki: the text is Jira wiki markup — a Jira Server origin,
	// whose bodies are carried verbatim in both directions.
	DialectWiki
)

// Present derives the reader's and editor's view of a body from the mirror's
// two columns: the ADF (empty on Linear) and the text. The dialect says what
// the text is when the ADF column holds no document.
func Present(raw json.RawMessage, text string, dialect Dialect) Presented {
	if dialect == DialectWiki && !hasDoc(raw) {
		// A Jira Server body is a wiki-markup string, carried verbatim
		// (GDK-1637): the mirror has no document — the column is empty, or
		// holds the origin's own JSON string, which the sync copies as it
		// came — so the text is shown as the literal characters it is.
		if text == "" {
			return Presented{}
		}
		return Presented{Display: wikiDisplay(text), Source: text}
	}
	if isNull(raw) {
		if text == "" {
			return Presented{}
		}
		return Presented{Display: FromMarkdown(text), Source: text}
	}
	if IsSimple(string(raw)) {
		src := Markdown(raw)
		return Presented{Display: FromMarkdown(src), Source: src}
	}
	// Source carries a placeholder for every node markdown cannot hold
	// (preserve.go, GDK-1396); Loss still names their kinds, so a UI can say
	// what the markers stand for.
	return Presented{Display: raw, Source: Source(raw), Loss: FormatLoss(string(raw))}
}

// wikiDisplay is the reader's view of a wiki-markup body: one codeBlock
// holding the characters unchanged — the renderer's existing monospace block,
// no markdown interpretation, no new node type.
func wikiDisplay(text string) json.RawMessage {
	b, err := json.Marshal(map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{map[string]any{
			"type":    "codeBlock",
			"content": []any{map[string]any{"type": "text", "text": text}},
		}},
	})
	if err != nil {
		return nil // unreachable: every value above is marshal-infallible
	}
	return b
}

// hasDoc reports whether raw holds an ADF document — as opposed to nothing,
// or to the plain JSON string a Jira Server origin sends in the field an ADF
// origin fills with a document (GDK-1637).
func hasDoc(raw json.RawMessage) bool {
	return strings.HasPrefix(strings.TrimSpace(string(raw)), "{")
}

func isNull(raw json.RawMessage) bool {
	s := string(raw)
	return len(raw) == 0 || s == "null" || s == `""`
}
