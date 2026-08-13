package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// rawWithLevel builds a minimal Jira issue shell so migration json_extract can
// recover hierarchyLevel from issues.raw (the same path live data uses).
func rawWithLevel(level int) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"fields": map[string]any{
			"issuetype": map[string]any{"hierarchyLevel": level},
		},
	})
	return b
}

func epicIssue(id, key, title, parent string, level int) IssueRecord {
	return IssueRecord{
		Item: Item{
			ID: "jira:" + id, SourceID: "jira", Kind: "issue", ExternalID: id,
			Key: key, Title: title, CreatedAt: ago(10), UpdatedAt: ago(1),
		},
		Issue: Issue{
			ProjectKey: "NMB", IssueType: "X", IssueTypeID: "1",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
			ParentKey: parent, HierarchyLevel: level,
			Raw: rawWithLevel(level),
		},
	}
}

// TestEpicKeyMigrationBackfill: v10 → v11 must backfill hierarchy_level from
// raw and epic_key from the parent chain (story→epic, subtask→story→epic).
func TestEpicKeyMigrationBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	// Apply v1..v10 only so v11's backfill has real rows to rewrite.
	for i := 0; i < 10; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 10`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	// Epic EP-1 (level 1), Story ST-1 → EP-1, Subtask SB-1 → ST-1, orphan OR-1.
	insert := func(id, key, title, parent string, level int) {
		t.Helper()
		itemID := "jira:" + id
		if _, err := raw.Exec(`
			INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
			VALUES (?, 'jira', 'issue', ?, ?, ?, '2026-01-01', '2026-01-02', '2026-01-02')`,
			itemID, id, key, title); err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(`
			INSERT INTO issues (item_id, key, project_key, parent_key, priority_rank, reopen_count, comment_count, raw)
			VALUES (?, ?, 'NMB', ?, 0, 0, 0, ?)`,
			itemID, key, nullIfEmpty(parent), string(rawWithLevel(level))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`INSERT INTO sources (id, kind) VALUES ('jira', 'jira')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	insert("1", "EP-1", "Billing epic", "", 1)
	insert("2", "ST-1", "Story under epic", "EP-1", 0)
	insert("3", "SB-1", "Subtask under story", "ST-1", -1)
	insert("4", "OR-1", "Orphan story", "", 0)
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("migrate to v11: %v", err)
	}
	defer db.Close()
	if db.SchemaVersion() < 11 {
		t.Fatalf("schema version %d, want >= 11", db.SchemaVersion())
	}

	type row struct {
		key, parent string
		level       int
		epic        sql.NullString
	}
	rows, err := db.sql.QueryContext(context.Background(), `SELECT key, COALESCE(parent_key,''), hierarchy_level, epic_key FROM issues ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]row{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key, &r.parent, &r.level, &r.epic); err != nil {
			t.Fatal(err)
		}
		got[r.key] = r
	}
	check := func(key string, level int, epic string) {
		t.Helper()
		r, ok := got[key]
		if !ok {
			t.Fatalf("missing %s", key)
		}
		if r.level != level {
			t.Errorf("%s hierarchy_level=%d want %d", key, r.level, level)
		}
		if epic == "" {
			if r.epic.Valid {
				t.Errorf("%s epic_key=%q want NULL", key, r.epic.String)
			}
		} else if !r.epic.Valid || r.epic.String != epic {
			t.Errorf("%s epic_key=%v want %q", key, r.epic, epic)
		}
	}
	check("EP-1", 1, "")      // epic itself
	check("ST-1", 0, "EP-1")  // story → epic
	check("SB-1", -1, "EP-1") // subtask → story → epic
	check("OR-1", 0, "")      // no parent
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// TestEpicKeyDerivationOnUpsert covers write-path recompute: story→epic,
// subtask two-hop, no-epic NULL, reverse batch order, and IssueLite split.
func TestEpicKeyDerivationOnUpsert(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}

	// Reverse arrival: child first, then story, then epic — full recompute must
	// still resolve epic_key once the parent chain is complete.
	batch1 := Batch{Records: []IssueRecord{
		epicIssue("30", "SB-30", "Subtask first", "ST-30", -1),
	}}
	if n, err := db.UpsertIssues(context.Background(), batch1); err != nil || n != 1 {
		t.Fatalf("batch1: n=%d err=%v", n, err)
	}
	// Direct parent is stored even when the parent row is not mirrored yet;
	// epic_key stays NULL until a hierarchy_level==1 ancestor is present.
	assertEpic(t, db, "SB-30", "ST-30", nil)

	batch2 := Batch{Records: []IssueRecord{
		epicIssue("20", "ST-30", "Story second", "EP-30", 0),
	}}
	if n, err := db.UpsertIssues(context.Background(), batch2); err != nil || n != 1 {
		t.Fatalf("batch2: n=%d err=%v", n, err)
	}
	// Story's parent not mirrored yet; subtask still has no epic.
	assertEpic(t, db, "ST-30", "EP-30", nil)
	assertEpic(t, db, "SB-30", "ST-30", nil)

	batch3 := Batch{Records: []IssueRecord{
		epicIssue("10", "EP-30", "Epic last", "", 1),
	}}
	start := time.Now()
	if n, err := db.UpsertIssues(context.Background(), batch3); err != nil || n != 1 {
		t.Fatalf("batch3: n=%d err=%v", n, err)
	}
	// Full recompute cost on this tiny fixture (logged for the completion report).
	t.Logf("epic recompute after batch3 (3 issues total): %s", time.Since(start))

	assertEpic(t, db, "EP-30", "", nil)
	assertEpic(t, db, "ST-30", "EP-30", strPtr("EP-30"))
	assertEpic(t, db, "SB-30", "ST-30", strPtr("EP-30"))

	// Orphan with no epic ancestor stays NULL.
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{
		epicIssue("40", "OR-40", "No parent", "", 0),
	}}); err != nil {
		t.Fatal(err)
	}
	assertEpic(t, db, "OR-40", "", nil)

	// IssueLite exposes parent_key and epic_key separately.
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]IssueLite{}
	for _, l := range lites {
		byKey[l.IssueKey] = l
	}
	sb := byKey["SB-30"]
	if sb.ParentKey == nil || *sb.ParentKey != "ST-30" {
		t.Errorf("SB-30 ParentKey=%v want ST-30", sb.ParentKey)
	}
	if sb.EpicKey == nil || *sb.EpicKey != "EP-30" {
		t.Errorf("SB-30 EpicKey=%v want EP-30", sb.EpicKey)
	}
	if byKey["EP-30"].EpicKey != nil {
		t.Errorf("epic's EpicKey should be nil, got %v", byKey["EP-30"].EpicKey)
	}
	if byKey["EP-30"].ParentKey != nil {
		t.Errorf("epic's ParentKey should be nil, got %v", byKey["EP-30"].ParentKey)
	}
}

// TestEpicKeyRecomputeBenchmark logs wall time for a full-table epic_key UPDATE
// over a larger fixture so the completion report has a measured number.
func TestEpicKeyRecomputeBenchmark(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	const n = 500
	recs := make([]IssueRecord, 0, n)
	// 1 epic + many stories under it + one subtask each for a third of them.
	recs = append(recs, epicIssue("e", "EP-BENCH", "Bench epic", "", 1))
	for i := 0; i < n-1; i++ {
		key := "ST-B" + itoa(i)
		parent := "EP-BENCH"
		level := 0
		if i%3 == 0 {
			// subtask of previous story
			if i > 0 {
				key = "SB-B" + itoa(i)
				parent = "ST-B" + itoa(i-1)
				level = -1
			}
		}
		recs = append(recs, epicIssue("id"+itoa(i), key, "t"+itoa(i), parent, level))
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: recs, Force: true}); err != nil {
		t.Fatal(err)
	}
	// Time a second full recompute in isolation (same SQL as post-upsert).
	start := time.Now()
	err := db.write(context.Background(), func(tx *sql.Tx) error {
		return recomputeEpicKeys(tx)
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("recomputeEpicKeys on %d issues: %s", n, elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("recompute too slow on %d rows: %s", n, elapsed)
	}
	// Spot-check: a story under the epic has epic_key set.
	var ek sql.NullString
	if err := db.sql.QueryRowContext(context.Background(), `SELECT epic_key FROM issues WHERE key = 'ST-B1'`).Scan(&ek); err != nil {
		t.Fatal(err)
	}
	if !ek.Valid || ek.String != "EP-BENCH" {
		t.Errorf("ST-B1 epic_key=%v want EP-BENCH", ek)
	}
}

func assertEpic(t *testing.T, db *DB, key, wantParent string, wantEpic *string) {
	t.Helper()
	var parent, epic sql.NullString
	err := db.sql.QueryRowContext(context.Background(),
		`SELECT parent_key, epic_key FROM issues WHERE key = ?`, key,
	).Scan(&parent, &epic)
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	gotP := ""
	if parent.Valid {
		gotP = parent.String
	}
	if gotP != wantParent {
		t.Errorf("%s parent_key=%q want %q", key, gotP, wantParent)
	}
	if wantEpic == nil {
		if epic.Valid {
			t.Errorf("%s epic_key=%q want NULL", key, epic.String)
		}
	} else if !epic.Valid || epic.String != *wantEpic {
		t.Errorf("%s epic_key=%v want %q", key, epic, *wantEpic)
	}
}

func strPtr(s string) *string { return &s }

func itoa(i int) string { return strconv.Itoa(i) }
