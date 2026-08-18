package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

// GroupQueryHits is the result of Config.GroupQuery. Keys not in the map
// fall through to groupRules / assignee. A present empty string means
// unclassified (stop).
func (db *DB) GroupQueryHits(ctx context.Context, query string) (map[string]string, error) {
	if err := config.ValidateGroupQuery(query); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(query), ";"))
	if query == "" {
		return nil, nil
	}
	rows, err := db.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("groupQuery: %w", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(cols) < 2 {
		return nil, fmt.Errorf("groupQuery must return at least two columns (key, group), got %d", len(cols))
	}
	out := map[string]string{}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		key := scanText(raw[0])
		if key == "" {
			continue
		}
		if raw[1] == nil {
			continue
		}
		out[key] = scanText(raw[1])
	}
	return out, rows.Err()
}

func scanText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case sql.NullString:
		if !t.Valid {
			return ""
		}
		return t.String
	default:
		return fmt.Sprint(t)
	}
}
