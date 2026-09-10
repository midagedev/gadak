package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"

	sqlite "modernc.org/sqlite"
)

// identityMatch is the "is this person me?" predicate, in SQL, once.
//
// It is the same rule web/src/lib/person-match.ts applies and for the same
// reason: id first because it never changes, then a case-insensitive email
// because a site that hides emails leaves one side blank, then the built-in
// tracker's actor slug — which is stamped into the id column, not an email.
// Empty on either side never matches, which is why localSchemaV8 stores ”
// and not NULL: `” = ”` would make every no-credential workspace match
// every unassigned issue.
//
// %s is the issues_full column prefix for the role being tested (assignee /
// reporter). Kept as one string so a change to the rule cannot land on one
// role and miss the other.
const identityMatch = `(
    (m.account_id <> '' AND i.%[1]s_id = m.account_id)
 OR (m.email <> '' AND i.%[1]s_email <> '' AND lower(i.%[1]s_email) = lower(m.email))
 OR (m.actor_slug <> '' AND i.%[1]s_id = m.actor_slug)
)`

// identityViews are the two questions an agent should not have to hand-write
// a `:me` substitution for (GDK-1438).
//
// They are TEMP views, not views inside local.db, because SQLite forbids a
// view from referencing another database — a view stored in local.db cannot
// read main.issues_full, and one stored in the mirror cannot read local.me
// (measured: "view my_open cannot reference objects in database main"). The
// temp schema has no such rule, so the views are created per connection by
// the same hook that owns the ATTACH. That also means they cost nothing on a
// mirror that is never queried through them, and that dropping local.db does
// not leave a broken view behind in the mirror the product calls a cache.
//
// my_open is "assigned to me and not finished": status_category, never a
// status display name, which localizes per account and silently returns zero
// rows. handed_off is the delegation ledger — reported by me, held by
// somebody else or nobody — the same delegatedBy() the web filters use.
//
// SELECT i.* keeps the views' columns exactly issues_full's, so everything
// RECIPES teaches about issues_full is true of these too.
var identityViews = []string{
	`CREATE TEMP VIEW IF NOT EXISTS my_open AS
SELECT i.* FROM issues_full i, local.me m
WHERE i.status_category <> 'done' AND ` + fmtRole(identityMatch, "assignee"),

	`CREATE TEMP VIEW IF NOT EXISTS handed_off AS
SELECT i.* FROM issues_full i, local.me m
WHERE ` + fmtRole(identityMatch, "reporter") + ` AND NOT ` + fmtRole(identityMatch, "assignee"),
}

// fmtRole substitutes the role prefix into identityMatch. A tiny helper
// rather than fmt.Sprintf at package level so the views stay `var` literals
// a reader can grep for whole.
func fmtRole(tmpl, role string) string {
	return strings.ReplaceAll(strings.ReplaceAll(tmpl, "%[1]s", role), "%s", role)
}

// dropIdentityViews / createIdentityViewsTx are migrate()'s pair. A temp
// view is re-validated whenever the schema it reads changes, so a connection
// that carries these views into a pending migration dies on the first
// `DROP VIEW issues_full` a migration performs (measured: migration 41,
// "error in view my_open: no such table: issues_full"). migrate() therefore
// takes them down for the duration and puts them back inside the same
// transaction — the same connection, which is what makes it airtight where a
// pool-level Exec would only be likely.
func dropIdentityViews(tx *sql.Tx) error {
	for _, name := range identityViewNames {
		if _, err := tx.Exec(`DROP VIEW IF EXISTS temp.` + name); err != nil {
			return err
		}
	}
	return nil
}

func createIdentityViewsTx(tx *sql.Tx) error {
	for _, stmt := range identityViews {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// identityViewNames is the same list as identityViews, for the drop side.
var identityViewNames = []string{"my_open", "handed_off"}

// createIdentityViews installs the temp views on a freshly attached
// connection — but only on a mirror this build has finished migrating, for
// the reason dropIdentityViews explains. A connection opened onto a mirror
// that still needs migrating gets its views from migrate() instead.
//
// Failures are silent by design, exactly like the ATTACH they follow: a
// connection whose main database is not a mirror (a snapshot, a scratch db)
// must still be usable, and the only cost of a missing view is the error the
// query itself reports.
func createIdentityViews(conn sqlite.ExecQuerierContext) {
	if !identityViewsSafe(conn) {
		return
	}
	for _, stmt := range identityViews {
		if _, err := conn.ExecContext(context.Background(), stmt, []driver.NamedValue{}); err != nil {
			return
		}
	}
}

// identityViewsSafe reports whether main is at a migration level that will
// not move under this connection. len(migrations) is the normal answer;
// personalStateCopyVersion-1 is migrate()'s documented resting level when
// local.db could not be read, and a mirror parked there is equally settled.
func identityViewsSafe(conn sqlite.ExecQuerierContext) bool {
	rows, err := conn.QueryContext(context.Background(), "PRAGMA user_version", []driver.NamedValue{})
	if err != nil {
		return false
	}
	defer rows.Close()
	dest := make([]driver.Value, 1)
	if err := rows.Next(dest); err != nil {
		return false
	}
	v, ok := dest[0].(int64)
	if !ok {
		return false
	}
	return int(v) == len(migrations) || int(v) == personalStateCopyVersion-1
}

// RecordWhoAmI stamps this workspace's identity into local.me (GDK-1438).
// The row is the one local.my_open / local.handed_off match against, so this
// is what makes "select * from my_open" answerable without a :me parameter.
//
// Idempotent and cheap: the common case is a rewrite of identical values.
// An empty identity is still written — clearing a credential must clear the
// answer, not leave the previous person's rows looking like mine.
func (db *DB) RecordWhoAmI(w config.WhoAmI) error {
	_, err := db.sql.Exec(
		`UPDATE local.me SET account_id = ?, email = ?, actor_slug = ?, resolved_at = ? WHERE id = 1`,
		w.AccountID, w.Email, w.ActorSlug, time.Now().UTC().Format(config.ISOMilli),
	)
	return err
}

// WhoAmI reads back the recorded identity. Used by the CLI's status surface
// and by tests; a missing row (a local.db older than V8, or one this build
// could not migrate) answers the zero value rather than an error, the same
// bias every other local.db read has.
func (db *DB) WhoAmI() config.WhoAmI {
	var w config.WhoAmI
	err := db.sql.QueryRow(`SELECT account_id, email, actor_slug FROM local.me WHERE id = 1`).
		Scan(&w.AccountID, &w.Email, &w.ActorSlug)
	if err != nil && err != sql.ErrNoRows {
		return config.WhoAmI{}
	}
	return w
}
