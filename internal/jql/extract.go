package jql

import (
	"net/url"
	"regexp"
	"strings"
)

// Extracted is the JQL (or filter id) pulled out of a paste — a raw query, a
// Jira navigator URL, or a `jql=` fragment.
type Extracted struct {
	JQL      string
	FilterID string
	IsURL    bool
}

var (
	fieldOpRe = regexp.MustCompile(`(?i)\b(project|projectkey|status|statuscategory|statuscategoryid|assignee|reporter|labels?|priority|issuetype|type|components?|fixversions?|created|createddate|updated|updateddate|resolution|text|summary|description|comment|key|issuekey|issue)\b\s*(=|!=|~|!~|>=|<=|>|<|\bin\b|\bis\b)`)
	orderByRe = regexp.MustCompile(`(?i)\border\s+by\b`)
	jqlEqRe   = regexp.MustCompile(`(?:\?|&|#|^)jql=`)
)

// Extract pulls JQL out of a navigator URL or returns the string as-is.
// A URL that only carries filter=<id> sets FilterID and leaves JQL empty —
// the id is not in the mirror.
func Extract(input string) Extracted {
	input = strings.TrimSpace(input)
	if input == "" {
		return Extracted{}
	}

	if u, err := url.Parse(input); err == nil && u.Scheme != "" && u.Host != "" {
		out := Extracted{IsURL: true}
		q := u.Query()
		if j := strings.TrimSpace(q.Get("jql")); j != "" {
			out.JQL = j
		}
		if id := strings.TrimSpace(q.Get("filter")); id != "" {
			out.FilterID = id
		}
		if out.JQL == "" && u.Fragment != "" {
			if fq, err := url.ParseQuery(strings.TrimPrefix(u.Fragment, "?")); err == nil {
				if j := strings.TrimSpace(fq.Get("jql")); j != "" {
					out.JQL = j
				}
				if id := strings.TrimSpace(fq.Get("filter")); id != "" && out.FilterID == "" {
					out.FilterID = id
				}
			}
		}
		if out.JQL != "" || out.FilterID != "" {
			return out
		}
		// A Jira URL with neither — still not FTS text.
		return out
	}

	// Bare "?jql=..." / "jql=..." pasted from a query string.
	if loc := jqlEqRe.FindStringIndex(input); loc != nil {
		raw := input[loc[1]:]
		if amp := strings.IndexByte(raw, '&'); amp >= 0 {
			raw = raw[:amp]
		}
		if j, err := url.QueryUnescape(raw); err == nil {
			raw = j
		}
		raw = strings.TrimSpace(raw)
		if raw != "" {
			return Extracted{JQL: raw, IsURL: true}
		}
	}

	return Extracted{JQL: input}
}

// LooksLike reports whether the paste should be tried as JQL rather than
// full-text. Conservative: plain words stay FTS.
func LooksLike(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	ex := Extract(s)
	if ex.IsURL || ex.FilterID != "" {
		return true
	}
	if fieldOpRe.MatchString(s) {
		return true
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "currentuser()") {
		return true
	}
	return orderByRe.MatchString(s)
}
