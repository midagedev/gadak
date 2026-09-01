// Package sqlhint is the shared SQL-error help used by gadak sql and
// gadak_query (MCP), and the owner of SQL comment stripping (StripComments)
// used by the group-query and MCP SELECT gates. Status, issue type, and
// priority names are localized per account; filtering on the display column
// is the locale trap that returns zero rows silently. A "no such column"
// error also gets a did-you-mean when the unknown name is close to a real
// column (GDK-255: issue_key → key).
package sqlhint

import (
	"database/sql"
	"fmt"
	"math"
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
	if !displayNameFilterRe.MatchString(StripComments(query)) {
		return ""
	}
	return ZeroRowWarning
}

// hintTables are the agent-facing relations whose columns we offer as
// did-you-mean candidates. The JSON/SQL name mismatch (issue_key vs key)
// lives on issues_full; items/pages/comments catch the rest of a typo.
var hintTables = []string{"issues_full", "items", "pages", "comments"}

var noSuchColumnRe = regexp.MustCompile(`(?i)no such column:\s+([^\s(]+)`)

// WithColumnSuggestion appends `did you mean "col"?` to a SQLite
// "no such column" error when one column from hintTables is close enough.
// When the name exists verbatim on a hint table, the query hit the wrong
// table — say which one holds it (GDK-974: summary lives on issues_full,
// and the miss was silent). Distant names stay unadorned — a bad guess is
// worse than none.
func WithColumnSuggestion(db *sql.DB, err error) error {
	if err == nil || db == nil {
		return err
	}
	name, ok := parseNoSuchColumn(err.Error())
	if !ok {
		return err
	}
	cols, owner, colErr := hintColumns(db)
	if colErr != nil || len(cols) == 0 {
		return err
	}
	if table, ok := owner[strings.ToLower(name)]; ok {
		return fmt.Errorf("%w; column %q exists on %s — query %s", err, name, table, table)
	}
	sug := suggestColumn(name, cols)
	if sug == "" {
		return err
	}
	return fmt.Errorf("%w; did you mean %q?", err, sug)
}

func parseNoSuchColumn(msg string) (string, bool) {
	m := noSuchColumnRe.FindStringSubmatch(msg)
	if len(m) < 2 {
		return "", false
	}
	name := strings.Trim(m[1], "`\"[]")
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return "", false
	}
	return name, true
}

// hintColumns lists the candidate columns and which hint table owns each
// name (first table in hintTables order wins, so issues_full claims key).
func hintColumns(db *sql.DB) ([]string, map[string]string, error) {
	owner := map[string]string{}
	var out []string
	for _, table := range hintTables {
		rows, err := db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			continue
		}
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull int
			var dflt *string
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return nil, nil, err
			}
			if name == "" {
				continue
			}
			k := strings.ToLower(name)
			if _, ok := owner[k]; ok {
				continue
			}
			owner[k] = table
			out = append(out, name)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, nil, err
		}
	}
	return out, owner, nil
}

func suggestColumn(unknown string, columns []string) string {
	if unknown == "" || len(columns) == 0 {
		return ""
	}
	lower := strings.ToLower(unknown)
	for _, c := range columns {
		if strings.EqualFold(c, unknown) {
			return ""
		}
	}
	// Prefer "<prefix>_<column>" over raw edit distance so issue_key → key
	// wins instead of issue_type (which is closer by Levenshtein).
	best := ""
	for _, c := range columns {
		if strings.HasSuffix(lower, "_"+strings.ToLower(c)) && len(c) > len(best) {
			best = c
		}
	}
	if best != "" {
		return best
	}
	bestDist := math.MaxInt
	winner := ""
	ties := 0
	for _, c := range columns {
		d := levenshtein(lower, strings.ToLower(c))
		if d < bestDist {
			bestDist = d
			winner = c
			ties = 1
		} else if d == bestDist {
			ties++
		}
	}
	if ties != 1 || !distanceOK(bestDist, len(unknown)) {
		return ""
	}
	return winner
}

func distanceOK(dist, nameLen int) bool {
	if dist == 1 {
		return true
	}
	if dist == 2 && nameLen >= 6 {
		return true
	}
	return false
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return len(b)
	}
	if b == "" {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = min(del, ins, sub)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
