package store

import (
	"context"
	"database/sql"
	"errors"
)

// PageVersion is one history stamp on a wiki page. Bodies are never stored —
// only the number, when, who, the editor's note, and the minor-edit flag.
// Field names match the page_versions columns, not the Confluence payload.
type PageVersion struct {
	Number     int
	CreatedAt  string
	AuthorID   string
	AuthorName string
	Message    string
	MinorEdit  bool
}

// ReplacePageVersions writes the complete stamp list for one item. The caller
// must have a successful full listing; an empty slice clears stored rows.
// Re-running with the same numbers is a no-op on cardinality (PK upsert).
func (db *DB) ReplacePageVersions(ctx context.Context, itemID string, vers []PageVersion) error {
	if itemID == "" {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM page_versions WHERE item_id = ?`, itemID); err != nil {
			return err
		}
		for _, v := range vers {
			if v.Number <= 0 {
				continue
			}
			minor := 0
			if v.MinorEdit {
				minor = 1
			}
			// created_at is nullable (unknown). The other text columns are
			// NOT NULL DEFAULT '' — empty string, never NULL.
			if _, err := tx.Exec(`
				INSERT INTO page_versions (
					item_id, number, created_at, author_id, author_name, message, minor_edit
				) VALUES (?,?,?,?,?,?,?)
				ON CONFLICT(item_id, number) DO UPDATE SET
					created_at = excluded.created_at,
					author_id = excluded.author_id,
					author_name = excluded.author_name,
					message = excluded.message,
					minor_edit = excluded.minor_edit`,
				itemID, v.Number, nz(v.CreatedAt), v.AuthorID, v.AuthorName, v.Message, minor,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// PageVersions returns stamps for itemID in number order. Missing item is empty.
func (db *DB) PageVersions(ctx context.Context, itemID string) ([]PageVersion, error) {
	if itemID == "" {
		return nil, nil
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT number, COALESCE(created_at, ''), author_id, author_name, message, minor_edit
		FROM page_versions
		WHERE item_id = ?
		ORDER BY number ASC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PageVersion
	for rows.Next() {
		var v PageVersion
		var minor int
		if err := rows.Scan(&v.Number, &v.CreatedAt, &v.AuthorID, &v.AuthorName, &v.Message, &minor); err != nil {
			return nil, err
		}
		v.MinorEdit = minor != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

// HasPageVersion reports whether a stamp for this version number is already
// stored. Sync uses it as the incremental gate: if the incoming page version
// is already on disk, history cannot have grown and must not be refetched.
func (db *DB) HasPageVersion(ctx context.Context, itemID string, number int) (bool, error) {
	if itemID == "" || number <= 0 {
		return false, nil
	}
	var n int
	err := db.sql.QueryRowContext(ctx,
		`SELECT 1 FROM page_versions WHERE item_id = ? AND number = ?`,
		itemID, number).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
