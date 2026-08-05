package fields

import (
	"fmt"
	"strings"
	"unicode"
)

// SuggestAlias proposes a stable config key for a field the config does not
// name yet.
//
// The slug is ASCII-only, and a name with no ASCII letters falls back to the
// field id (cf_10019). Two reasons, both from the same fact: Jira returns field
// names in the account's own language. A Korean account calls customfield_10019
// "순위" where an English one says "Rank", so a name-derived alias is not the
// same on two machines — and a fieldMap is exactly the thing teams share with
// `scry team export`. An id-derived alias is ugly but identical everywhere.
func SuggestAlias(name, fieldID string, used map[string]bool) string {
	base := ASCIISlug(name)
	if base == "" {
		base = FieldIDSlug(fieldID)
	}
	if base == "" {
		base = "field"
	}
	if !used[base] {
		return base
	}
	tail := fieldID
	if strings.HasPrefix(fieldID, "customfield_") {
		tail = strings.TrimPrefix(fieldID, "customfield_")
	}
	candidate := base + "_" + tail
	if !used[candidate] {
		return candidate
	}
	// Extremely unlikely: keep appending until free.
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s_%d", candidate, i)
		if !used[c] {
			return c
		}
	}
}

// ASCIISlug lowercases and snake-cases the ASCII letters and digits in s,
// dropping everything else. Returns "" when s carries no ASCII alphanumerics,
// which is the signal to fall back to the field id — see SuggestAlias.
func ASCIISlug(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(s) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// FieldIDSlug turns customfield_10019 into cf_10019.
func FieldIDSlug(fieldID string) string {
	return "cf_" + strings.TrimPrefix(ASCIISlug(fieldID), "customfield_")
}

// NormalizeName collapses whitespace runs, trims, and lowercases for grouping
// fields that share a display name across board templates.
func NormalizeName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}
