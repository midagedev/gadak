package store

import (
	"strings"
	"unicode"
)

// CJK mid-compound search (GDK-259 / docs/decisions/0009): items_fts keeps
// unicode61's token boundaries (under the porter stemming wrapper since
// GDK-1021) and carries the cjk_bigram column, holding the overlapping
// 2-grams of CJK runs. Query rewriting (read.go ftsPrefixQuery)
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
// bigrams of the title, label, body and comment text, space-joined. Labels
// joined the source set in GDK-1021 — a Korean label like 간편결제 must
// mid-compound-match 결제 exactly like a title token would, and the plain
// labels column alone only gives whole-token matching. Exported because
// internal/snapshot writes rows through the same shape — any writer that
// fills only the scored columns leaves CJK mid-match silently empty
// rather than broken, which is this design's named trap (0009 §Consequences).
func FTSCJKBigramColumn(title, labels, body, comments string) string {
	parts := make([]string, 0, 4)
	for _, text := range []string{title, labels, body, comments} {
		if grams := cjkBigrams(text); len(grams) > 0 {
			parts = append(parts, strings.Join(grams, " "))
		}
	}
	return strings.Join(parts, " ")
}

// FTSLabelsText is the items_fts.labels value for one row: the stored JSON
// label array (issues_raw.labels / pages.labels, both "[]" when absent) as
// space-joined text. The single owner of the column's text form — writeFTS
// callers, the rebuild, the snapshot pipeline and scripts/scrub-demo-db.py's
// portable twin must all emit this shape, or a label-only issue silently
// drops out of search again (GDK-1021).
func FTSLabelsText(labelsJSON string) string {
	return strings.Join(parseArray(&labelsJSON), " ")
}

// isTokenRune reports whether r is a non-CJK token character for the
// script_runs column: a letter or digit (unicode61's token set, the same set
// unicode61 counts Han, kana and Hangul as) that is not itself CJK by
// isCJKRune. Everything else — punctuation, spaces, symbols, CJK — breaks a
// run, exactly the boundaries unicode61 already breaks tokens on except the
// CJK ones, which is the whole point (GDK-1978).
func isTokenRune(r rune) bool {
	return !isCJKRune(r) && (unicode.IsLetter(r) || unicode.IsNumber(r))
}

// scriptRuns returns the maximal runs of non-CJK token characters (letters
// and digits) in s that sit immediately adjacent to a CJK rune — adjacent on
// either side — in scan order. A run is emitted iff the rune before its first
// character or the rune after its last character is CJK: those are exactly
// the runs unicode61 swallows into the neighboring CJK token, because it
// counts CJK as token characters. A run bounded by punctuation or space on
// both sides (デプロイは完了。NMB-140) was never swallowed — it is already its
// own token — and English text has no CJK to be glued to, so it emits nothing
// at all (GDK-1978).
func scriptRuns(s string) []string {
	rs := []rune(s)
	var out []string
	start := -1
	for i := 0; i <= len(rs); i++ {
		if i < len(rs) && isTokenRune(rs[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			left := start > 0 && isCJKRune(rs[start-1])
			right := i < len(rs) && isCJKRune(rs[i])
			if left || right {
				out = append(out, string(rs[start:i]))
			}
			start = -1
		}
	}
	return out
}

// FTSScriptRunsColumn is the items_fts.script_runs value for one row: the
// CJK-adjacent Latin/digit runs of the title, label, body and comment text,
// space-joined in scan order, so the phrase query ["nmb","110"*] matches the
// rescued `NMB 110` of 追跡issueはNMB-110で the same way it matches Korean's
// space-spelled NMB-110이며. The sixth and last items_fts column (GDK-1978);
// exported for the same writers as FTSCJKBigramColumn — a writer that omits
// it leaves the axis silently empty, the contentless trap of 0009
// §Consequences a third time.
func FTSScriptRunsColumn(title, labels, body, comments string) string {
	parts := make([]string, 0, 4)
	for _, text := range []string{title, labels, body, comments} {
		if runs := scriptRuns(text); len(runs) > 0 {
			parts = append(parts, strings.Join(runs, " "))
		}
	}
	return strings.Join(parts, " ")
}
