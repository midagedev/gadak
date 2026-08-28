package jira

import (
	"encoding/json"
	"sort"
	"strings"
)

// Doc builds the ADF document a comment body has to be. The composer sends plain
// text with `@Display Name` typed into it plus the account ids it resolved, so
// this is where those become real mention nodes — a mention that stays plain text
// notifies nobody, which is the whole point of typing it.
//
// mentions maps display name to account id.
func Doc(text string, mentions map[string]string) json.RawMessage {
	return DocWithMedia(text, mentions, nil)
}

// Media is one inline image in a comment: the Jira media UUID (not the
// attachment id — see Client.MediaRef) plus the filename, which is carried as
// `alt` so our own renderer can match the node to the attachment without
// persisting the UUID anywhere (web/src/lib/adf.ts, findAttachment).
type Media struct {
	ID       string
	Filename string
}

// CodeRegion is one byte range [Start, End) of a text that markdown renders
// as code — where an `@` names data, not a person.
type CodeRegion struct {
	Start, End int
}

// CodeRegions is a text's set of code regions.
type CodeRegions []CodeRegion

// Cover reports whether the byte at off lies inside one of the regions.
func (rs CodeRegions) Cover(off int) bool {
	for _, r := range rs {
		if off >= r.Start && off < r.End {
			return true
		}
	}
	return false
}

// FindCodeRegions returns the byte ranges of text that markdown renders as
// code: fenced blocks (``` or ~~~ at line start) and inline code spans
// (backtick runs closed by a run of the same length). It is the single owner
// of that judgment for the two surfaces that must agree on it — mention
// candidate extraction (cmd/gadak/agent.go) and the @Name substitution below:
// code on one side and a person on the other is how a package name summons a
// user (GDK-894).
//
// This is a minimal scanner following CommonMark's basic rules — fences need
// their opening character, a run of ≥3, and ≤3 leading spaces; inline spans
// close on the next run of exactly the same length; an unmatched backtick run
// is literal text — not a full implementation. The asymmetry it is built for:
// over-excluding leaves a mention as plain text, visible and rephrasable,
// while under-excluding silently summons a person into a conversation about
// code.
func FindCodeRegions(text string) CodeRegions {
	fences := fenceRegions(text)
	spans := spanRegions(text, fences)
	out := make(CodeRegions, 0, len(fences)+len(spans))
	out = append(out, fences...)
	out = append(out, spans...)
	return out
}

// fenceRegions marks fenced code blocks, whole-line based. A region covers
// the opening fence line (an `@` in its info string is code too) through the
// end of the closing fence line; an unclosed fence runs to the end of text.
func fenceRegions(text string) CodeRegions {
	var out CodeRegions
	var fenceChar byte
	fenceLen, start := 0, 0
	lineStart := 0
	for lineStart <= len(text) {
		end := len(text)
		if nl := strings.IndexByte(text[lineStart:], '\n'); nl >= 0 {
			end = lineStart + nl
		}
		line := text[lineStart:end]
		if fenceChar == 0 {
			if ch, n := fenceOpen(line); n > 0 {
				fenceChar, fenceLen, start = ch, n, lineStart
			}
		} else if fenceClosed(line, fenceChar, fenceLen) {
			out = append(out, CodeRegion{start, end})
			fenceChar = 0
		}
		if end == len(text) {
			break
		}
		lineStart = end + 1
	}
	if fenceChar != 0 {
		out = append(out, CodeRegion{start, len(text)})
	}
	return out
}

// fenceOpen reports the fence character and run length when a line opens a
// fence: ≤3 leading spaces, then a run of ≥3 backticks or tildes. A backtick
// fence's info string may not itself contain a backtick (CommonMark).
func fenceOpen(line string) (byte, int) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return 0, 0
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return 0, 0
	}
	n := i
	for n < len(line) && line[n] == ch {
		n++
	}
	if n-i < 3 {
		return 0, 0
	}
	if ch == '`' && strings.IndexByte(line[n:], '`') >= 0 {
		return 0, 0
	}
	return ch, n - i
}

// fenceClosed matches a closing fence: same character, a run at least as long
// as the opening one, nothing after it but spaces (a trailing \r too, for
// CRLF text the caller did not normalize).
func fenceClosed(line string, ch byte, openLen int) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	n := i
	for n < len(line) && line[n] == ch {
		n++
	}
	return n-i >= openLen && strings.TrimRight(line[n:], " \r") == ""
}

// spanRegions marks inline code spans: a backtick run of length N opens, the
// next run of exactly N closes — CommonMark's rule for quoting a lone
// backtick. The closer search runs over the remaining text but never inside
// a fenced region; an opener with no closer is literal text and protects
// nothing.
func spanRegions(text string, fences CodeRegions) CodeRegions {
	var out CodeRegions
	i, f := 0, 0
	for i < len(text) {
		for f < len(fences) && fences[f].End <= i {
			f++
		}
		if f < len(fences) && i >= fences[f].Start {
			i = fences[f].End
			continue
		}
		if text[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(text) && text[j] == '`' {
			j++
		}
		if end := closingRunEnd(text, j, j-i, fences); end > 0 {
			out = append(out, CodeRegion{i, end})
			i = end
		} else {
			i = j
		}
	}
	return out
}

// closingRunEnd finds the end offset of the backtick run that closes an
// opener of length n whose run ended at j, or -1 when it is unmatched.
func closingRunEnd(text string, j, n int, fences CodeRegions) int {
	f := 0
	for k := j; k < len(text); {
		for f < len(fences) && fences[f].End <= k {
			f++
		}
		if f < len(fences) && k >= fences[f].Start {
			k = fences[f].End
			continue
		}
		if text[k] != '`' {
			k++
			continue
		}
		m := k
		for m < len(text) && text[m] == '`' {
			m++
		}
		if m-k == n {
			return m
		}
		k = m
	}
	return -1
}

// DocWithMedia is Doc plus inline images, appended after the text — where a
// screenshot belongs in a comment that describes it.
func DocWithMedia(text string, mentions map[string]string, media []Media) json.RawMessage {
	// Longest name first: "@김현" must not win over "@김현철".
	names := make([]string, 0, len(mentions))
	for name := range mentions {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	var code CodeRegions
	if len(names) > 0 {
		// Region offsets are computed on the normalized text the loop below
		// walks, so the two never disagree about where a byte sits.
		code = FindCodeRegions(normalized)
	}
	lines := strings.Split(normalized, "\n")
	content := make([]any, 0, len(lines))
	offset := 0
	for _, line := range lines {
		para := map[string]any{"type": "paragraph"}
		if nodes := inline(line, offset, code, names, mentions); len(nodes) > 0 {
			para["content"] = nodes
		}
		content = append(content, para)
		offset += len(line) + 1
	}
	for _, m := range media {
		if m.ID == "" {
			continue
		}
		// collection must be present and empty for an issue attachment; Jira
		// rejects the node without it.
		attrs := map[string]any{"type": "file", "id": m.ID, "collection": ""}
		if m.Filename != "" {
			attrs["alt"] = m.Filename
		}
		content = append(content, map[string]any{
			"type":    "mediaSingle",
			"attrs":   map[string]any{"layout": "center"},
			"content": []any{map[string]any{"type": "media", "attrs": attrs}},
		})
	}
	doc, err := json.Marshal(map[string]any{"type": "doc", "version": 1, "content": content})
	if err != nil {
		return json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	}
	return doc
}

// inline splits one line into text and mention nodes. lineStart is the line's
// offset in the whole text, so code regions — which can span lines — can be
// tested against each `@` (GDK-894: a token the composer quoted as code must
// not become a mention node here, whatever the mentions map says).
func inline(line string, lineStart int, code CodeRegions, names []string, ids map[string]string) []any {
	nodes := []any{}
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			nodes = append(nodes, map[string]any{"type": "text", "text": buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(line); {
		if line[i] == '@' && !code.Cover(lineStart+i) {
			if name := match(line[i+1:], names); name != "" {
				flush()
				nodes = append(nodes, map[string]any{
					"type":  "mention",
					"attrs": map[string]any{"id": ids[name], "text": "@" + name},
				})
				i += 1 + len(name)
				continue
			}
		}
		buf.WriteByte(line[i])
		i++
	}
	flush()
	return nodes
}

func match(rest string, names []string) string {
	for _, name := range names {
		if strings.HasPrefix(rest, name) {
			return name
		}
	}
	return ""
}
