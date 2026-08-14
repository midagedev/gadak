package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// SourceQuery is a named query mirrored from a connector (Jira saved filter).
// Jira is the record: ReplaceSourceQueries rewrites one source's rows.
type SourceQuery struct {
	ID          string          `json:"id"`
	SourceID    string          `json:"source_id"`
	ExternalID  string          `json:"external_id"`
	Name        string          `json:"name"`
	QueryText   string          `json:"query_text"`
	Config      json.RawMessage `json:"config"`
	Favourite   bool            `json:"favourite"`
	Owner       string          `json:"owner"`
	Applied     []string        `json:"applied"`
	Unsupported []string        `json:"unsupported"`
	UpdatedAt   string          `json:"updated_at"`
}

// SourceQueries returns one source's named queries, starred first then by name.
func (db *DB) SourceQueries(ctx context.Context, sourceID string) ([]SourceQuery, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, source_id, external_id, name, query_text, config, favourite, owner,
		       applied, unsupported, updated_at
		FROM source_queries
		WHERE source_id = ?
		ORDER BY favourite DESC, name COLLATE NOCASE`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceQuery{}
	for rows.Next() {
		var v SourceQuery
		var cfg, applied, unsupported string
		var owner, updated *string
		var fav int
		if err := rows.Scan(&v.ID, &v.SourceID, &v.ExternalID, &v.Name, &v.QueryText, &cfg,
			&fav, &owner, &applied, &unsupported, &updated); err != nil {
			return nil, err
		}
		v.Config = json.RawMessage(cfg)
		v.Favourite = fav != 0
		if owner != nil {
			v.Owner = *owner
		}
		if updated != nil {
			v.UpdatedAt = *updated
		}
		v.Applied = decodeStringList(applied)
		v.Unsupported = decodeStringList(unsupported)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReplaceSourceQueries swaps one source's named queries in a single write.
// An empty list clears that source (the account deleted every filter).
func (db *DB) ReplaceSourceQueries(ctx context.Context, sourceID string, queries []SourceQuery) error {
	now := Now()
	return db.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM source_queries WHERE source_id = ?`, sourceID); err != nil {
			return err
		}
		for _, q := range queries {
			if q.ID == "" || q.Name == "" {
				continue
			}
			if q.SourceID == "" {
				q.SourceID = sourceID
			}
			if len(q.Config) == 0 {
				q.Config = json.RawMessage(`{}`)
			}
			applied, _ := json.Marshal(q.Applied)
			if q.Applied == nil {
				applied = []byte("[]")
			}
			unsup, _ := json.Marshal(q.Unsupported)
			if q.Unsupported == nil {
				unsup = []byte("[]")
			}
			if _, err := tx.Exec(`
				INSERT INTO source_queries
				  (id, source_id, external_id, name, query_text, config, favourite, owner, applied, unsupported, updated_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
				q.ID, q.SourceID, q.ExternalID, q.Name, q.QueryText, string(q.Config),
				boolInt(q.Favourite), nz(q.Owner), string(applied), string(unsup), now,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func decodeStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
