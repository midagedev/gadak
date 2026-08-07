package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/midagedev/scry/internal/fields"
	"github.com/midagedev/scry/internal/jira"
)

// FieldUsageRow is one (project, alias) fill statistic from field_usage.
type FieldUsageRow struct {
	ProjectKey string
	Alias      string
	Filled     int
	Total      int
}

// ScanFieldFill streams every issue's project key and the fields object from
// stored raw JSON. Rows with empty or unparseable raw are skipped (older rows).
// fn receives field id → raw value for custom and system fields present in raw.
func (db *DB) ScanFieldFill(ctx context.Context, fn func(projectKey string, fieldVals map[string]json.RawMessage) error) error {
	rows, err := db.sql.QueryContext(ctx, `SELECT project_key, raw FROM issues`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectKey string
		var raw sql.NullString
		if err := rows.Scan(&projectKey, &raw); err != nil {
			return err
		}
		if !raw.Valid || raw.String == "" {
			continue
		}
		fieldVals, ok := parseRawFields([]byte(raw.String))
		if !ok {
			continue
		}
		if err := fn(projectKey, fieldVals); err != nil {
			return err
		}
	}
	return rows.Err()
}

// parseRawFields extracts the fields object from a stored issue raw document.
func parseRawFields(raw []byte) (map[string]json.RawMessage, bool) {
	var shell struct {
		Fields json.RawMessage `json:"fields"`
	}
	if json.Unmarshal(raw, &shell) != nil || len(shell.Fields) == 0 {
		return nil, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(shell.Fields, &m) != nil {
		return nil, false
	}
	return m, true
}

// ReingestCustom recomputes issues.custom and the FTS body from the stored raw
// JSON — no network. Returns the number of issues rewritten.
func (db *DB) ReingestCustom(ctx context.Context, specs []fields.SpecIDs, bodyFieldIDs []string) (int, error) {
	// comments_text per item, same join rule as upsertRecord. Text only —
	// cheap to hold for the whole table.
	commentsByItem := map[string]string{}
	crs, err := db.sql.QueryContext(ctx, `
		SELECT item_id, body_text FROM comments
		WHERE body_text IS NOT NULL AND body_text != ''
		ORDER BY item_id, created_at`)
	if err != nil {
		return 0, err
	}
	for crs.Next() {
		var itemID, body string
		if err := crs.Scan(&itemID, &body); err != nil {
			crs.Close()
			return 0, err
		}
		if prev := commentsByItem[itemID]; prev == "" {
			commentsByItem[itemID] = body
		} else {
			commentsByItem[itemID] = prev + "\n" + body
		}
	}
	if err := crs.Err(); err != nil {
		crs.Close()
		return 0, err
	}
	crs.Close()

	// Stream the issues and keep only the rows that change. A raw document
	// fetched with *all runs tens of KB, so holding every one for a large
	// mirror would cost gigabytes; each raw lives only for its own iteration.
	type update struct {
		itemID     string
		rowid      int64
		title      string
		bodyText   string
		customJSON string
		comments   string
	}
	var updates []update
	rs, err := db.sql.QueryContext(ctx, `
		SELECT i.item_id, it.rowid, COALESCE(it.title, ''), COALESCE(it.body_text, ''),
		       COALESCE(i.custom, '{}'), COALESCE(i.raw, ''), COALESCE(i.description_adf, '')
		FROM issues i JOIN items it ON it.id = i.item_id`)
	if err != nil {
		return 0, err
	}
	for rs.Next() {
		var itemID, title, bodyText, customJSON, raw, descADF string
		var rowid int64
		if err := rs.Scan(&itemID, &rowid, &title, &bodyText, &customJSON, &raw, &descADF); err != nil {
			rs.Close()
			return 0, err
		}
		fieldVals, ok := parseRawFields([]byte(raw))
		if !ok {
			continue
		}
		custom := fields.Coalesce(specs, fieldVals)
		newCustom := jsonObject(custom)

		// body = description plain + body field ids (same additive rule as sync.build).
		var bodyParts []string
		if descADF != "" {
			if t := jira.PlainText(json.RawMessage(descADF)); t != "" {
				bodyParts = append(bodyParts, t)
			}
		}
		// If description_adf empty, try fields.description from raw.
		if len(bodyParts) == 0 {
			if t := jira.PlainText(fieldVals["description"]); t != "" {
				bodyParts = append(bodyParts, t)
			}
		}
		for _, id := range bodyFieldIDs {
			if t := jira.PlainText(fieldVals[id]); t != "" {
				bodyParts = append(bodyParts, t)
			}
		}
		newBody := strings.TrimSpace(strings.Join(bodyParts, "\n\n"))

		if newCustom == customJSON && newBody == bodyText {
			continue
		}
		updates = append(updates, update{
			itemID:     itemID,
			rowid:      rowid,
			title:      title,
			bodyText:   newBody,
			customJSON: newCustom,
			comments:   commentsByItem[itemID],
		})
	}
	if err := rs.Err(); err != nil {
		rs.Close()
		return 0, err
	}
	rs.Close()

	if len(updates) == 0 {
		return 0, nil
	}

	const batch = 1000
	for i := 0; i < len(updates); i += batch {
		end := i + batch
		if end > len(updates) {
			end = len(updates)
		}
		chunk := updates[i:end]
		err := db.write(ctx, func(tx *sql.Tx) error {
			for _, u := range chunk {
				if _, err := tx.Exec(`UPDATE issues SET custom = ? WHERE item_id = ?`, u.customJSON, u.itemID); err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE items SET body_text = ? WHERE id = ?`, u.bodyText, u.itemID); err != nil {
					return err
				}
				if err := writeFTS(tx, u.rowid, u.title, u.bodyText, u.comments); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("reingest custom batch: %w", err)
		}
	}
	return len(updates), nil
}

// ReplaceFieldUsage replaces the entire field_usage table.
func (db *DB) ReplaceFieldUsage(ctx context.Context, rows []FieldUsageRow) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM field_usage`); err != nil {
			return err
		}
		for _, r := range rows {
			if _, err := tx.Exec(`
				INSERT INTO field_usage (project_key, alias, filled, total) VALUES (?,?,?,?)`,
				r.ProjectKey, r.Alias, r.Filled, r.Total); err != nil {
				return err
			}
		}
		return nil
	})
}

// FieldUsage returns every field_usage row.
func (db *DB) FieldUsage(ctx context.Context) ([]FieldUsageRow, error) {
	rs, err := db.sql.QueryContext(ctx, `SELECT project_key, alias, filled, total FROM field_usage ORDER BY project_key, alias`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []FieldUsageRow
	for rs.Next() {
		var r FieldUsageRow
		if err := rs.Scan(&r.ProjectKey, &r.Alias, &r.Filled, &r.Total); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

// ComputeFieldUsage builds field_usage rows from issues.custom for the given
// aliases. total is the issue count per project; filled is how many issues have
// a non-empty value for that alias.
func (db *DB) ComputeFieldUsage(ctx context.Context, aliases []string) ([]FieldUsageRow, error) {
	if len(aliases) == 0 {
		return nil, nil
	}
	rs, err := db.sql.QueryContext(ctx, `SELECT project_key, COALESCE(custom, '{}') FROM issues`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	type projStat struct {
		total  int
		filled map[string]int
	}
	byProj := map[string]*projStat{}
	for rs.Next() {
		var projectKey, customJSON string
		if err := rs.Scan(&projectKey, &customJSON); err != nil {
			return nil, err
		}
		st := byProj[projectKey]
		if st == nil {
			st = &projStat{filled: map[string]int{}}
			byProj[projectKey] = st
		}
		st.total++
		var custom map[string]any
		_ = json.Unmarshal([]byte(customJSON), &custom)
		for _, alias := range aliases {
			if fields.IsFilledAny(custom[alias]) {
				st.filled[alias]++
			}
		}
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}

	var out []FieldUsageRow
	for project, st := range byProj {
		for _, alias := range aliases {
			out = append(out, FieldUsageRow{
				ProjectKey: project,
				Alias:      alias,
				Filled:     st.filled[alias],
				Total:      st.total,
			})
		}
	}
	return out, nil
}

// HasCustomFieldKeysInRaw reports whether any stored raw document contains a
// customfield_ key under fields. Used by `scry fields --apply` to refuse when
// the mirror was never synced with custom fields.
func (db *DB) HasCustomFieldKeysInRaw(ctx context.Context) (bool, error) {
	found := false
	err := db.ScanFieldFill(ctx, func(_ string, fieldVals map[string]json.RawMessage) error {
		for id := range fieldVals {
			if strings.HasPrefix(id, "customfield_") {
				found = true
				return errStopScan
			}
		}
		return nil
	})
	if err == errStopScan {
		return true, nil
	}
	return found, err
}

// errStopScan is a private sentinel to short-circuit ScanFieldFill.
var errStopScan = fmt.Errorf("stop scan")

/* ── sync run history ── */

// SyncRun is one recorded sync pass. Only meaningful runs are stored: ones
// that changed something, were a full pass, or failed — the watch loop's
// no-op incrementals would otherwise bury the history in noise.
type SyncRun struct {
	Kind       string `json:"kind"` // full | incremental (+reconcile)
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Fetched    int    `json:"fetched"`
	Changed    int    `json:"changed"`
	Deleted    int    `json:"deleted"`
	Error      string `json:"error,omitempty"`
}

// AppendSyncRun stores one run and prunes the history to the newest 100.
func (db *DB) AppendSyncRun(ctx context.Context, sourceID string, r SyncRun) error {
	var errText any
	if r.Error != "" {
		errText = r.Error
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			INSERT INTO sync_runs (source_id, kind, started_at, finished_at, fetched, changed, deleted, error)
			VALUES (?,?,?,?,?,?,?,?)`,
			sourceID, r.Kind, r.StartedAt, r.FinishedAt, r.Fetched, r.Changed, r.Deleted, errText); err != nil {
			return err
		}
		_, err := tx.Exec(`
			DELETE FROM sync_runs WHERE source_id = ? AND id NOT IN (
				SELECT id FROM sync_runs WHERE source_id = ? ORDER BY id DESC LIMIT 100)`,
			sourceID, sourceID)
		return err
	})
}

// SyncRuns returns the newest runs first, at most limit.
func (db *DB) SyncRuns(ctx context.Context, sourceID string, limit int) ([]SyncRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rs, err := db.sql.QueryContext(ctx, `
		SELECT kind, started_at, finished_at, fetched, changed, deleted, COALESCE(error, '')
		FROM sync_runs WHERE source_id = ? ORDER BY id DESC LIMIT ?`, sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []SyncRun
	for rs.Next() {
		var r SyncRun
		if err := rs.Scan(&r.Kind, &r.StartedAt, &r.FinishedAt, &r.Fetched, &r.Changed, &r.Deleted, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}
