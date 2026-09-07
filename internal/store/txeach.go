package store

import "database/sql"

// txEach is each (read.go) for a transaction: the flow layer's walks run
// inside db.write, where the handle is a *sql.Tx. ctx stays implicit — the
// transaction already carries it (BeginTx in writeOnce), which is why the
// callers' own loops never passed one either. Same shape as each so a loop
// reads identically on either handle; the manual rows.Close() / rows.Err()
// ritual this file's callers used to hand-copy is the class this closes
// (GDK-1576).
func txEach(tx *sql.Tx, query string, scan func(*sql.Rows) error, args ...any) error {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
