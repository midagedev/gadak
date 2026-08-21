// Package sqlhint is the shared zero-row display-name warning used by
// gadak sql and gadak_query (MCP). Status, issue type, and priority names
// are localized per account; filtering on the display column is the locale
// trap that returns zero rows silently.
package sqlhint

import (
	"regexp"
	"strings"
)

// ZeroRowWarning is the sentence both surfaces print when a query compared a
// display-name column and returned nothing.
const ZeroRowWarning = "zero rows with a display-name filter; status/priority/type are localized — retry with status_category, priority_rank, or issue_type_id"

// displayNameFilterRe matches a comparison against a display-name column:
// status, issue_type (also the Jira-API spelling issuetype), or priority.
// Word boundaries keep the stable id/category spellings from matching —
// status_id, status_category, issue_type_id, priority_rank all continue with
// `_…` where the pattern requires `=`, and a leading `_` is not a boundary.
var displayNameFilterRe = regexp.MustCompile(`(?i)\b(status|issue_type|issuetype|priority)\s*=`)

// ZeroRowDisplayNameWarning returns ZeroRowWarning when rowCount is 0 and
// query compares a localized display-name column. Comments are stripped
// first so a commented example is not a filter.
func ZeroRowDisplayNameWarning(query string, rowCount int) string {
	if rowCount != 0 {
		return ""
	}
	if !displayNameFilterRe.MatchString(stripSQLComments(query)) {
		return ""
	}
	return ZeroRowWarning
}

// stripSQLComments removes -- line comments and /* */ block comments outside
// of string literals so a leading comment does not hide the real keyword.
func stripSQLComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\'' {
			b.WriteByte('\'')
			i++
			for i < len(s) {
				if s[i] == '\'' {
					b.WriteByte('\'')
					i++
					if i < len(s) && s[i] == '\'' {
						b.WriteByte('\'')
						i++
						continue
					}
					break
				}
				b.WriteByte(s[i])
				i++
			}
			continue
		}
		if s[i] == '"' {
			b.WriteByte('"')
			i++
			for i < len(s) {
				c := s[i]
				b.WriteByte(c)
				i++
				if c == '"' {
					if i < len(s) && s[i] == '"' {
						b.WriteByte('"')
						i++
						continue
					}
					break
				}
			}
			continue
		}
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < len(s) {
				i += 2
			}
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
