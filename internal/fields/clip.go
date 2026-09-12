package fields

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Clip is the single owner of "collapse whitespace, then truncate to a
// display width with an ellipsis" (GDK-1811). Three byte-identical copies
// of this body had drifted apart in comment only — cmd/gadak's clip,
// internal/parenthint's clip, internal/freshness's clipWidth — and the
// freshness copy justified itself with "package main is not importable",
// which was true and beside the point.
//
// Home: this package, not internal/term. The audit round that folded the
// three copies was pointed at internal/term, which turned out to own PTY
// session handling — its OSC parsing is escape-sequence work, not display
// width — and importing it would have pulled creack/pty and x/sys into
// internal/freshness, which every MCP read touches. Lead correction,
// 2026-09-12; internal/fields is where this binary's width authority
// already lived.
//
// Columns, not runes: a Hangul or CJK rune occupies two cells, so a
// rune-counted cut renders twice as wide as the same cut in ASCII — which
// is how a 72-"character" column landed at 144 on a Korean issue.
// go-runewidth is this repo's width authority; the ellipsis is one cell,
// and Truncate keeps the whole result inside cols.
//
// Whitespace first, because a value that is clipped for a one-line cell is
// a value whose newlines and runs of spaces would break that cell anyway.
func Clip(s string, cols int) string {
	s = strings.Join(strings.Fields(s), " ")
	if runewidth.StringWidth(s) <= cols {
		return s
	}
	return runewidth.Truncate(s, cols, "…")
}
