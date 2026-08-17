package store

import (
	"context"
	"database/sql"
	"strings"
)

// looksLikeKey reports whether q could be an item key. A key query is a
// single token (no whitespace) whose normalized form (lowercase, strip
// non-alphanumeric) is non-empty and contains at least one digit.
// "NMB-140", "nmb140", "NMB-14" qualify; "billing", "로그인", and
// "flaky upload" do not — those are searches, not lookups.
func looksLikeKey(q string) bool {
	if q == "" || strings.ContainsAny(q, " \t\n\r") {
		return false
	}
	n := normKey(q)
	if n == "" {
		return false
	}
	for _, r := range n {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// normKey matches the web's normKey (filters.svelte.ts): lowercase, then
// drop every rune that is not ASCII alphanumeric or a Hangul syllable.
func normKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '가' && r <= '힣') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// reconstructJiraKey turns a hyphenless (or punctuation-stripped) token into
// PROJ-N form by splitting off the trailing digit run: "nmb140" → "NMB-140".
// All-digit page ids and letter-only tokens return "".
func reconstructJiraKey(q string) string {
	n := normKey(q)
	if n == "" {
		return ""
	}
	i := len(n)
	for i > 0 && n[i-1] >= '0' && n[i-1] <= '9' {
		i--
	}
	if i == 0 || i == len(n) {
		return ""
	}
	return strings.ToUpper(n[:i]) + "-" + n[i:]
}

type keyHit struct {
	kind   string // issue | page
	key    string
	reason string // key-exact | key-prefix
	page   *PageLite
}

// lookupKeyHits resolves q against items.key before FTS runs.
// Order: case-insensitive exact, then normalized-key exact (reconstructed
// PROJ-N), then key prefix ordered by key. Capped at limit so a wide prefix
// cannot blow the window; FTS never evicts a hit that made this list.
func (db *DB) lookupKeyHits(ctx context.Context, q string, limit int) ([]keyHit, error) {
	if limit <= 0 {
		return nil, nil
	}
	forms := []string{q}
	if rec := reconstructJiraKey(q); rec != "" && !strings.EqualFold(rec, q) {
		forms = append(forms, rec)
	}
	// An all-digit query is the number half of a key: a person reads
	// "CRWN-4152" off a list and types "4152" (GDK-186). The exact number is
	// an exact lookup for any project; a shorter run is a prefix on it.
	digits := allDigitsQuery(q)

	seen := map[string]bool{}
	var exact, prefix []keyHit
	for _, form := range forms {
		hits, err := db.keysEqual(ctx, form)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if seen[h.key] {
				continue
			}
			seen[h.key] = true
			h.reason = "key-exact"
			exact = append(exact, h)
		}
	}
	if digits != "" {
		hits, err := db.keysNumberEqual(ctx, digits, limit)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if seen[h.key] {
				continue
			}
			seen[h.key] = true
			h.reason = "key-exact"
			exact = append(exact, h)
		}
	}
	for _, form := range forms {
		hits, err := db.keysPrefix(ctx, form, limit)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if seen[h.key] {
				continue
			}
			seen[h.key] = true
			h.reason = "key-prefix"
			prefix = append(prefix, h)
		}
	}
	if digits != "" {
		hits, err := db.keysNumberPrefix(ctx, digits, limit)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if seen[h.key] {
				continue
			}
			seen[h.key] = true
			h.reason = "key-prefix"
			prefix = append(prefix, h)
		}
	}
	out := append(exact, prefix...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (db *DB) keysEqual(ctx context.Context, key string) ([]keyHit, error) {
	return db.scanKeyHits(ctx, keyLookupSQL+` WHERE it.key = ? COLLATE NOCASE`, key)
}

// allDigitsQuery returns the normalized query when it is nothing but digits
// ("4152", "#4152"), else "". Pure digit queries are the one form the
// PROJ-then-digits reconstruction can never cover.
func allDigitsQuery(q string) string {
	n := normKey(q)
	if n == "" {
		return ""
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return n
}

// keysNumberEqual finds keys whose number half is exactly n: PROJ-<n> for any
// project. Page ids have no dash, so they never match here — their exact
// lookup is keysEqual.
func (db *DB) keysNumberEqual(ctx context.Context, n string, limit int) ([]keyHit, error) {
	return db.scanKeyHits(ctx, keyLookupSQL+`
		WHERE it.key LIKE ? ESCAPE '\' COLLATE NOCASE
		ORDER BY it.key
		LIMIT ?`, "%-"+n, limit)
}

// keysNumberPrefix finds keys whose number half starts with n.
func (db *DB) keysNumberPrefix(ctx context.Context, n string, limit int) ([]keyHit, error) {
	return db.scanKeyHits(ctx, keyLookupSQL+`
		WHERE it.key LIKE ? ESCAPE '\' COLLATE NOCASE
		ORDER BY it.key
		LIMIT ?`, "%-"+n+"%", limit)
}

func (db *DB) keysPrefix(ctx context.Context, prefix string, limit int) ([]keyHit, error) {
	return db.scanKeyHits(ctx, keyLookupSQL+`
		WHERE it.key LIKE ? ESCAPE '\' COLLATE NOCASE
		ORDER BY it.key
		LIMIT ?`, likePrefix(prefix), limit)
}

const keyLookupSQL = `
	SELECT it.kind, COALESCE(it.key, ''),
	       COALESCE(it.title, ''), COALESCE(p.space_key, ''), COALESCE(sp.name, ''),
	       COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''),
	       COALESCE(it.author, ''), COALESCE(it.author_id, ''),
	       COALESCE(it.updated_at, ''), COALESCE(p.version, 0), COALESCE(it.url, ''),
	       COALESCE(p.excerpt, ''), COALESCE(p.labels, '[]')
	FROM items it
	LEFT JOIN pages p ON p.item_id = it.id
	LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key`

func (db *DB) scanKeyHits(ctx context.Context, query string, args ...any) ([]keyHit, error) {
	var out []keyHit
	err := each(ctx, db.sql, query, func(rows *sql.Rows) error {
		var kind, key, title, spaceKey, spaceName, spaceHomepageID, parentID string
		var author, authorID, updatedAt, url, excerpt, labels string
		var version int
		if err := rows.Scan(&kind, &key, &title, &spaceKey, &spaceName, &spaceHomepageID, &parentID,
			&author, &authorID, &updatedAt, &version, &url, &excerpt, &labels); err != nil {
			return err
		}
		if key == "" || (kind != "issue" && kind != "page") {
			return nil
		}
		h := keyHit{kind: kind, key: key}
		if kind == "page" {
			h.page = &PageLite{
				Key: key, Title: title, SpaceKey: spaceKey, SpaceName: spaceName,
				SpaceHomepageID: spaceHomepageID, ParentID: parentID,
				Author: author, AuthorID: authorID, UpdatedAt: updatedAt,
				Version: version, URL: url, Excerpt: excerpt, Labels: parseArray(&labels),
			}
		}
		out = append(out, h)
		return nil
	}, args...)
	return out, err
}

func likePrefix(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 1)
	for _, r := range s {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}
