// Package parenthint is the single owner of parent-rejection detection and
// the mirror hierarchy hint that follows it (CLI create/edit and REST
// PUT parent / POST create). Both surfaces call Wrap so a new path cannot
// inherit the bare origin 400.
//
// The field key differs by verb, and both shapes were measured against a
// real Cloud site on 2026-08-21: POST /issue answers with `parent` AND
// `parentId`, PUT /issue/{key} answers with `pid`. The messages themselves
// are localized per account, so the keys are the only stable part.
// GDK-424 tested `parent` inline in the create path, which could never
// have matched the edit path (GDK-525).
//
// The hint names CLI --parent because that wording is the existing
// contract (GDK-330); REST appends the same sentence rather than a
// second copy. This package does not compose HTTP status.
package parenthint

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Querier is the mirror read the hint needs. *sql.DB (CLI openReadOnly)
// and *store.DB (REST's existing connection) both satisfy it; this
// package never opens a connection of its own.
type Querier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// Hinted wraps an origin error with the mirror hierarchy sentence.
// Unwrap keeps the origin error matchable; Error appends the hint the
// same way the CLI used to (origin + newline + hint).
type Hinted struct {
	err  error
	Hint string
}

func (h *Hinted) Error() string {
	if h == nil {
		return ""
	}
	if h.err == nil {
		return h.Hint
	}
	if h.Hint == "" {
		return h.err.Error()
	}
	return h.err.Error() + "\n" + h.Hint
}

func (h *Hinted) Unwrap() error {
	if h == nil {
		return nil
	}
	return h.err
}

// Rejection reports whether err is the origin refusing the parent we sent.
func Rejection(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// "pid:" keeps the colon so ordinary words containing "pid" (rapid,
	// insipid) in an unrelated message cannot claim to be this rejection.
	return strings.Contains(s, "parent") || strings.Contains(s, "pid:")
}

// Wrap appends the mirror's hierarchy answer to a parent rejection and
// returns every other error untouched. q may be nil: the origin error
// then stands alone (best-effort; no mirror, no row).
func Wrap(err error, parentKey string, q Querier) error {
	if err == nil || parentKey == "" || !Rejection(err) {
		return err
	}
	if hint := Hint(q, parentKey); hint != "" {
		return &Hinted{err: err, Hint: hint}
	}
	return err
}

// Hint reads the rejected parent from the mirror and states the
// hierarchy rule Jira's 400 leaves out. Best-effort: no querier, no row,
// or any error returns "" and the origin error stands alone.
func Hint(q Querier, parentKey string) string {
	if q == nil || parentKey == "" {
		return ""
	}
	var issueType, projectKey string
	var level int
	err := q.QueryRow(
		`SELECT COALESCE(issue_type, ''), COALESCE(hierarchy_level, 0), COALESCE(project_key, '') FROM issues WHERE key = ?`,
		parentKey).Scan(&issueType, &level, &projectKey)
	if err != nil {
		return ""
	}
	// A parent sits exactly one level above its child, so a level-1 parent
	// (an epic) is refused for another epic — the case the old early return
	// answered with silence (GDK-525).
	if level >= 1 {
		below := fmt.Sprintf("level-%d", level-1)
		if level == 1 {
			below = "level-0 (standard types such as Task, Bug or Story)"
		}
		return fmt.Sprintf("hint: %s is %q (hierarchy level %d) — a parent sits exactly one level above its child, so %s can only parent %s issues. Two issues at the same level cannot be parent and child.",
			parentKey, issueType, level, parentKey, below)
	}
	hint := fmt.Sprintf("hint: %s is %q (hierarchy level %d) — a standard issue can only sit under a level-1 parent (an epic); only sub-task types can sit under %s. Pick an epic as --parent, or use a sub-task issue type.",
		parentKey, issueType, level, parentKey)
	if extra := openEpicHint(q, projectKey); extra != "" {
		return hint + "\n" + extra
	}
	return hint
}

// openEpicHint names up to three open level-1 issues in the rejected parent's
// project. Empty project, query failure, or zero rows return "" so the base
// hint still stands. Filter is hierarchy_level + status_category, never a
// localized type name (GDK-330).
func openEpicHint(q Querier, projectKey string) string {
	if q == nil || projectKey == "" {
		return ""
	}
	rows, err := q.Query(
		`SELECT key, COALESCE(summary, '') FROM issues_full
		 WHERE hierarchy_level = 1 AND status_category != 'done' AND project_key = ?
		 ORDER BY updated_at DESC LIMIT 3`,
		projectKey)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var key, summary string
		if err := rows.Scan(&key, &summary); err != nil {
			return ""
		}
		parts = append(parts, fmt.Sprintf("%s %q", key, clip(summary, 60)))
	}
	if err := rows.Err(); err != nil || len(parts) == 0 {
		return ""
	}
	return "open epics in " + projectKey + ": " + strings.Join(parts, ", ")
}

// clip flattens a value onto one line and cuts it to a column budget. Columns,
// not runes: a Hangul or CJK rune occupies two cells, so a rune-counted cut
// renders twice as wide as the same cut in ASCII.
func clip(s string, cols int) string {
	s = strings.Join(strings.Fields(s), " ")
	if runewidth.StringWidth(s) <= cols {
		return s
	}
	return runewidth.Truncate(s, cols, "…")
}
