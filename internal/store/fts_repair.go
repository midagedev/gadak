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
// reloads it with the same content writeFTS produces: title, body_text, the
// item's comments joined by newlines in insertion order (rowid order —
// comments are replaced wholesale, so rowids follow insert order), and the
// CJK bigram column. SQL cannot emit overlapping 2-grams, so rows are walked
// in Go (insertFTSBatch); the walk pages by items.rowid so a large mirror
// rebuilds without holding every body in memory. The comment concatenation
// still mirrors scripts/scrub-demo-db.py's rebuild_portable_fts, verified
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
		const batch = 500
		var after int64
		for {
			n, last, err := insertFTSBatch(ctx, tx, after, batch)
			if err != nil {
				return err
			}
			rows += int64(n)
			if n < batch {
				return nil
			}
			after = last
		}
	})
	return rows, err
}

// insertFTSBatch loads up to batch items with rowid > after (rowid order),
// closes the read, then inserts each row with the same four content columns
// writeFTS writes. Returns the rows written and the highest rowid seen, which
// the caller uses as the next page cursor.
func insertFTSBatch(ctx context.Context, tx *sql.Tx, after int64, batch int) (n int, last int64, err error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT i.rowid, COALESCE(i.title, ''), COALESCE(i.body_text, ''),
		       COALESCE((SELECT group_concat(body_text, char(10))
		                 FROM (SELECT body_text FROM comments
		                       WHERE item_id = i.id AND body_text <> ''
		                       ORDER BY rowid)), '')
		FROM items i
		WHERE i.rowid > ?
		ORDER BY i.rowid
		LIMIT ?`, after, batch)
	if err != nil {
		return 0, 0, err
	}
	type srcRow struct {
		rowid              int64
		title, body, comms string
	}
	var loaded []srcRow
	for rows.Next() {
		var r srcRow
		if err := rows.Scan(&r.rowid, &r.title, &r.body, &r.comms); err != nil {
			rows.Close()
			return 0, 0, err
		}
		loaded = append(loaded, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()

	ins, err := tx.PrepareContext(ctx,
		`INSERT INTO items_fts (rowid, title, body_text, comments_text, cjk_bigram) VALUES (?,?,?,?,?)`)
	if err != nil {
		return 0, 0, err
	}
	defer ins.Close()
	for _, r := range loaded {
		if _, err := ins.Exec(r.rowid, r.title, r.body, r.comms, FTSCJKBigramColumn(r.title, r.body, r.comms)); err != nil {
			return n, last, err
		}
		n++
		last = r.rowid
	}
	return n, last, nil
}
