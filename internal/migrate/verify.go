package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

// VerifyRow is one line of the migration report: a metric counted on the
// source mirror at export time against the same metric on the freshly
// filled target mirror.
type VerifyRow struct {
	Metric   string `json:"metric"`
	Source   int    `json:"source"`
	Migrated int    `json:"migrated"`
	// Skipped is the part of Migrated that was already at the target
	// before this run (the Linear path's idempotency scan); zero on the
	// built-in-tracker path, where the target is always new.
	Skipped int `json:"skipped,omitempty"`
}

// VerifyMirror re-counts the exported axes on the target mirror. The
// derived rows (reopens, epic keys) matter most: they are never stored in
// the fixture, so equality proves the migrated changelog reproduces them.
func VerifyMirror(ctx context.Context, db *sql.DB, st *Stats) ([]VerifyRow, error) {
	marks, args := inClause(st.Projects)
	issueJoin := ` FROM %s x JOIN issues_full i ON i.item_id = x.item_id WHERE i.project_key IN (` + marks + `)`

	counts := []struct {
		metric string
		source int
		query  string
	}{
		{"issues", st.Issues, `SELECT COUNT(*) FROM issues_full WHERE project_key IN (` + marks + `)`},
		{"comments", st.Comments, fmt.Sprintf(`SELECT COUNT(*)`+issueJoin, "comments")},
		{"attachments", st.Attachments, fmt.Sprintf(`SELECT COUNT(*)`+issueJoin, "attachments")},
		{"links", st.Links, fmt.Sprintf(`SELECT COUNT(*)`+issueJoin, "links")},
		{"history", st.History, fmt.Sprintf(`SELECT COUNT(*)`+issueJoin, "changelog")},
		{"reopens", st.ReopenSum, `SELECT COALESCE(SUM(reopen_count),0) FROM issues_full WHERE project_key IN (` + marks + `)`},
		{"epic keys", st.EpicKeys, `SELECT COUNT(*) FROM issues_full WHERE project_key IN (` + marks + `) AND COALESCE(epic_key,'') != ''`},
	}

	var out []VerifyRow
	for _, c := range counts {
		var n int
		if err := db.QueryRowContext(ctx, c.query, args...).Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, VerifyRow{Metric: c.metric, Source: c.source, Migrated: n})
	}

	if len(st.Spaces) > 0 {
		smarks, sargs := inClause(st.Spaces)
		var pages, pageComments int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pages WHERE space_key IN (`+smarks+`)`, sargs...).Scan(&pages); err != nil {
			return nil, err
		}
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM comments c JOIN pages p ON p.item_id = c.item_id
			WHERE p.space_key IN (`+smarks+`)`, sargs...).Scan(&pageComments); err != nil {
			return nil, err
		}
		out = append(out,
			VerifyRow{Metric: "pages", Source: st.Pages, Migrated: pages},
			VerifyRow{Metric: "page comments", Source: st.PageComments, Migrated: pageComments})
	}
	return out, nil
}
