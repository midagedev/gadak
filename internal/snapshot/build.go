package snapshot

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

func buildInto(tmp string, opts Options) error {
	// Fresh schema via the same migration path as a live mirror.
	sdb, err := store.Open(tmp)
	if err != nil {
		return err
	}
	schemaVer := sdb.SchemaVersion()
	if err := sdb.Close(); err != nil {
		return err
	}

	src, err := openSQLite(opts.From, true)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := openSQLite(tmp, false)
	if err != nil {
		return err
	}
	defer dst.Close()

	// Prefer a single-file output.
	if _, err := dst.Exec(`PRAGMA journal_mode=DELETE`); err != nil {
		return err
	}
	if _, err := dst.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return err
	}

	tx, err := dst.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := copySources(src, tx, opts.Now); err != nil {
		return err
	}

	// Spaces are source metadata (no personal/credential payload beyond names).
	// Copy before pages so joins in readers always resolve when the source had them.
	if err := copySpaces(src, tx); err != nil {
		return err
	}

	issues, err := loadIssues(src)
	if err != nil {
		return err
	}

	// Stable order: created_at, then key.
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].createdAt != issues[j].createdAt {
			return issues[i].createdAt < issues[j].createdAt
		}
		return issues[i].key < issues[j].key
	})

	children, err := loadChildren(src)
	if err != nil {
		return err
	}

	// Item ids present in the destination (originals + pages). Used for links
	// and item_refs so clones never dangle and orphan refs are dropped.
	keptIDs := map[string]bool{}

	// Expand to scale target by cycling originals. Empty source → no issues.
	planned := planIssues(issues, opts.Scale)
	applySpread(planned, opts.Spread, opts.Now)

	// Key allocator for clones.
	nextNum := maxKeyNums(issues)

	for _, p := range planned {
		itemID := p.src.itemID
		key := p.src.key
		if p.cloneSeq > 0 {
			itemID = fmt.Sprintf("snap:clone:%d", p.cloneSeq)
			key = nextKey(p.src.projectKey, p.src.key, nextNum)
		} else {
			keptIDs[itemID] = true
		}
		if err := insertIssueBundle(tx, p, itemID, key, children); err != nil {
			return fmt.Errorf("write %s: %w", key, err)
		}
	}

	// Original links only (clone links skipped — avoids dangling target_keys).
	if err := copyOriginalLinks(src, tx, planned); err != nil {
		return err
	}

	// Documents: kind=page items + pages projection + their comments.
	// No scale/clone (scale is an issue-volume tool); timestamps kept as source.
	pages, err := loadPages(src)
	if err != nil {
		return err
	}
	sort.SliceStable(pages, func(i, j int) bool {
		if pages[i].createdAt != pages[j].createdAt {
			return pages[i].createdAt < pages[j].createdAt
		}
		return pages[i].key < pages[j].key
	})
	for _, p := range pages {
		keptIDs[p.itemID] = true
		if err := insertPageBundle(tx, p, children); err != nil {
			return fmt.Errorf("write page %s: %w", p.key, err)
		}
	}

	// Cross-refs only for items that actually landed (originals + all pages).
	if err := copyItemRefs(src, tx, keptIDs); err != nil {
		return err
	}

	if err := writeSyncState(tx, src, schemaVer); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Ensure no WAL leftovers for a clean rename.
	_, _ = dst.Exec(`PRAGMA journal_mode=DELETE`)
	return nil
}

func copySources(src *sql.DB, tx *sql.Tx, now time.Time) error {
	rows, err := src.Query(`SELECT id, kind, base_url, synced_at FROM sources`)
	if err != nil {
		return err
	}
	defer rows.Close()
	nowStr := formatTime(now)
	for rows.Next() {
		var id, kind string
		var base, synced sql.NullString
		if err := rows.Scan(&id, &kind, &base, &synced); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO sources (id, kind, base_url, synced_at) VALUES (?,?,?,?)`,
			id, kind, nullStr(base), nowStr,
		); err != nil {
			return err
		}
	}
	return rows.Err()
}

func writeSyncState(tx *sql.Tx, src *sql.DB, schemaVer int) error {
	// One clean row per source that exists (or a single jira placeholder).
	rows, err := src.Query(`SELECT id FROM sources`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		ids = []string{"jira"}
	}
	for _, id := range ids {
		if _, err := tx.Exec(`
			INSERT INTO sync_state (
				source_id, watermark, version, last_full_sync_at, last_error,
				schema_version, first_sync_at, sync_count, last_notified_at
			) VALUES (?, NULL, 1, NULL, NULL, ?, NULL, 0, NULL)
			ON CONFLICT(source_id) DO UPDATE SET
				watermark = NULL, version = 1, last_full_sync_at = NULL,
				last_error = NULL, schema_version = excluded.schema_version,
				first_sync_at = NULL, sync_count = 0, last_notified_at = NULL`,
			id, schemaVer,
		); err != nil {
			return err
		}
	}
	return nil
}

func loadIssues(src *sql.DB) ([]issueRow, error) {
	itemCols, err := columnNames(src, "items")
	if err != nil {
		return nil, err
	}
	issueCols, err := columnNames(src, "issues")
	if err != nil {
		return nil, err
	}

	// Join items ↔ issues on item_id = id.
	q := fmt.Sprintf(`
		SELECT %s, %s
		FROM issues i
		JOIN items it ON it.id = i.item_id`,
		qualify(itemCols, "it", "it_"),
		qualify(issueCols, "i", "i_"),
	)
	rows, err := src.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allCols := make([]string, 0, len(itemCols)+len(issueCols))
	for _, c := range itemCols {
		allCols = append(allCols, "it_"+c)
	}
	for _, c := range issueCols {
		allCols = append(allCols, "i_"+c)
	}

	var out []issueRow
	for rows.Next() {
		vals, err := scanMap(rows, allCols)
		if err != nil {
			return nil, err
		}
		item := map[string]any{}
		issue := map[string]any{}
		for _, c := range itemCols {
			item[c] = vals["it_"+c]
		}
		for _, c := range issueCols {
			issue[c] = vals["i_"+c]
		}
		ir := issueRow{
			itemID:     asString(item["id"]),
			key:        asString(issue["key"]),
			projectKey: asString(issue["project_key"]),
			createdAt:  asString(issue["created_at"]),
			updatedAt:  asString(issue["updated_at"]),
			itemCols:   item,
			issueCols:  issue,
		}
		if ir.createdAt == "" {
			ir.createdAt = asString(item["created_at"])
		}
		if ir.updatedAt == "" {
			ir.updatedAt = asString(item["updated_at"])
		}
		out = append(out, ir)
	}
	return out, rows.Err()
}

func loadChildren(src *sql.DB) (children, error) {
	var c children
	var err error
	c.comments, err = loadTableMaps(src, "comments")
	if err != nil {
		return c, err
	}
	c.attachments, err = loadTableMaps(src, "attachments")
	if err != nil {
		return c, err
	}
	c.changelog, err = loadTableMaps(src, "changelog")
	if err != nil {
		return c, err
	}
	c.commentsBy = groupBy(c.comments, "item_id")
	c.attachmentsBy = groupBy(c.attachments, "item_id")
	c.changelogBy = groupBy(c.changelog, "item_id")
	return c, nil
}

func loadPages(src *sql.DB) ([]pageRow, error) {
	ok, err := tableExists(src, "pages")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	itemCols, err := columnNames(src, "items")
	if err != nil {
		return nil, err
	}
	pageCols, err := columnNames(src, "pages")
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`
		SELECT %s, %s
		FROM pages p
		JOIN items it ON it.id = p.item_id`,
		qualify(itemCols, "it", "it_"),
		qualify(pageCols, "p", "p_"),
	)
	rows, err := src.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allCols := make([]string, 0, len(itemCols)+len(pageCols))
	for _, c := range itemCols {
		allCols = append(allCols, "it_"+c)
	}
	for _, c := range pageCols {
		allCols = append(allCols, "p_"+c)
	}

	var out []pageRow
	for rows.Next() {
		vals, err := scanMap(rows, allCols)
		if err != nil {
			return nil, err
		}
		item := map[string]any{}
		page := map[string]any{}
		for _, c := range itemCols {
			item[c] = vals["it_"+c]
		}
		for _, c := range pageCols {
			page[c] = vals["p_"+c]
		}
		pr := pageRow{
			itemID:    asString(item["id"]),
			key:       asString(item["key"]),
			createdAt: asString(item["created_at"]),
			itemCols:  item,
			pageCols:  page,
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

func insertPageBundle(tx *sql.Tx, p pageRow, ch children) error {
	item := copyMap(p.itemCols)
	page := copyMap(p.pageCols)
	itemID := p.itemID

	// Destination may have columns the source lacks (body_adf, labels, excerpt).
	if _, ok := page["body_adf"]; !ok {
		page["body_adf"] = ""
	}
	if _, ok := page["labels"]; !ok {
		page["labels"] = "[]"
	}
	if _, ok := page["excerpt"]; !ok {
		page["excerpt"] = ""
	}
	if _, ok := page["parent_id"]; !ok {
		page["parent_id"] = ""
	}
	if _, ok := page["status"]; !ok {
		page["status"] = "current"
	}
	if page["version"] == nil {
		page["version"] = 1
	}

	if err := insertRow(tx, "items", itemColumns, item); err != nil {
		return err
	}
	if err := insertRow(tx, "pages", pageColumns, page); err != nil {
		return err
	}

	// Page comments live on the shared comments table (same as issues).
	comms := ch.commentsBy[itemID]
	for _, row := range comms {
		row = copyMap(row)
		row["item_id"] = itemID
		if err := insertRow(tx, "comments", commentColumns, row); err != nil {
			return err
		}
	}
	// Attachments metadata (no file bytes) — same policy as issue path.
	for _, row := range ch.attachmentsBy[itemID] {
		row = copyMap(row)
		row["item_id"] = itemID
		if err := insertRow(tx, "attachments", attachmentColumns, row); err != nil {
			return err
		}
	}

	// FTS — same contentless delete+insert path as issues / store.writeFTS.
	var rowid int64
	if err := tx.QueryRow(`SELECT rowid FROM items WHERE id = ?`, itemID).Scan(&rowid); err != nil {
		return err
	}
	bodies := make([]string, 0, len(comms))
	for _, row := range comms {
		if bt := asString(row["body_text"]); bt != "" {
			bodies = append(bodies, bt)
		}
	}
	title := asString(item["title"])
	body := asString(item["body_text"])
	if _, err := tx.Exec(`DELETE FROM items_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO items_fts (rowid, title, body_text, comments_text) VALUES (?,?,?,?)`,
		rowid, title, body, strings.Join(bodies, "\n"),
	); err != nil {
		return err
	}
	return nil
}

func copySpaces(src *sql.DB, tx *sql.Tx) error {
	ok, err := tableExists(src, "spaces")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	// Map-based like pages: the source may predate later columns (homepage_id
	// is v17), and asString turns an absent key into ''.
	maps, err := loadTableMaps(src, "spaces")
	if err != nil {
		return err
	}
	for _, m := range maps {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO spaces (source_id, key, name, kind, homepage_id) VALUES (?,?,?,?,?)`,
			asString(m["source_id"]), asString(m["key"]), asString(m["name"]),
			asString(m["kind"]), asString(m["homepage_id"]),
		); err != nil {
			return err
		}
	}
	return nil
}

func copyItemRefs(src *sql.DB, tx *sql.Tx, keptIDs map[string]bool) error {
	ok, err := tableExists(src, "item_refs")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := src.Query(`
		SELECT item_id, target_kind, target_key, via FROM item_refs
		ORDER BY item_id, target_kind, target_key`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, targetKind, targetKey, via string
		if err := rows.Scan(&itemID, &targetKind, &targetKey, &via); err != nil {
			return err
		}
		if !keptIDs[itemID] {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO item_refs (item_id, target_kind, target_key, via) VALUES (?,?,?,?)`,
			itemID, targetKind, targetKey, via,
		); err != nil {
			return err
		}
	}
	return rows.Err()
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&n)
	return n > 0, err
}

func insertIssueBundle(tx *sql.Tx, p plannedIssue, itemID, key string, ch children) error {
	item := copyMap(p.src.itemCols)
	issue := copyMap(p.src.issueCols)

	item["id"] = itemID
	item["key"] = key
	issue["item_id"] = itemID
	issue["key"] = key

	if p.useMap {
		item["created_at"] = p.itemCreatedAt
		item["updated_at"] = p.itemUpdatedAt
		item["synced_at"] = p.itemSyncedAt
		issue["created_at"] = p.createdAt
		issue["updated_at"] = p.updatedAt
		if p.statusChangedAt != "" {
			issue["status_changed_at"] = p.statusChangedAt
		} else if asString(issue["status_changed_at"]) != "" {
			issue["status_changed_at"] = mapOrEven(asString(issue["status_changed_at"]), &p, 0, 1)
		}
		if p.resolvedAt != "" {
			issue["resolved_at"] = p.resolvedAt
		} else if asString(issue["resolved_at"]) != "" {
			issue["resolved_at"] = mapOrEven(asString(issue["resolved_at"]), &p, 0, 1)
		}
		if p.reopenedAt != "" {
			issue["reopened_at"] = p.reopenedAt
		} else if asString(issue["reopened_at"]) != "" {
			issue["reopened_at"] = mapOrEven(asString(issue["reopened_at"]), &p, 0, 1)
		}
		if p.assigneeChangedAt != "" {
			issue["assignee_changed_at"] = p.assigneeChangedAt
		} else if asString(issue["assignee_changed_at"]) != "" {
			issue["assignee_changed_at"] = mapOrEven(asString(issue["assignee_changed_at"]), &p, 0, 1)
		}
	}

	// Destination may have columns the source lacks (reopen_reason, cloned_from).
	if _, ok := issue["reopen_reason"]; !ok {
		issue["reopen_reason"] = ""
	}
	if _, ok := issue["cloned_from"]; !ok {
		issue["cloned_from"] = ""
	}

	if err := insertRow(tx, "items", itemColumns, item); err != nil {
		return err
	}
	if err := insertRow(tx, "issues", issueColumns, issue); err != nil {
		return err
	}

	// Children.
	srcID := p.src.itemID
	comms := ch.commentsBy[srcID]
	for i, row := range comms {
		row = copyMap(row)
		if p.cloneSeq > 0 {
			row["id"] = fmt.Sprintf("snap:clone:%d:c:%v", p.cloneSeq, row["id"])
		}
		row["item_id"] = itemID
		if p.useMap {
			if asString(row["created_at"]) != "" {
				row["created_at"] = mapOrEven(asString(row["created_at"]), &p, i, len(comms))
			}
			if asString(row["updated_at"]) != "" {
				row["updated_at"] = mapOrEven(asString(row["updated_at"]), &p, i, len(comms))
			}
		}
		if err := insertRow(tx, "comments", commentColumns, row); err != nil {
			return err
		}
	}
	atts := ch.attachmentsBy[srcID]
	for i, row := range atts {
		row = copyMap(row)
		if p.cloneSeq > 0 {
			row["id"] = fmt.Sprintf("snap:clone:%d:a:%v", p.cloneSeq, row["id"])
		}
		row["item_id"] = itemID
		if p.useMap && asString(row["created_at"]) != "" {
			row["created_at"] = mapOrEven(asString(row["created_at"]), &p, i, len(atts))
		}
		if err := insertRow(tx, "attachments", attachmentColumns, row); err != nil {
			return err
		}
	}
	changes := ch.changelogBy[srcID]
	for i, row := range changes {
		row = copyMap(row)
		if p.cloneSeq > 0 {
			row["id"] = fmt.Sprintf("snap:clone:%d:h:%v", p.cloneSeq, row["id"])
		}
		row["item_id"] = itemID
		if p.useMap && asString(row["at"]) != "" {
			row["at"] = mapOrEven(asString(row["at"]), &p, i, len(changes))
		}
		if err := insertRow(tx, "changelog", changelogColumns, row); err != nil {
			return err
		}
	}

	// FTS — same contentless delete+insert path as store.writeFTS.
	var rowid int64
	if err := tx.QueryRow(`SELECT rowid FROM items WHERE id = ?`, itemID).Scan(&rowid); err != nil {
		return err
	}
	bodies := make([]string, 0, len(ch.commentsBy[srcID]))
	for _, row := range ch.commentsBy[srcID] {
		if bt := asString(row["body_text"]); bt != "" {
			bodies = append(bodies, bt)
		}
	}
	title := asString(item["title"])
	body := asString(item["body_text"])
	if _, err := tx.Exec(`DELETE FROM items_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO items_fts (rowid, title, body_text, comments_text) VALUES (?,?,?,?)`,
		rowid, title, body, strings.Join(bodies, "\n"),
	); err != nil {
		return err
	}
	return nil
}

func copyOriginalLinks(src *sql.DB, tx *sql.Tx, planned []plannedIssue) error {
	// Only original (non-clone) item ids keep links.
	origIDs := map[string]bool{}
	for _, p := range planned {
		if p.cloneSeq == 0 {
			origIDs[p.src.itemID] = true
		}
	}
	rows, err := src.Query(`
		SELECT item_id, type, direction, target_key FROM links
		ORDER BY item_id, type, direction, target_key`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, typ, dir, target string
		if err := rows.Scan(&itemID, &typ, &dir, &target); err != nil {
			return err
		}
		if !origIDs[itemID] {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO links (item_id, type, direction, target_key) VALUES (?,?,?,?)`,
			itemID, typ, dir, target,
		); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Destination column lists (current schema).
var (
	itemColumns = []string{
		"id", "source_id", "kind", "external_id", "key", "title", "body_text",
		"author", "author_id", "url", "created_at", "updated_at", "synced_at",
	}
	issueColumns = []string{
		"item_id", "key", "project_key", "issue_type", "issue_type_id",
		"status", "status_id", "status_category", "priority", "priority_rank",
		"assignee", "assignee_id", "assignee_email", "reporter", "reporter_id", "reporter_email",
		"parent_key", "labels", "components", "fix_versions", "affects_versions",
		"environment_text", "duedate", "resolution", "created_at", "updated_at",
		"status_changed_at", "resolved_at", "reopen_count", "reopened_at",
		"assignee_changed_at", "comment_count", "description_adf", "custom", "raw",
		"reopen_reason", "cloned_from", "hierarchy_level", "epic_key",
	}
	pageColumns = []string{
		"item_id", "space_key", "parent_id", "version", "status",
		"body_adf", "labels", "excerpt",
	}
	commentColumns = []string{
		"id", "item_id", "external_id", "author", "author_id",
		"body_adf", "body_text", "created_at", "updated_at",
	}
	attachmentColumns = []string{
		"id", "item_id", "external_id", "filename", "mime_type", "size", "author", "created_at",
	}
	changelogColumns = []string{
		"id", "item_id", "at", "author", "field", "from_value", "from_id", "to_value", "to_id",
	}
)

func insertRow(tx *sql.Tx, table string, cols []string, data map[string]any) error {
	vals := make([]any, len(cols))
	ph := make([]string, len(cols))
	for i, c := range cols {
		ph[i] = "?"
		v, ok := data[c]
		if !ok || v == nil {
			vals[i] = nil
			continue
		}
		switch x := v.(type) {
		case string:
			if x == "" && (c == "labels" || c == "components" || c == "fix_versions" || c == "affects_versions") {
				vals[i] = "[]"
			} else if x == "" && c == "custom" {
				vals[i] = "{}"
			} else {
				vals[i] = v
			}
		default:
			vals[i] = v
		}
		// JSON array columns must stay json_each-able even when source had NULL.
		if vals[i] == nil {
			switch c {
			case "labels", "components", "fix_versions", "affects_versions":
				vals[i] = "[]"
			case "custom":
				vals[i] = "{}"
			}
		}
	}
	// Defaults for NOT NULL integer columns.
	if table == "issues" {
		if vals[indexOf(cols, "priority_rank")] == nil {
			vals[indexOf(cols, "priority_rank")] = 0
		}
		if vals[indexOf(cols, "reopen_count")] == nil {
			vals[indexOf(cols, "reopen_count")] = 0
		}
		if vals[indexOf(cols, "comment_count")] == nil {
			vals[indexOf(cols, "comment_count")] = 0
		}
		if vals[indexOf(cols, "reopen_reason")] == nil {
			vals[indexOf(cols, "reopen_reason")] = ""
		}
		if vals[indexOf(cols, "cloned_from")] == nil {
			vals[indexOf(cols, "cloned_from")] = ""
		}
	}
	if table == "pages" {
		if vals[indexOf(cols, "version")] == nil {
			vals[indexOf(cols, "version")] = 1
		}
		if vals[indexOf(cols, "parent_id")] == nil {
			vals[indexOf(cols, "parent_id")] = ""
		}
		if vals[indexOf(cols, "status")] == nil {
			vals[indexOf(cols, "status")] = "current"
		}
		if vals[indexOf(cols, "body_adf")] == nil {
			vals[indexOf(cols, "body_adf")] = ""
		}
		if vals[indexOf(cols, "labels")] == nil {
			vals[indexOf(cols, "labels")] = "[]"
		}
		if vals[indexOf(cols, "excerpt")] == nil {
			vals[indexOf(cols, "excerpt")] = ""
		}
	}
	q := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`,
		table, strings.Join(cols, ","), strings.Join(ph, ","))
	_, err := tx.Exec(q, vals...)
	return err
}

func indexOf(cols []string, name string) int {
	for i, c := range cols {
		if c == name {
			return i
		}
	}
	return -1
}

func columnNames(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func qualify(cols []string, table, prefix string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf(`%s."%s" AS "%s%s"`, table, c, prefix, c)
	}
	return strings.Join(parts, ", ")
}

func scanMap(rows *sql.Rows, cols []string) (map[string]any, error) {
	raw := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range raw {
		ptrs[i] = &raw[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	m := make(map[string]any, len(cols))
	for i, c := range cols {
		m[c] = normalize(raw[i])
	}
	return m, nil
}

func loadTableMaps(db *sql.DB, table string) ([]map[string]any, error) {
	cols, err := columnNames(db, table)
	if err != nil {
		return nil, err
	}
	// Stable order so clone/spread child placement is deterministic.
	order := "rowid"
	if hasCol(cols, "id") {
		order = `"id"`
	}
	q := fmt.Sprintf(`SELECT %s FROM %s ORDER BY %s`, strings.Join(quoteIdents(cols), ","), table, order)
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		m, err := scanMap(rows, cols)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func hasCol(cols []string, name string) bool {
	for _, c := range cols {
		if c == name {
			return true
		}
	}
	return false
}

func groupBy(rows []map[string]any, key string) map[string][]map[string]any {
	m := map[string][]map[string]any{}
	for _, r := range rows {
		k := asString(r[key])
		m[k] = append(m[k], r)
	}
	return m
}

func normalize(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	default:
		return x
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

func nullStr(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
