package adf

import "encoding/json"

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

// Present derives the reader's and editor's view of a body from the mirror's
// two columns: the ADF (empty on Linear) and the text.
func Present(raw json.RawMessage, text string) Presented {
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

func isNull(raw json.RawMessage) bool {
	s := string(raw)
	return len(raw) == 0 || s == "null" || s == `""`
}
