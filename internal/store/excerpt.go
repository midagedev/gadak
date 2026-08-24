package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/midagedev/gadak/internal/adf"
)

// pageExcerptRunes is the max length of pages.excerpt (rune-safe for CJK).
const pageExcerptRunes = 200

// pageExcerptFromADF builds the stored pages.excerpt value: ADF plain text,
// whitespace collapsed to single spaces, at most pageExcerptRunes runes, cut at
// a word boundary when one exists in the window (CJK has no spaces — rune cut).
// Flattening is adf.PlainText — the same walker FTS indexes (GDK-814 closed a
// guard-less local copy that dropped wiki-markup string bodies and mention
// labels from excerpts while FTS kept them).
func pageExcerptFromADF(raw string) string {
	return pageExcerptFromPlain(adf.PlainText(json.RawMessage(raw)))
}

// pageExcerptFromPlain applies the same normalize + truncate rules to already-
// flattened text (used by tests).
func pageExcerptFromPlain(s string) string {
	s = normalizeWhitespace(s)
	if s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= pageExcerptRunes {
		return s
	}
	r := []rune(s)
	cut := r[:pageExcerptRunes]
	// Prefer the last space in the window so Latin words are not mid-cut.
	for i := len(cut) - 1; i >= 0; i-- {
		if unicode.IsSpace(cut[i]) {
			return strings.TrimSpace(string(cut[:i]))
		}
	}
	// No whitespace (CJK run, or one long token) — hard cut at the rune limit.
	return string(cut)
}

// normalizeWhitespace collapses every Unicode whitespace run (including
// newlines and tabs) to a single ASCII space and trims ends.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// backfillPageExcerpts fills pages.excerpt from body_adf for existing rows.
// Called from the v15 migration in the same transaction as the ALTER.
func backfillPageExcerpts(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT item_id, COALESCE(body_adf, '') FROM pages`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id, adf string
	}
	var batch []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.adf); err != nil {
			return err
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range batch {
		ex := pageExcerptFromADF(r.adf)
		if _, err := tx.Exec(`UPDATE pages SET excerpt = ? WHERE item_id = ?`, ex, r.id); err != nil {
			return err
		}
	}
	return nil
}
