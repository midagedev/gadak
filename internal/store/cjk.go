package store

import "strings"

// CJK mid-compound search (GDK-259 / docs/decisions/0009): items_fts keeps
// the unicode61 tokenizer and gains a fourth column, cjk_bigram, carrying the
// overlapping 2-grams of CJK runs. Query rewriting (read.go ftsPrefixQuery)
// turns a CJK term of two or more runes into the AND of its bigrams, which is
// the only way `결제` hits `간편결제` without switching tokenizers.

// cjkRanges is the rune set treated as CJK, kept in lockstep with
// scripts/scrub-demo-db.py (cjk_ranges): the portable snapshot rebuild must
// emit the same bigrams this build writes, or the hosted copy silently loses
// CJK mid-match while the local mirror keeps it.
var cjkRanges = [][2]rune{
	{0x1100, 0x11FF},   // Hangul Jamo
	{0x3041, 0x30FF},   // Hiragana + Katakana
	{0x3130, 0x318F},   // Hangul Compatibility Jamo
	{0x31F0, 0x31FF},   // Katakana Phonetic Extensions
	{0x3400, 0x4DBF},   // CJK Extension A
	{0x4E00, 0x9FFF},   // CJK Unified Ideographs
	{0xA960, 0xA97F},   // Hangul Jamo Extended-A
	{0xAC00, 0xD7A3},   // Hangul Syllables
	{0xD7B0, 0xD7FF},   // Hangul Jamo Extended-B
	{0xF900, 0xFAFF},   // CJK Compatibility Ideographs
	{0x20000, 0x2FFFD}, // CJK Extensions (Plane 2)
	{0x30000, 0x3FFFD}, // CJK Extension G+ (Plane 3)
}

func isCJKRune(r rune) bool {
	for _, rg := range cjkRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// isCJKTerm reports whether s is one CJK term: non-empty and every rune CJK.
// Mixed-script terms (결제API) are not — they take the ordinary prefix query.
func isCJKTerm(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isCJKRune(r) {
			return false
		}
	}
	return true
}

// cjkBigrams returns the overlapping 2-grams of every CJK run in s, in scan
// order. A run of one rune emits nothing: one-rune CJK queries keep the
// token-start rewrite, because indexing unigrams was measured and rejected —
// `결` would light up 결과/결재/결제 alike (0009 §4 A-uni). Non-CJK runes
// break runs, so 결제API결제 yields two 결제 grams, never one spanning the
// Latin part.
func cjkBigrams(s string) []string {
	var out []string
	run := make([]rune, 0, 8)
	emit := func() {
		for i := 0; i+1 < len(run); i++ {
			out = append(out, string(run[i:i+2]))
		}
		run = run[:0]
	}
	for _, r := range s {
		if isCJKRune(r) {
			run = append(run, r)
			continue
		}
		emit()
	}
	emit()
	return out
}

// FTSCJKBigramColumn is the items_fts.cjk_bigram value for one row: the CJK
// bigrams of the title, body and comment text, space-joined. Exported because
// internal/snapshot writes rows through the same shape — any writer that
// fills only the three scored columns leaves CJK mid-match silently empty
// rather than broken, which is this design's named trap (0009 §Consequences).
func FTSCJKBigramColumn(title, body, comments string) string {
	parts := make([]string, 0, 3)
	for _, text := range []string{title, body, comments} {
		if grams := cjkBigrams(text); len(grams) > 0 {
			parts = append(parts, strings.Join(grams, " "))
		}
	}
	return strings.Join(parts, " ")
}
