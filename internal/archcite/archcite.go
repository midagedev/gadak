// Package archcite extracts repo-path citations from code comments. It is
// the checker behind the gate test beside it: a comment that names a path
// under cmd/, internal/ or web/src/ must name a path that exists. Dead
// citations — the audit's GDK-1569 found four pointing at a file that no
// longer owned what the comment described — are otherwise invisible: no
// gate reads comments, and the reader trusts the pointer until it lies.
//
// The check is path-existence only. A citation of an existing file whose
// symbol moved (this round's four: the file was there, the rule was not)
// still needs eyes; symbol resolution is deliberately not attempted.
package archcite

import (
	"regexp"
	"strings"
)

// citeRe matches one repo-path citation: a root (cmd, internal, web/src),
// then slash-separated segments of letters, digits, '_' and '-' with
// dotted extensions (done-words.test.ts) — a bare root is not a citation,
// one segment is (internal/store, internal/pairing.Offer). The leading
// boundary keeps import paths (…/gadak/internal/jql) and words (recmd)
// from matching: the character before the root must not be a word
// character, '.', '/' or '-'. RE2 has no lookbehind, so the boundary is a
// consuming group — FindAll still returns every citation, because a
// non-match boundary character is not part of two matches' prefixes.
var citeRe = regexp.MustCompile(
	`(?:^|[^A-Za-z0-9_./-])((?:cmd|internal|web/src)/[A-Za-z0-9_-]+(?:\.[A-Za-z0-9]+)*(?:/[A-Za-z0-9_-]+(?:\.[A-Za-z0-9]+)*)*)`)

// citation is one repo-path citation with the line it sat on.
type citation struct {
	Path string
	Line int    // 1-based line of Text within the file
	Text string // the comment line, for the failure message
}

// citations extracts the citations from comment text, which may span
// lines. start is the 1-based line of text's first line.
func citations(text string, start int) []citation {
	var out []citation
	line := start
	for _, l := range strings.Split(text, "\n") {
		for _, m := range citeRe.FindAllStringSubmatch(l, -1) {
			out = append(out, citation{Path: m[1], Line: line, Text: strings.TrimRight(l, " \t\r")})
		}
		line++
	}
	return out
}
