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
	if sb.HierarchyLevel != -1 {
		t.Errorf("SB-30 HierarchyLevel=%d want -1", sb.HierarchyLevel)
	}
	if byKey["EP-30"].EpicKey != nil {
		t.Errorf("epic's EpicKey should be nil, got %v", byKey["EP-30"].EpicKey)
	}
	if byKey["EP-30"].ParentKey != nil {
		t.Errorf("epic's ParentKey should be nil, got %v", byKey["EP-30"].ParentKey)
	}
	if byKey["EP-30"].HierarchyLevel != 1 {
		t.Errorf("EP-30 HierarchyLevel=%d want 1", byKey["EP-30"].HierarchyLevel)
	}
	if byKey["ST-30"].HierarchyLevel != 0 {
		t.Errorf("ST-30 HierarchyLevel=%d want 0", byKey["ST-30"].HierarchyLevel)
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
		return recomputeEpicKeys(tx, nil)
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

// TestAffectedEpicKeysIncludesChildren asserts the scope a single-epic upsert
// would recompute: the epic plus its already-mirrored children and grandchildren,
// not the rest of the table.
func TestAffectedEpicKeysIncludesChildren(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{
		epicIssue("2", "ST-ARR", "story", "EP-ARR", 0),
		epicIssue("3", "SB-ARR", "subtask", "ST-ARR", -1),
		epicIssue("9", "OR-UNREL", "unrelated", "", 0),
	}, Force: true}); err != nil {
		t.Fatal(err)
	}
	var got []string
	err := db.write(context.Background(), func(tx *sql.Tx) error {
		var err error
		got, err = affectedEpicKeys(tx, []string{"EP-ARR"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"EP-ARR": true, "ST-ARR": true, "SB-ARR": true}
	if len(got) != len(want) {
		t.Fatalf("affected=%v want %v", got, want)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("affected includes %q, which is outside the batch parent chain", k)
		}
	}
}

func strPtr(s string) *string { return &s }

func itoa(i int) string { return strconv.Itoa(i) }

func epicKeyMap(t *testing.T, db *DB) map[string]string {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(), `SELECT key, COALESCE(epic_key, '') FROM issues ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, epic string
		if err := rows.Scan(&key, &epic); err != nil {
			t.Fatal(err)
		}
		out[key] = epic
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func fullRecomputeEpicKeys(t *testing.T, db *DB) {
	t.Helper()
	err := db.write(context.Background(), func(tx *sql.Tx) error {
		return recomputeEpicKeys(tx, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUpsertIssuesDoesNotFullTableEpicUpdate is FAIL-first for GDK-755: a
// single-issue upsert must not run the table-wide epic_key UPDATE. sqlite
// total_changes() counts every row the full UPDATE touches.
func TestUpsertIssuesDoesNotFullTableEpicUpdate(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	const n = 20
	recs := make([]IssueRecord, 0, n)
	recs = append(recs, epicIssue("e", "EP-SCOPE", "scope epic", "", 1))
	for i := 0; i < n-1; i++ {
		recs = append(recs, epicIssue("s"+itoa(i), "ST-S"+itoa(i), "story", "EP-SCOPE", 0))
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: recs, Force: true}); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := db.sql.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{
		epicIssue("orphan", "OR-SCOPE", "unrelated", "", 0),
	}}); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := db.sql.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	var issues int
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&issues); err != nil {
		t.Fatal(err)
	}
	delta := after - before
	if delta >= issues {
		t.Fatalf("UpsertIssues of 1 issue applied %d sqlite changes with %d issues in the table; scoped epic_key recompute must not UPDATE the whole table", delta, issues)
	}
}

// TestEpicKeyDeleteMatchesFullRecompute: deleting an epic must leave the same
// epic_key column as a full-table recompute (children/grandchildren go NULL).
func TestEpicKeyDeleteMatchesFullRecompute(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{
		epicIssue("1", "EP-DEL", "epic", "", 1),
		epicIssue("2", "ST-DEL", "story", "EP-DEL", 0),
		epicIssue("3", "SB-DEL", "subtask", "ST-DEL", -1),
	}, Force: true}); err != nil {
		t.Fatal(err)
	}
	if n, err := db.DeleteItems(context.Background(), "jira", []string{"EP-DEL"}); err != nil || n != 1 {
		t.Fatalf("DeleteItems EP-DEL: n=%d err=%v", n, err)
	}
	got := epicKeyMap(t, db)
	fullRecomputeEpicKeys(t, db)
	want := epicKeyMap(t, db)
	if len(got) != len(want) {
		t.Fatalf("issue count %d vs full recompute %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s epic_key=%q after delete, full recompute wants %q", k, got[k], v)
		}
	}
}

// TestEpicKeyParentChangeMatchesFullRecompute: moving a story to another epic
// (and its subtask via the two-hop walk) must match a full-table recompute.
func TestEpicKeyParentChangeMatchesFullRecompute(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{
		epicIssue("1", "EP-A", "epic A", "", 1),
		epicIssue("2", "EP-B", "epic B", "", 1),
		epicIssue("3", "ST-M", "story", "EP-A", 0),
		epicIssue("4", "SB-M", "subtask", "ST-M", -1),
	}, Force: true}); err != nil {
		t.Fatal(err)
	}
	moved := epicIssue("3", "ST-M", "story", "EP-B", 0)
	moved.Item.UpdatedAt = ago(0)
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{moved}}); err != nil {
		t.Fatal(err)
	}
	got := epicKeyMap(t, db)
	fullRecomputeEpicKeys(t, db)
	want := epicKeyMap(t, db)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s epic_key=%q after parent change, full recompute wants %q", k, got[k], v)
		}
	}
	if want["ST-M"] != "EP-B" || want["SB-M"] != "EP-B" {
		t.Fatalf("full recompute ST-M=%q SB-M=%q want EP-B", want["ST-M"], want["SB-M"])
	}
}

// TestEpicKeyEpicMoveMatchesFullRecompute: upserting an epic that already has
// children (the reverse-arrival / "epic last" case) must match full recompute.
func TestEpicKeyEpicMoveMatchesFullRecompute(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{
		epicIssue("2", "ST-ARR", "story", "EP-ARR", 0),
		epicIssue("3", "SB-ARR", "subtask", "ST-ARR", -1),
	}, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Records: []IssueRecord{
		epicIssue("1", "EP-ARR", "epic", "", 1),
	}}); err != nil {
		t.Fatal(err)
	}
	got := epicKeyMap(t, db)
	fullRecomputeEpicKeys(t, db)
	want := epicKeyMap(t, db)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s epic_key=%q after epic arrival, full recompute wants %q", k, got[k], v)
		}
	}
	if want["ST-ARR"] != "EP-ARR" || want["SB-ARR"] != "EP-ARR" {
		t.Fatalf("full recompute ST-ARR=%q SB-ARR=%q want EP-ARR", want["ST-ARR"], want["SB-ARR"])
	}
}
