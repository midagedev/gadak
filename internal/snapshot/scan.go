package snapshot

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/midagedev/scry/internal/secretscan"
)

// tables scanned for credential-shaped strings (copy targets only).
var scanTables = []string{
	"sources", "items", "issues", "comments", "attachments", "changelog", "links", "sync_state",
}

// scanCredentials walks every TEXT column of the copy-target tables. On hit it
// returns an error naming table, row identity, and pattern — never the value.
func scanCredentials(db *sql.DB) error {
	for _, table := range scanTables {
		cols, err := textColumns(db, table)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			continue
		}
		// Prefer a stable row id column for diagnostics.
		idCol := rowIDColumn(table, cols)
		selectCols := append([]string{}, cols...)
		if idCol != "" {
			// ensure id is first for reporting
			seen := false
			for _, c := range selectCols {
				if c == idCol {
					seen = true
					break
				}
			}
			if !seen {
				selectCols = append([]string{idCol}, selectCols...)
			}
		}
		q := fmt.Sprintf("SELECT %s FROM %s", strings.Join(quoteIdents(selectCols), ", "), table)
		rows, err := db.Query(q)
		if err != nil {
			return fmt.Errorf("credential scan %s: %w", table, err)
		}
		for rows.Next() {
			vals := make([]sql.NullString, len(selectCols))
			ptrs := make([]any, len(selectCols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}
			rowID := "?"
			if idCol != "" {
				for i, c := range selectCols {
					if c == idCol && vals[i].Valid {
						rowID = vals[i].String
						break
					}
				}
			}
			for i, c := range selectCols {
				if !vals[i].Valid || vals[i].String == "" {
					continue
				}
				if name := secretscan.Match(vals[i].String); name != "" {
					rows.Close()
					return fmt.Errorf("credential-shaped string detected: table=%s row=%s column=%s pattern=%s",
						table, rowID, c, name)
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func textColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		// SQLite type affinity: treat anything declared TEXT (or empty/blob-as-text
		// storage that is still text-declared in our schema) as scannable.
		u := strings.ToUpper(typ)
		if u == "" || strings.Contains(u, "CHAR") || strings.Contains(u, "CLOB") || strings.Contains(u, "TEXT") {
			cols = append(cols, name)
		}
	}
	return cols, rows.Err()
}

func rowIDColumn(table string, cols []string) string {
	switch table {
	case "issues":
		return "item_id"
	case "links":
		return "item_id"
	case "sync_state":
		return "source_id"
	case "sources", "items", "comments", "attachments", "changelog":
		return "id"
	}
	for _, c := range cols {
		if c == "id" || c == "key" || c == "item_id" {
			return c
		}
	}
	if len(cols) > 0 {
		return cols[0]
	}
	return ""
}

func quoteIdents(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = `"` + strings.ReplaceAll(n, `"`, `""`) + `"`
	}
	return out
}
