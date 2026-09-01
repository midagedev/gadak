package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

// Replacing a workspace's origin — standalone → a real Jira site (GDK-241,
// GDK-344) — is not a cache invalidation. An issue key is not globally unique:
// `init --standalone` seeds project STD and a real site's project can be called
// STD too, so a personal row naming STD-1 does not go stale when the origin
// changes. It silently rebinds to a different issue that happens to share the
// key, which is worse than losing it.
//
// The set of tables that must go used to be a literal list inside
// DropSourceMirror, and that is exactly why GDK-418 existed: enrichments
// (schemaV2), feed_reads (schemaV4), field_usage (schemaV7) and sync_runs
// (schemaV8) were each added by a migration written after that list, and not
// one of them was added to it. Four more DELETEs would fix today's four and
// leave the next migration to repeat it. So the list is derived from a
// classification every table must appear in, and TestEveryTableIsOriginScoped
// enumerates sqlite_master to fail on a table missing from it — the decision is
// now something a new table cannot skip rather than something an author has to
// remember.
type originScope int

const (
	// scopeMirror is origin content, or a projection of it that the `sources`
	// cascade takes with it. The mirror is disposable by contract
	// (specs/000-product/data-model.md).
	scopeMirror originScope = iota
	// scopeDerived is ours, but every row names something the origin minted — a
	// key, a project, a source id, an account id. It dies with the origin.
	scopeDerived
	// scopeAuthored is what the user wrote here, that no origin has a copy of.
	// data-model.md names saved views as a deliberate exception to "gadak keeps
	// no originals"; conversion reports them, never deletes them.
	scopeAuthored
	// scopeLocal is about this installation rather than the origin: our own
	// counters, and history the product invariant protects. Survives. History
	// is additionally epoch-scoped (see localSchemaV3) so it stops resolving
	// against a new mirror without being destroyed.
	scopeLocal
)

// tableRule is one table's answer to "the origin is being replaced".
//
// dropForSource and dropForOrigin each take at most one bind parameter (the
// source id) so the executor can stay a loop rather than a switch. Both empty
// means the rows survive, and then `why` is mandatory — a table that keeps its
// rows must say so out loud, or "we decided to keep it" and "we forgot it" read
// identically in a diff.
type tableRule struct {
	table         string
	scope         originScope
	dropForSource string
	dropForOrigin string
	why           string
}

// originScopedTables is every table in both schemas, in the order conversion
// must visit them: rows keyed into `items` before the cascade removes the items
// they name, and the contentless FTS index before its content rows.
var originScopedTables = []tableRule{
	// ── Rows keyed into the mirror by issue key. These must precede `sources`.
	// watches/favorites live in local.db (GDK-105) but the subquery reads the
	// mirror's items: both files sit on one ATTACHed connection, so the
	// cross-file DELETE works — and still has to run before the cascade
	// removes the rows it reads.
	{table: "watches", scope: scopeDerived,
		dropForSource: `DELETE FROM local.watches WHERE key IN (SELECT key FROM items WHERE source_id = ?)`},
	{table: "favorites", scope: scopeDerived,
		dropForSource: `DELETE FROM local.favorites WHERE key IN (SELECT key FROM items WHERE source_id = ?)`},
	// enrichments are written by plugins, not by us (data-model.md), but the
	// payload describes one mirror row and is addressed by its key.
	{table: "enrichments", scope: scopeDerived,
		dropForSource: `DELETE FROM enrichments WHERE key IN (SELECT key FROM items WHERE source_id = ?)`},

	// ── The mirror spine and everything the cascade reaches.
	{table: "items_fts", scope: scopeMirror,
		dropForSource: `DELETE FROM items_fts WHERE rowid IN (SELECT rowid FROM items WHERE source_id = ?)`,
		why:           "contentless fts5 has no triggers: clear it before its content rows go"},
	{table: "sources", scope: scopeMirror,
		dropForSource: `DELETE FROM sources WHERE id = ?`,
		why:           "the cascade root"},
	{table: "items", scope: scopeMirror, why: "cascades from sources"},
	{table: "issues_raw", scope: scopeMirror, why: "cascades from items"},
	{table: "pages", scope: scopeMirror, why: "cascades from items"},
	{table: "comments", scope: scopeMirror, why: "cascades from items"},
	{table: "dev_links", scope: scopeMirror, why: "cascades from items"},
	{table: "remote_links", scope: scopeMirror, why: "cascades from items"},
	{table: "attachments", scope: scopeMirror, why: "cascades from items"},
	{table: "changelog", scope: scopeMirror, why: "cascades from items"},
	{table: "links", scope: scopeMirror, why: "cascades from items"},
	{table: "item_refs", scope: scopeMirror, why: "cascades from items"},
	{table: "page_versions", scope: scopeMirror, why: "cascades from items"},
	{table: "source_queries", scope: scopeMirror, why: "cascades from sources"},

	// ── Per-source bookkeeping the cascade does not reach.
	{table: "deleted_items", scope: scopeMirror,
		dropForSource: `DELETE FROM deleted_items WHERE source_id = ?`},
	{table: "spaces", scope: scopeMirror,
		dropForSource: `DELETE FROM spaces WHERE source_id = ?`},
	{table: "sync_state", scope: scopeMirror,
		dropForSource: `DELETE FROM sync_state WHERE source_id = ?`},
	// status_catalog is the cached origin status catalog (GDK-591). Ids are
	// origin-minted; keeping them across a replacement rebinds history walks
	// onto the new origin's categories.
	{table: "status_catalog", scope: scopeMirror,
		dropForSource: `DELETE FROM status_catalog WHERE source_id = ?`},
	// users is the cached origin account catalog (GDK-590). account ids are
	// origin-minted; keeping them across a replacement would attribute the
	// new origin's history to the retired origin's accounts.
	{table: "users", scope: scopeMirror,
		dropForSource: `DELETE FROM users WHERE source_id = ?`},
	// sync_runs reuse the source ids `jira` and `confluence`, so the new
	// origin's first sync would appear beneath the retired origin's runs as if
	// one history. GDK-418: missed by the old list.
	{table: "sync_runs", scope: scopeDerived,
		dropForSource: `DELETE FROM sync_runs WHERE source_id = ?`},

	// ── Named by the origin, but not by one source of it.
	// Feed event ids are built from mirror identity ("cr:" + issue key, see
	// feed.go), so a read mark on cr:STD-1 makes the new site's STD-1 arrive
	// already read. There is no source column to scope by, and the events
	// themselves are recomputed from the mirror, so the marks go whole.
	// Rows live in local.db since GDK-105; deleting them whole is unchanged.
	{table: "feed_reads", scope: scopeDerived,
		dropForOrigin: `DELETE FROM local.feed_reads`},
	// field_usage is per (project_key, alias) of the retired origin's schema.
	{table: "field_usage", scope: scopeDerived,
		dropForOrigin: `DELETE FROM field_usage`},
	// versions is the project version catalog (GDK-532). No source_id — the
	// origin's version id is site-wide. dropForSource binds the source id, so
	// the predicate keeps Confluence-only drops from wiping Jira trains.
	{table: "versions", scope: scopeMirror,
		dropForSource: `DELETE FROM versions WHERE 'jira' = ?`,
		dropForOrigin: `DELETE FROM versions`,
		why:           "project version catalog; conversion must not rebind old trains onto a new origin's project keys"},
	// local.recents ranks pickers by ids the origin minted (account ids,
	// issue-type ids, transition ids). None of them exists on the new origin,
	// so keeping them offers the user dead options.
	{table: "recents", scope: scopeDerived,
		dropForOrigin: `DELETE FROM local.recents`},

	// ── Survivors.
	{table: "saved_views", scope: scopeAuthored,
		why: "authored here (local.db since GDK-105, surviving `rm gadak.db`); data-model.md's named exception to keeping no originals. " +
			"A view whose query names a retired project is reported by OriginReset, not deleted"},
	{table: "recipes", scope: scopeAuthored,
		why: "authored here (local.db, GDK-503); a name for mirror SQL the user wrote. " +
			"Not origin content. Kept on conversion the same way saved_views are"},
	{table: "dashboards", scope: scopeAuthored,
		why: "authored here (local.db, GDK-781); an HTML wall plus datasource " +
			"queries the user wrote. Not origin content — a dashboard's SQL names " +
			"the schema, not an origin. Kept on conversion the same way saved_views are"},
	{table: "api_usage", scope: scopeLocal,
		why: "our own outbound HTTP counters per day, not origin content (schemaV6)"},
	{table: "visits", scope: scopeLocal,
		why: "protected history; hidden from the timeline by origin_epoch instead of deleted"},
	{table: "searches", scope: scopeLocal,
		why: "protected history; hidden from the timeline by origin_epoch instead of deleted"},
	{table: "local_meta", scope: scopeLocal,
		why: "holds origin_epoch itself"},
}

// OriginReset is what a conversion took, so a surface can say it instead of
// leaving the user to discover an empty feed. Keyed by table rather than by
// field so a rule added later reports itself without a struct change.
type OriginReset struct {
	// Removed is table → rows deleted. Only tables whose rows named the
	// retired origin appear; the mirror's own cascade is not itemised.
	Removed map[string]int `json:"removed"`
	// RetiredHistory is visit and search rows still on disk but no longer in
	// the timeline: they name keys the new origin would resolve to other
	// issues. Readable with `gadak sql` (local.visits / local.searches).
	RetiredHistory int `json:"retired_history"`
	// OriginEpoch is the generation history is stamped with from now on.
	OriginEpoch int `json:"origin_epoch"`
	// SavedViews are authored views whose stored query mentions a project the
	// retired origin owned. Kept — the user wrote them — and named here,
	// because `project = STD` now means the new site's STD. A text match on
	// the opaque config: the web owns that shape, so this over-reports rather
	// than staying silent.
	SavedViews []string `json:"saved_views_naming_retired_projects"`
}

// currentEpochSQL is a scalar subquery rather than a value cached in Go. The
// epoch changes once per conversion, and a cache would need invalidating on
// every connection the driver hands out (store.Open, gadak sql, MCP, tests);
// an index probe per statement is the cheaper correctness.
const currentEpochSQL = `COALESCE((SELECT CAST(v AS INTEGER) FROM local.local_meta WHERE k = 'origin_epoch'), 0)`

// DropSourceMirror deletes everything one source mirrored: its items (and their
// cascaded children), FTS rows, spaces, tombstones, sync state and sync runs —
// so the next sync starts full against the new origin. Used when a standalone
// workspace converts to connected: a pre-namespace mirror holds `jira:N` /
// `confluence:N` rows the new site's upsert would silently overwrite on an id
// collision. The disposable cache is dropped whole rather than reconciled row by
// row, and the personal rows keyed into it go with it.
//
// This is the per-source half. ResetForNewOrigin is the whole conversion and is
// what the surfaces call; this stays exported for a single-source drop.
func (db *DB) DropSourceMirror(ctx context.Context, sourceID string) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		return dropSourceTx(tx, sourceID, nil)
	})
}

func dropSourceTx(tx *sql.Tx, sourceID string, removed map[string]int) error {
	for _, r := range originScopedTables {
		if r.dropForSource == "" {
			continue
		}
		res, err := tx.Exec(r.dropForSource, sourceID)
		if err != nil {
			return fmt.Errorf("drop %s for source %s: %w", r.table, sourceID, err)
		}
		if removed != nil && r.scope == scopeDerived {
			n, _ := res.RowsAffected()
			removed[r.table] += int(n)
		}
	}
	return nil
}

// ResetForNewOrigin is the one owner of "this workspace's origin is being
// replaced". Both conversion surfaces reach it through
// originbind.DropStandaloneProjection, so the decision cannot exist on the CLI
// path and not the HTTP one (GDK-247).
//
// One transaction: a conversion that dropped the mirror but left the feed marks
// behind would be the same class of half-state this function exists to remove.
func (db *DB) ResetForNewOrigin(ctx context.Context, sourceIDs []string) (OriginReset, error) {
	out := OriginReset{Removed: map[string]int{}}
	err := db.write(ctx, func(tx *sql.Tx) error {
		out = OriginReset{Removed: map[string]int{}}

		// Read before dropping: after the cascade there is no way to learn
		// which projects the retired origin owned.
		projects, err := retiredProjectsTx(tx, sourceIDs)
		if err != nil {
			return err
		}
		out.SavedViews, err = viewsNamingTx(tx, projects)
		if err != nil {
			return err
		}

		for _, src := range sourceIDs {
			if err := dropSourceTx(tx, src, out.Removed); err != nil {
				return err
			}
		}
		for _, r := range originScopedTables {
			if r.dropForOrigin == "" {
				continue
			}
			res, err := tx.Exec(r.dropForOrigin)
			if err != nil {
				return fmt.Errorf("drop %s for origin: %w", r.table, err)
			}
			n, _ := res.RowsAffected()
			out.Removed[r.table] += int(n)
		}

		// History is retired, not deleted: bump the epoch, and every row
		// written under the old one drops out of the timeline while staying
		// readable through gadak sql.
		if err := tx.QueryRow(`SELECT
			(SELECT count(*) FROM local.visits   WHERE origin_epoch = ` + currentEpochSQL + `) +
			(SELECT count(*) FROM local.searches WHERE origin_epoch = ` + currentEpochSQL + `)`).
			Scan(&out.RetiredHistory); err != nil {
			return fmt.Errorf("count retiring history: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO local.local_meta (k, v) VALUES ('origin_epoch', '1')
			ON CONFLICT(k) DO UPDATE SET v = CAST(local_meta.v AS INTEGER) + 1`); err != nil {
			return fmt.Errorf("bump origin epoch: %w", err)
		}
		if err := tx.QueryRow(`SELECT ` + currentEpochSQL).Scan(&out.OriginEpoch); err != nil {
			return fmt.Errorf("read origin epoch: %w", err)
		}
		for t, n := range out.Removed {
			if n == 0 {
				delete(out.Removed, t)
			}
		}
		return nil
	})
	if err != nil {
		return OriginReset{Removed: map[string]int{}}, err
	}
	return out, nil
}

func retiredProjectsTx(tx *sql.Tx, sourceIDs []string) ([]string, error) {
	if len(sourceIDs) == 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(sourceIDs)), ",")
	args := make([]any, len(sourceIDs))
	for i, s := range sourceIDs {
		args[i] = s
	}
	rows, err := tx.Query(`SELECT DISTINCT i.project_key FROM issues i
		JOIN items it ON it.id = i.item_id
		WHERE it.source_id IN (`+marks+`) AND i.project_key <> ''`, args...)
	if err != nil {
		return nil, fmt.Errorf("read retired projects: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// viewsNamingTx returns the names of saved views whose stored config mentions
// one of these project keys as a whole token. The config is opaque JSON owned
// by the web, so this deliberately errs towards naming a view that turns out to
// be fine: the alternative is a view that silently reads a stranger's project.
func viewsNamingTx(tx *sql.Tx, projects []string) ([]string, error) {
	if len(projects) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(`SELECT name, config FROM local.saved_views ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("read saved views: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		var cfg []byte
		if err := rows.Scan(&name, &cfg); err != nil {
			return nil, err
		}
		for _, p := range projects {
			if mentionsToken(string(cfg), p) {
				out = append(out, name)
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// mentionsToken reports whether s contains tok bounded by non-alphanumerics, so
// project STD matches `project = STD` and `STD-14` but not `STDIO`.
func mentionsToken(s, tok string) bool {
	alnum := func(b byte) bool {
		return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	for i := 0; ; {
		j := strings.Index(s[i:], tok)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(tok)
		beforeOK := start == 0 || !alnum(s[start-1])
		afterOK := end == len(s) || !alnum(s[end])
		if beforeOK && afterOK {
			return true
		}
		i = start + 1
	}
}

// String renders the reset for a human: one line, empty when nothing went.
func (r OriginReset) String() string {
	var parts []string
	keys := slices.Sorted(maps.Keys(r.Removed))
	for _, t := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", t, r.Removed[t]))
	}
	if r.RetiredHistory > 0 {
		parts = append(parts, fmt.Sprintf("history retired %d (kept, readable with gadak sql)", r.RetiredHistory))
	}
	if len(r.SavedViews) > 0 {
		parts = append(parts, fmt.Sprintf("saved views naming a retired project: %s", strings.Join(r.SavedViews, ", ")))
	}
	if len(parts) == 0 {
		return ""
	}
	return "dropped rows bound to the previous origin — " + strings.Join(parts, "; ")
}

// MarshalJSON keeps Removed non-nil so `--json` never emits a bare null for a
// field a caller iterates.
func (r OriginReset) MarshalJSON() ([]byte, error) {
	type alias OriginReset
	a := alias(r)
	if a.Removed == nil {
		a.Removed = map[string]int{}
	}
	if a.SavedViews == nil {
		a.SavedViews = []string{}
	}
	return json.Marshal(a)
}
