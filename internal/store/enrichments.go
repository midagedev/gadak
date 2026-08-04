package store

import (
	"database/sql"
	"encoding/json"
)

// Enrichments are rows an external plugin writes with raw SQL; the server merges
// them into its responses. Only the read side lives here — a Go write API would
// be a second, privileged path into a table whose whole point is that any
// process in any language can fill it (data-model.md, `enrichments`).

// EnrichmentsByKind returns every payload of one kind, keyed by issue key. The
// list and delta responses merge it in.
func (db *DB) EnrichmentsByKind(kind string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	return out, each(db.sql, `SELECT key, payload FROM enrichments WHERE kind = ?`,
		func(rows *sql.Rows) error {
			var key, payload string
			if err := rows.Scan(&key, &payload); err != nil {
				return err
			}
			out[key] = json.RawMessage(payload)
			return nil
		}, kind)
}

// EnrichmentsFor returns every kind attached to one issue, for the detail view.
func (db *DB) EnrichmentsFor(key string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	return out, each(db.sql, `SELECT kind, payload FROM enrichments WHERE key = ?`,
		func(rows *sql.Rows) error {
			var kind, payload string
			if err := rows.Scan(&kind, &payload); err != nil {
				return err
			}
			out[kind] = json.RawMessage(payload)
			return nil
		}, key)
}
