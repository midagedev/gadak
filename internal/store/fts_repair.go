package store

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
)

// normalizeFTSDDL puts a CREATE statement into the form compared against the
// live schema: SQLite stores in sqlite_master what was executed (minus the
// trailing semicolon), and external writers format freely, so case and
// whitespace runs are ignored while every token must match.
func normalizeFTSDDL(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), ";"))), " ")
}

// repairItemsFTS closes GDK-112: a database can carry an items_fts whose DDL
// is not the shape this binary writes — the committed examples/demo.db
// snapshot is rebuilt without contentless_delete=1 for Datasette Lite
// portability (GDK-101), and any other externally-produced copy can differ the
// same way. Writes then die on the DELETE half of writeFTS ("cannot DELETE
// from contentless fts5 table") at the first changed issue of a sync, with
// nothing at Open time pointing at the cause.
//
// The index is a disposable cache over items/comments (the mirror contract),
// so the fix is to compare live DDL against itemsFTSCreate — the same
// statement schemaV1 creates the table with — and rebuild from the source
// tables when they disagree. The check is one sqlite_master row read on the
// happy path; rebuilding only ever runs on a diverged database.
func (db *DB) repairItemsFTS(ctx context.Context) error {
	var live sql.NullString
	err := db.sql.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'items_fts'`,
	).Scan(&live)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if live.Valid && normalizeFTSDDL(live.String) == normalizeFTSDDL(itemsFTSCreate) {
		return nil
	}
	rows, err := db.rebuildItemsFTS(ctx)
	if err != nil {
		return err
	}
	log.Printf("store: %s: rebuilt items_fts (%d rows): DDL did not match this build's schema", db.path, rows)
	return nil
}

// rebuildItemsFTS drops and recreates items_fts with the canonical DDL and
// reloads it with the same content writeFTS produces: title, body_text, and
// the item's comments joined by newlines in insertion order (rowid order —
// comments are replaced wholesale, so rowids follow insert order). The INSERT
// mirrors scripts/scrub-demo-db.py's rebuild_portable_fts, which was verified
// against this shape by MATCH-count probes.
func (db *DB) rebuildItemsFTS(ctx context.Context) (int64, error) {
	var rows int64
	err := db.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DROP TABLE IF EXISTS items_fts`); err != nil {
			return err
		}
		if _, err := tx.Exec(itemsFTSCreate); err != nil {
			return err
		}
		res, err := tx.Exec(`
			INSERT INTO items_fts (rowid, title, body_text, comments_text)
			SELECT i.rowid, COALESCE(i.title, ''), COALESCE(i.body_text, ''),
			       COALESCE((SELECT group_concat(body_text, char(10))
			                 FROM (SELECT body_text FROM comments
			                       WHERE item_id = i.id AND body_text <> ''
			                       ORDER BY rowid)), '')
			FROM items i`)
		if err != nil {
			return err
		}
		rows, err = res.RowsAffected()
		return err
	})
	return rows, err
}
