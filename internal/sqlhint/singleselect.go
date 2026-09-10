package sqlhint

import "strings"

// The single-statement SELECT/WITH gate. Two surfaces refuse the same shapes —
// the MCP query tool and the saved groupQuery in settings — and both used to
// re-walk StripComments → TrimSpace → semicolon scan → FirstKeyword in their
// own copy (GDK-928). The walk lives here now; the rejection wording stays
// with the caller, because the two surfaces address different readers (an
// agent reading a tool error, a user saving a setting) and merging their
// sentences would be a user-visible change, not a cleanup.

// SingleSelectVerdict is what ClassifySingleSelect concluded.
type SingleSelectVerdict int

const (
	// SingleSelectOK: exactly one statement, starting with SELECT or WITH.
	SingleSelectOK SingleSelectVerdict = iota
	// SingleSelectEmpty: nothing left once comments and space are stripped.
	SingleSelectEmpty
	// SingleSelectMultiStatement: a semicolon other than one trailing.
	SingleSelectMultiStatement
	// SingleSelectNoKeyword: non-empty, but no leading keyword to read.
	SingleSelectNoKeyword
	// SingleSelectOtherKeyword: a leading keyword that is not SELECT or WITH.
	// The returned keyword is the one found, verbatim, for the caller's message.
	SingleSelectOtherKeyword
)

// ClassifySingleSelect reports whether q is a single SELECT or WITH statement.
// The second result is the leading keyword as written, set only for
// SingleSelectOtherKeyword. Callers own the error sentences.
func ClassifySingleSelect(q string) (SingleSelectVerdict, string) {
	s := strings.TrimSpace(StripComments(q))
	if s == "" {
		return SingleSelectEmpty, ""
	}
	// Allow one trailing semicolon; anything else is multi-statement.
	body := strings.TrimRight(s, " \t\n\r;")
	if strings.Contains(body, ";") {
		return SingleSelectMultiStatement, ""
	}
	kw := FirstKeyword(body)
	switch strings.ToUpper(kw) {
	case "SELECT", "WITH":
		return SingleSelectOK, ""
	case "":
		return SingleSelectNoKeyword, ""
	default:
		return SingleSelectOtherKeyword, kw
	}
}
