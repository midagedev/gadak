package store

import "database/sql"

// PoolStats reports the connection pool's state. A caller that has finished
// with a mirror can assert nothing is still checked out: a connection that
// outlives Close belongs to something still running, and under WAL it writes
// journal files back into a directory the caller may already have removed
// (GDK-270).
func (db *DB) PoolStats() sql.DBStats { return db.sql.Stats() }
