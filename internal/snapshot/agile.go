package snapshot

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// copyAgile carries the boards and sprints tables into the snapshot the way
// link_types ride along: reference data with no personal payload, and the
// thing the board's sprint scope reads (GDK-1656). Absent tables (an older
// source) are simply skipped.
func copyAgile(src *sql.DB, tx *sql.Tx) error {
	for _, table := range []string{"boards", "sprints"} {
		ok, err := tableExists(src, table)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		cols, err := columnNamesOf(src, table)
		if err != nil {
			return err
		}
		list := ""
		for i, c := range cols {
			if i > 0 {
				list += ", "
			}
			list += `"` + c + `"`
		}
		rows, err := src.Query(fmt.Sprintf(`SELECT %s FROM %s`, list, table))
		if err != nil {
			return err
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		marks := ""
		for i := range cols {
			if i > 0 {
				marks += ","
			}
			marks += "?"
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}
			if _, err := tx.Exec(fmt.Sprintf(`INSERT OR IGNORE INTO %s (%s) VALUES (%s)`, table, list, marks), vals...); err != nil {
				rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

func columnNamesOf(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

var sprintVersion = regexp.MustCompile(`^Sprint (\d+)$`)

// deriveSprints invents a sprint history for a fixture whose source has none
// (Options.DeriveSprints — the demo fixture, GDK-1656): every fix version
// named "Sprint N" becomes sprint N on one scrum board, the highest N is the
// future sprint, the next one down is active, everything older is closed,
// and each issue carrying such a version sits in that sprint. Dates are
// two-week windows around Now, so the active sprint is always mid-flight in
// a snapshot however old the fixture gets. Deterministic: the numbers come
// from the data, the clock from Now. Nothing runs when no such version
// exists.
func deriveSprints(tx *sql.Tx, now time.Time) error {
	rows, err := tx.Query(`
		SELECT DISTINCT it.source_id, je.value
		FROM issues_raw i JOIN items it ON it.id = i.item_id, json_each(i.fix_versions) je
		WHERE je.value LIKE 'Sprint %'`)
	if err != nil {
		return err
	}
	type sv struct {
		source string
		n      int
		name   string
	}
	var found []sv
	for rows.Next() {
		var source, name string
		if err := rows.Scan(&source, &name); err != nil {
			rows.Close()
			return err
		}
		m := sprintVersion.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		found = append(found, sv{source, n, name})
	}
	rows.Close()
	if len(found) == 0 {
		return nil
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].source != found[j].source {
			return found[i].source < found[j].source
		}
		return found[i].n < found[j].n
	})
	if _, err := tx.Exec(`DELETE FROM sprints; DELETE FROM boards`); err != nil {
		return err
	}
	const twoWeeks = 14 * 24 * time.Hour
	stamp := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
	bySource := map[string][]sv{}
	for _, f := range found {
		bySource[f.source] = append(bySource[f.source], f)
	}
	boardID := int64(0)
	for _, source := range sortedKeys(bySource) {
		list := bySource[source]
		boardID++
		if _, err := tx.Exec(`INSERT INTO boards (source_id, id, name, type, project_key) VALUES (?,?,?,?,?)`,
			source, boardID, "Team board", "scrum", ""); err != nil {
			return err
		}
		// The highest number is future, the one below active, the rest closed:
		// active runs from a week ago to a week ahead; each step is one window.
		activeIdx := len(list) - 2
		if activeIdx < 0 {
			activeIdx = 0
		}
		for i, s := range list {
			offset := time.Duration(i-activeIdx) * twoWeeks
			start := now.Add(-7*24*time.Hour + offset)
			end := start.Add(twoWeeks)
			state := "active"
			var complete, activated any
			switch {
			case i < activeIdx:
				state = "closed"
				complete, activated = stamp(end), stamp(start)
			case i > activeIdx:
				state = "future"
			default:
				activated = stamp(start)
			}
			if _, err := tx.Exec(`
				INSERT INTO sprints (source_id, id, board_id, name, goal, state, start_at, end_at, complete_at, activated_at)
				VALUES (?,?,?,?,?,?,?,?,?,?)`,
				source, s.n, boardID, s.name, "", state, stamp(start), stamp(end), complete, activated); err != nil {
				return err
			}
			if _, err := tx.Exec(`
				UPDATE issues_raw SET sprint_id = ?, sprint_name = ?, sprint_state = ?
				WHERE item_id IN (
					SELECT i.item_id FROM issues_raw i JOIN items it ON it.id = i.item_id, json_each(i.fix_versions) je
					WHERE it.source_id = ? AND je.value = ?)`,
				s.n, s.name, state, source, s.name); err != nil {
				return err
			}
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
