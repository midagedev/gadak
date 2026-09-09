package snapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func TestParseWindow(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"90d", 90 * 24 * time.Hour},
		{"12w", 12 * 7 * 24 * time.Hour},
		{"720h", 720 * time.Hour},
		{"30m", 30 * time.Minute},
		{"", 0},
	}
	for _, tc := range cases {
		got, err := ParseWindow(tc.in)
		if err != nil {
			t.Fatalf("ParseWindow(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseWindow(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
	if _, err := ParseWindow("-1d"); err == nil {
		t.Fatal("expected error for negative duration")
	}
	if _, err := ParseWindow("nope"); err == nil {
		t.Fatal("expected error for garbage")
	}
}

func TestPersonalDataDropped(t *testing.T) {
	src := seedSource(t, seedOpts{withPersonal: true})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := Build(Options{From: src, Out: out, Seed: 1, Now: now}); err != nil {
		t.Fatal(err)
	}
	db := openRO(t, out)
	defer db.Close()
	for _, table := range []string{"saved_views", "watches", "favorites", "feed_reads", "source_queries"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s count = %d, want 0", table, n)
		}
	}
	var watermark, lastErr, firstSync sql.NullString
	var version, schema, syncCount int
	err := db.QueryRow(`
		SELECT watermark, version, last_error, schema_version, first_sync_at, sync_count
		FROM sync_state WHERE source_id = 'jira'`).
		Scan(&watermark, &version, &lastErr, &schema, &firstSync, &syncCount)
	if err != nil {
		t.Fatal(err)
	}
	if watermark.Valid && watermark.String != "" {
		t.Errorf("watermark = %q, want empty", watermark.String)
	}
	if lastErr.Valid && lastErr.String != "" {
		t.Errorf("last_error = %q, want empty", lastErr.String)
	}
	if firstSync.Valid && firstSync.String != "" {
		t.Errorf("first_sync_at = %q, want empty", firstSync.String)
	}
	if syncCount != 0 {
		t.Errorf("sync_count = %d, want 0", syncCount)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}
	// The contract is "a snapshot is built at this binary's migration level",
	// not any particular number — pinning the literal made every new migration
	// edit this line. Ask the store what current is.
	fresh, err := store.Open(filepath.Join(t.TempDir(), "level.db"))
	if err != nil {
		t.Fatal(err)
	}
	wantSchema := fresh.SchemaVersion()
	_ = fresh.Close()
	if schema != wantSchema {
		t.Errorf("schema_version = %d, want %d (current migration level)", schema, wantSchema)
	}
	// Personal tables must not carry rows; deleted_items / enrichments also empty.
	// api_usage is this machine's own Jira call volume — operational data that
	// has no business travelling to whoever receives the snapshot.
	for _, table := range []string{"deleted_items", "enrichments", "api_usage"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s count = %d, want 0", table, n)
		}
	}
}

func TestContentPreserved(t *testing.T) {
	src := seedSource(t, seedOpts{})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := Build(Options{From: src, Out: out, Seed: 1, Now: now}); err != nil {
		t.Fatal(err)
	}
	srcDB := openRO(t, src)
	defer srcDB.Close()
	dstDB := openRO(t, out)
	defer dstDB.Close()

	assertCount := func(db *sql.DB, q string, want int) {
		t.Helper()
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s → %d, want %d", q, n, want)
		}
	}
	assertCount(srcDB, `SELECT COUNT(*) FROM issues`, 2)
	assertCount(dstDB, `SELECT COUNT(*) FROM issues`, 2)
	assertCount(dstDB, `SELECT COUNT(*) FROM comments`, 1)
	assertCount(dstDB, `SELECT COUNT(*) FROM changelog`, 2)
	assertCount(dstDB, `SELECT COUNT(*) FROM links`, 1)

	var title, status string
	if err := dstDB.QueryRow(`
		SELECT summary, status FROM issues_full WHERE key = 'NMB-1'`).Scan(&title, &status); err != nil {
		t.Fatalf("issues_full: %v", err)
	}
	if title != "Idempotency retry drops key" {
		t.Errorf("title = %q", title)
	}
	if status != "In Progress" {
		t.Errorf("status = %q", status)
	}

	// FTS must find a known word from the body.
	var hit int
	err := dstDB.QueryRow(`
		SELECT COUNT(*) FROM items_fts f
		JOIN items it ON it.rowid = f.rowid
		WHERE items_fts MATCH 'idempotency'`).Scan(&hit)
	if err != nil {
		t.Fatalf("fts: %v", err)
	}
	if hit < 1 {
		t.Error("expected FTS hit for 'idempotency'")
	}
}

// TestDocumentsPreserved asserts wiki pages, spaces, item_refs, page items,
// page comments, and page FTS survive the snapshot pipeline (silent data loss
// otherwise: DOCS empty on a shared mirror).
func TestDocumentsPreserved(t *testing.T) {
	src := seedSource(t, seedOpts{withPages: true})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := Build(Options{From: src, Out: out, Seed: 1, Now: now}); err != nil {
		t.Fatal(err)
	}
	dst := openRO(t, out)
	defer dst.Close()

	assertCount := func(q string, want int) {
		t.Helper()
		var n int
		if err := dst.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s → %d, want %d", q, n, want)
		}
	}

	// Issues still land.
	assertCount(`SELECT COUNT(*) FROM issues`, 2)
	// One space, two pages, one page→issue ref, one issue→page ref.
	assertCount(`SELECT COUNT(*) FROM spaces`, 1)
	assertCount(`SELECT COUNT(*) FROM pages`, 2)
	assertCount(`SELECT COUNT(*) FROM items WHERE kind = 'page'`, 2)
	assertCount(`SELECT COUNT(*) FROM item_refs`, 2)
	// Issue comment (1) + page comment (1).
	assertCount(`SELECT COUNT(*) FROM comments`, 2)
	assertCount(`SELECT COUNT(*) FROM comments c
		JOIN items it ON it.id = c.item_id WHERE it.kind = 'page'`, 1)

	// Page projection fields round-trip (body_adf is body content — same as issues).
	var spaceKey, bodyADF, excerpt, title string
	var version int
	err := dst.QueryRow(`
		SELECT p.space_key, p.version, p.body_adf, p.excerpt, it.title
		FROM pages p JOIN items it ON it.id = p.item_id
		WHERE it.key = '100'`).Scan(&spaceKey, &version, &bodyADF, &excerpt, &title)
	if err != nil {
		t.Fatalf("page 100: %v", err)
	}
	if spaceKey != "ENG" {
		t.Errorf("space_key = %q, want ENG", spaceKey)
	}
	if version != 3 {
		t.Errorf("version = %d, want 3", version)
	}
	if title != "Runbook: gateway timeout" {
		t.Errorf("title = %q", title)
	}
	if !strings.Contains(bodyADF, "runbookBody") {
		t.Errorf("body_adf missing expected text: %q", bodyADF)
	}
	if excerpt == "" {
		t.Error("excerpt empty")
	}

	// Space name join target.
	var spaceName string
	if err := dst.QueryRow(`SELECT name FROM spaces WHERE key = 'ENG'`).Scan(&spaceName); err != nil {
		t.Fatal(err)
	}
	if spaceName != "Engineering" {
		t.Errorf("space name = %q, want Engineering", spaceName)
	}

	// item_refs: page mentions issue; issue mentions page.
	var via string
	if err := dst.QueryRow(`
		SELECT via FROM item_refs
		WHERE item_id = 'confluence:100' AND target_kind = 'issue' AND target_key = 'NMB-1'`).
		Scan(&via); err != nil {
		t.Fatalf("page→issue ref: %v", err)
	}
	if via == "" {
		t.Error("via empty on page→issue ref")
	}
	if err := dst.QueryRow(`
		SELECT via FROM item_refs
		WHERE item_id = 'jira:10001' AND target_kind = 'page' AND target_key = '100'`).
		Scan(&via); err != nil {
		t.Fatalf("issue→page ref: %v", err)
	}

	// FTS indexes page body and page comment text.
	var hit int
	if err := dst.QueryRow(`
		SELECT COUNT(*) FROM items_fts f
		JOIN items it ON it.rowid = f.rowid
		WHERE it.kind = 'page' AND items_fts MATCH 'runbookBody'`).Scan(&hit); err != nil {
		t.Fatalf("page body fts: %v", err)
	}
	if hit < 1 {
		t.Error("expected FTS hit for page body word 'runbookBody'")
	}
	if err := dst.QueryRow(`
		SELECT COUNT(*) FROM items_fts f
		JOIN items it ON it.rowid = f.rowid
		WHERE it.kind = 'page' AND items_fts MATCH 'pagecomment'`).Scan(&hit); err != nil {
		t.Fatalf("page comment fts: %v", err)
	}
	if hit < 1 {
		t.Error("expected FTS hit for page comment word 'pagecomment'")
	}

	// Personal scrub still holds when documents are present.
	for _, table := range []string{"saved_views", "watches", "favorites", "feed_reads", "deleted_items", "enrichments", "api_usage", "source_queries"} {
		var n int
		if err := dst.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s count = %d, want 0", table, n)
		}
	}
}

// TestCredentialInPageRejected covers the pages body_adf/excerpt path of the
// credential scan — a secret only in page content must refuse publish.
func TestCredentialInPageRejected(t *testing.T) {
	src := seedSource(t, seedOpts{withPages: true, withPageSecret: true})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	_, err := Build(Options{From: src, Out: out, Seed: 1, Now: now, Force: true})
	if err == nil {
		t.Fatal("expected credential error for page body")
	}
	if !strings.Contains(err.Error(), "credential-shaped") {
		t.Errorf("error = %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output file exists after credential failure: %v", err)
	}
}

func TestSpreadInvariants(t *testing.T) {
	src := seedSource(t, seedOpts{spreadish: true})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	window := 90 * 24 * time.Hour
	if _, err := Build(Options{From: src, Out: out, Spread: window, Seed: 1, Now: now}); err != nil {
		t.Fatal(err)
	}
	db := openRO(t, out)
	defer db.Close()

	type row struct {
		key, created, updated, statusCh, resolved, reopened, assigneeCh string
	}
	rows, err := db.Query(`
		SELECT key, created_at, updated_at,
			COALESCE(status_changed_at,''), COALESCE(resolved_at,''),
			COALESCE(reopened_at,''), COALESCE(assignee_changed_at,'')
		FROM issues ORDER BY created_at, key`)
	if err != nil {
		t.Fatal(err)
	}
	var issues []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key, &r.created, &r.updated, &r.statusCh, &r.resolved, &r.reopened, &r.assigneeCh); err != nil {
			t.Fatal(err)
		}
		issues = append(issues, r)
	}
	rows.Close()
	if len(issues) < 2 {
		t.Fatalf("need ≥2 issues, got %d", len(issues))
	}

	// Order of keys by created must match source order (NMB-1 then NMB-2).
	if issues[0].key != "NMB-1" || issues[1].key != "NMB-2" {
		t.Errorf("created order keys = %s, %s", issues[0].key, issues[1].key)
	}

	for _, is := range issues {
		c, ok := parseTime(is.created)
		if !ok {
			t.Fatalf("bad created %q", is.created)
		}
		u, ok := parseTime(is.updated)
		if !ok {
			t.Fatalf("bad updated %q", is.updated)
		}
		if u.Before(c) {
			t.Errorf("%s: updated before created", is.key)
		}
		for _, label := range []struct {
			name, s string
		}{
			{"status_changed_at", is.statusCh},
			{"resolved_at", is.resolved},
			{"reopened_at", is.reopened},
			{"assignee_changed_at", is.assigneeCh},
		} {
			if label.s == "" {
				continue
			}
			tt, ok := parseTime(label.s)
			if !ok {
				t.Errorf("%s %s unparsable %q", is.key, label.name, label.s)
				continue
			}
			if tt.Before(c) || tt.After(u) {
				t.Errorf("%s %s %v outside [%v, %v]", is.key, label.name, tt, c, u)
			}
		}

		// Comments and changelog for this issue.
		itemID := ""
		if err := db.QueryRow(`SELECT item_id FROM issues WHERE key = ?`, is.key).Scan(&itemID); err != nil {
			t.Fatal(err)
		}
		crows, err := db.Query(`SELECT created_at, updated_at FROM comments WHERE item_id = ?`, itemID)
		if err != nil {
			t.Fatal(err)
		}
		for crows.Next() {
			var ca, ua string
			if err := crows.Scan(&ca, &ua); err != nil {
				t.Fatal(err)
			}
			for _, s := range []string{ca, ua} {
				tt, ok := parseTime(s)
				if !ok {
					continue
				}
				if tt.Before(c) || tt.After(u) {
					t.Errorf("%s comment %v outside issue span", is.key, tt)
				}
			}
		}
		crows.Close()
		hrows, err := db.Query(`SELECT at FROM changelog WHERE item_id = ?`, itemID)
		if err != nil {
			t.Fatal(err)
		}
		for hrows.Next() {
			var at string
			if err := hrows.Scan(&at); err != nil {
				t.Fatal(err)
			}
			tt, ok := parseTime(at)
			if !ok {
				continue
			}
			if tt.Before(c) || tt.After(u) {
				t.Errorf("%s changelog %v outside issue span", is.key, tt)
			}
		}
		hrows.Close()
	}

	// Span of created_at should cover most of the window.
	c0, _ := parseTime(issues[0].created)
	cN, _ := parseTime(issues[len(issues)-1].created)
	span := cN.Sub(c0)
	if span < window*8/10 {
		t.Errorf("created span %v too small for window %v", span, window)
	}
	// Min created near now-window, max near now.
	start := now.Add(-window)
	if c0.Before(start.Add(-time.Minute)) || c0.After(start.Add(time.Hour)) {
		// even spacing puts first at exactly start
		if !c0.Equal(start) {
			t.Errorf("min created %v, want near %v", c0, start)
		}
	}
	if cN.Before(now.Add(-time.Hour)) || cN.After(now.Add(time.Minute)) {
		if !cN.Equal(now) {
			t.Errorf("max created %v, want near %v", cN, now)
		}
	}
}

func TestDeterminism(t *testing.T) {
	src := seedSource(t, seedOpts{spreadish: true})
	dir := t.TempDir()
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	opts := Options{From: src, Spread: 30 * 24 * time.Hour, Scale: 5, Seed: 42, Now: now}
	out1 := filepath.Join(dir, "a.db")
	out2 := filepath.Join(dir, "b.db")
	opts.Out = out1
	if _, err := Build(opts); err != nil {
		t.Fatal(err)
	}
	opts.Out = out2
	if _, err := Build(opts); err != nil {
		t.Fatal(err)
	}
	h1 := logicalHash(t, out1)
	h2 := logicalHash(t, out2)
	if h1 != h2 {
		t.Errorf("logical hashes differ:\n  %s\n  %s", h1, h2)
	}
}

func TestScale(t *testing.T) {
	src := seedSource(t, seedOpts{})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := Build(Options{From: src, Out: out, Scale: 7, Seed: 1, Now: now}); err != nil {
		t.Fatal(err)
	}
	db := openRO(t, out)
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("issues = %d, want 7", n)
	}
	var dup int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT key FROM items GROUP BY key HAVING COUNT(*) > 1
		)`).Scan(&dup); err != nil {
		t.Fatal(err)
	}
	if dup != 0 {
		t.Errorf("duplicate items.key groups: %d", dup)
	}
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT key FROM issues GROUP BY key HAVING COUNT(*) > 1
		)`).Scan(&dup); err != nil {
		t.Fatal(err)
	}
	if dup != 0 {
		t.Errorf("duplicate issues.key groups: %d", dup)
	}
	// No orphan children.
	for _, q := range []string{
		`SELECT COUNT(*) FROM comments c LEFT JOIN items i ON i.id = c.item_id WHERE i.id IS NULL`,
		`SELECT COUNT(*) FROM changelog c LEFT JOIN items i ON i.id = c.item_id WHERE i.id IS NULL`,
		`SELECT COUNT(*) FROM attachments a LEFT JOIN items i ON i.id = a.item_id WHERE i.id IS NULL`,
		`SELECT COUNT(*) FROM links l LEFT JOIN items i ON i.id = l.item_id WHERE i.id IS NULL`,
	} {
		var orphans int
		if err := db.QueryRow(q).Scan(&orphans); err != nil {
			t.Fatal(err)
		}
		if orphans != 0 {
			t.Errorf("orphans for %s: %d", q, orphans)
		}
	}
}

// TestScaleClonesMixSlices is the gate for GDK-1558. A clone keeps its
// source's title, body, comments, status and changelog, so if it also kept the
// source's assignee and priority then a slice narrowed to one (assignee,
// priority) pair — which is what the "My issues" view is — held one title
// repeated once per clone. Measured on the shipped 20k snapshot before the
// fix: the (Dana Whitfield, Highest, inprogress) cell was 37 rows and one
// distinct summary.
//
// The rotation that breaks that up must not flatten the source's shape while
// doing it: a flat cycle through the pools gives every value an equal share,
// which turned the demo's Medium-heavy priority menu near-uniform. So this
// asserts both halves — cells mix sources, AND the output histogram is the
// source's.
func TestScaleClonesMixSlices(t *testing.T) {
	src := seedSource(t, seedOpts{facetMix: true})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	// 8 sources, 56 clones — a whole number of rounds (7 per source), which is
	// what makes the expected share drift exactly zero rather than merely
	// small. The 1.0pp tolerance below is therefore slack, not a fudge: a
	// rotation that ignores the weights lands ~28pp out (measured on this
	// seed against the flat-cycle version).
	const scale = 64
	const tolerancePP = 1.0
	if _, err := Build(Options{From: src, Out: out, Scale: scale, Seed: 1, Now: now}); err != nil {
		t.Fatal(err)
	}

	type facets struct{ assignee, assigneeID, assigneeEmail, priority, priorityID, rank string }
	type row struct {
		id, title string
		clone     bool
		f         facets
	}
	read := func(path string) []row {
		db := openRO(t, path)
		defer db.Close()
		// A NULL and an empty string must not read alike: "unassigned" is NULL
		// in issues_raw.assignee, and a rotation that turned it into the empty
		// string would break every `assignee IS NULL` filter without moving a
		// single count.
		rows, err := db.Query(`
			SELECT it.id, it.title,
			       CASE WHEN ir.assignee       IS NULL THEN '<null>' ELSE ir.assignee       END,
			       CASE WHEN ir.assignee_id    IS NULL THEN '<null>' ELSE ir.assignee_id    END,
			       CASE WHEN ir.assignee_email IS NULL THEN '<null>' ELSE ir.assignee_email END,
			       CASE WHEN ir.priority       IS NULL THEN '<null>' ELSE ir.priority       END,
			       CASE WHEN ir.priority_id    IS NULL THEN '<null>' ELSE ir.priority_id    END,
			       CAST(ir.priority_rank AS TEXT)
			FROM issues_raw ir JOIN items it ON it.id = ir.item_id
			ORDER BY it.id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.title, &r.f.assignee, &r.f.assigneeID, &r.f.assigneeEmail,
				&r.f.priority, &r.f.priorityID, &r.f.rank); err != nil {
				t.Fatal(err)
			}
			r.clone = strings.HasPrefix(r.id, "snap:clone:")
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}

	srcRows := read(src)
	if len(srcRows) < 3 {
		t.Fatalf("seed has %d issues; the mixing assertion needs at least 3", len(srcRows))
	}
	srcFacet := map[string]facets{} // title → its source bag
	people := map[facets]int{}      // assignee bag → source rows carrying it
	prios := map[facets]int{}       // priority bag → source rows carrying it
	assigneeOnly := func(f facets) facets {
		return facets{assignee: f.assignee, assigneeID: f.assigneeID, assigneeEmail: f.assigneeEmail}
	}
	priorityOnly := func(f facets) facets {
		return facets{priority: f.priority, priorityID: f.priorityID, rank: f.rank}
	}
	for _, r := range srcRows {
		if _, dup := srcFacet[r.title]; dup {
			t.Fatalf("seed title %q is not unique; the gate keys sources by title", r.title)
		}
		srcFacet[r.title] = r.f
		people[assigneeOnly(r.f)]++
		prios[priorityOnly(r.f)]++
	}
	if len(people) < 2 || len(prios) < 2 {
		t.Fatalf("seed pools too small to rotate: %d assignees, %d priorities", len(people), len(prios))
	}

	got := read(out)
	if len(got) != scale {
		t.Fatalf("output has %d issues, want %d", len(got), scale)
	}

	outPeople := map[facets]int{}
	outPrios := map[facets]int{}
	cells := map[[2]string]map[string]bool{} // (assignee, priority) → distinct source titles
	cellRows := map[[2]string]int{}
	originals := 0
	for _, r := range got {
		want, known := srcFacet[r.title]
		if !known {
			t.Fatalf("output title %q is not one of the source's — clones must not invent titles", r.title)
		}
		// (c) Originals are never rotated.
		if !r.clone {
			originals++
			if r.f != want {
				t.Errorf("original %s (%s): facets %+v, want the source's %+v", r.id, r.title, r.f, want)
			}
		}
		// (d) Nothing invented: every bag came out of the source's pools.
		if people[assigneeOnly(r.f)] == 0 {
			t.Errorf("%s: assignee %q/%q/%q is not in the source's pool", r.title, r.f.assignee, r.f.assigneeID, r.f.assigneeEmail)
		}
		if prios[priorityOnly(r.f)] == 0 {
			t.Errorf("%s: priority %q/%q/%q is not in the source's pool", r.title, r.f.priority, r.f.priorityID, r.f.rank)
		}
		outPeople[assigneeOnly(r.f)]++
		outPrios[priorityOnly(r.f)]++
		cell := [2]string{r.f.assignee, r.f.priority}
		if cells[cell] == nil {
			cells[cell] = map[string]bool{}
		}
		cells[cell][r.title] = true
		cellRows[cell]++
	}
	if originals != len(srcRows) {
		t.Errorf("output holds %d originals, want %d", originals, len(srcRows))
	}

	// (a) No cell of any size worth looking at is one source repeated.
	for cell, titles := range cells {
		rows := cellRows[cell]
		if rows < 4 {
			continue
		}
		want := min(rows, 3)
		if len(titles) < want {
			t.Errorf("cell assignee=%q priority=%q: %d rows but only %d distinct source(s), want >= %d",
				cell[0], cell[1], rows, len(titles), want)
		}
	}

	// (b) The output keeps the source's shape.
	checkShares := func(what string, srcCount, outCount map[facets]int, label func(facets) string) {
		for bag, n := range srcCount {
			srcShare := float64(n) / float64(len(srcRows)) * 100
			outShare := float64(outCount[bag]) / float64(len(got)) * 100
			if drift := outShare - srcShare; drift > tolerancePP || drift < -tolerancePP {
				t.Errorf("%s %s: source %.2f%%, output %.2f%% (drift %+.2fpp, tolerance ±%.1fpp)",
					what, label(bag), srcShare, outShare, drift, tolerancePP)
			}
		}
	}
	checkShares("assignee", people, outPeople, func(f facets) string { return f.assignee })
	checkShares("priority", prios, outPrios, func(f facets) string { return f.priority })

	// The symptom itself: the narrow slice the flagship clip opens on.
	one := srcFacet["Idempotency retry drops key"]
	db := openRO(t, out)
	defer db.Close()
	var distinct, total int
	if err := db.QueryRow(`
		SELECT COUNT(DISTINCT it.title), COUNT(*)
		FROM issues_raw ir JOIN items it ON it.id = ir.item_id
		WHERE ir.assignee = ? AND ir.priority = ?`, one.assignee, one.priority).Scan(&distinct, &total); err != nil {
		t.Fatal(err)
	}
	if distinct < 2 {
		t.Errorf("slice assignee=%q priority=%q: %d rows but %d distinct title(s) — the slice is one issue repeated",
			one.assignee, one.priority, total, distinct)
	}
}

func TestCredentialRejected(t *testing.T) {
	src := seedSource(t, seedOpts{withSecret: true})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	_, err := Build(Options{From: src, Out: out, Seed: 1, Now: now, Force: true})
	if err == nil {
		t.Fatal("expected credential error")
	}
	if !strings.Contains(err.Error(), "credential-shaped") {
		t.Errorf("error = %v", err)
	}
	// Must not leave the output path or a tmp sibling.
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output file exists after credential failure: %v", err)
	}
	matches, _ := filepath.Glob(out + ".tmp-*")
	if len(matches) > 0 {
		t.Errorf("temp files left behind: %v", matches)
	}
}

func TestForceOverwrite(t *testing.T) {
	src := seedSource(t, seedOpts{})
	out := filepath.Join(t.TempDir(), "snap.db")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if _, err := Build(Options{From: src, Out: out, Seed: 1, Now: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Options{From: src, Out: out, Seed: 1, Now: now}); err == nil {
		t.Fatal("expected refuse without --force")
	}
	if _, err := Build(Options{From: src, Out: out, Seed: 1, Now: now, Force: true}); err != nil {
		t.Fatal(err)
	}
}

// --- fixtures ----------------------------------------------------------------

type seedOpts struct {
	withPersonal   bool
	withSecret     bool
	withPageSecret bool
	withPages      bool
	spreadish      bool
	// facetMix (GDK-1558) grows the seed from 2 issues to 8 with a deliberately
	// lopsided facet histogram — assignee NULL×4 / Ada×3 / Bo×1, priority
	// High×1 / Medium×5 / Low×2 — because the two-issue seed cannot express
	// what the clone rotation has to preserve: three or more sources per cell,
	// and a shape that a flat cycle visibly flattens. Off by default, so every
	// other test still sees the original two issues.
	facetMix bool
}

func seedSource(t *testing.T, o seedOpts) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{
		ID: "jira", Kind: "jira", BaseURL: "https://example.invalid",
	}); err != nil {
		t.Fatal(err)
	}
	if o.withPages {
		if err := db.UpsertSource(context.Background(), store.Source{
			ID: "confluence", Kind: "confluence", BaseURL: "https://example.invalid/wiki",
		}); err != nil {
			t.Fatal(err)
		}
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1c := base
	t1u := base.Add(48 * time.Hour)
	t2c := base.Add(24 * time.Hour)
	t2u := base.Add(72 * time.Hour)
	if o.spreadish {
		// Tight cluster so spreading is meaningful.
		t1c = base
		t1u = base.Add(2 * time.Hour)
		t2c = base.Add(10 * time.Minute)
		t2u = base.Add(3 * time.Hour)
	}
	fmtT := func(tm time.Time) string {
		return tm.UTC().Format(config.ISOMilli)
	}
	body := "The idempotency key is dropped when the gateway times out."
	if o.withSecret {
		body = "token ATATT" + strings.Repeat("A", 30) + " should never ship"
	}
	emptyADF := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	batch := store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"},
		Priorities: []string{"Highest", "High", "Medium", "Low"},
		Force:      true,
		Records: []store.IssueRecord{
			{
				Item: store.Item{
					ID: "jira:10001", SourceID: "jira", Kind: "issue", ExternalID: "10001",
					Key: "NMB-1", Title: "Idempotency retry drops key", BodyText: body,
					Author: "Reporter", AuthorID: "acc-r",
					URL:       "https://example.invalid/browse/NMB-1",
					CreatedAt: fmtT(t1c), UpdatedAt: fmtT(t1u),
				},
				Issue: store.Issue{
					ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
					Status: "In Progress", StatusID: "3", StatusCategory: "inprogress",
					Priority: "High", Assignee: "Ada", AssigneeID: "acc-ada",
					AssigneeEmail: "ada@example.invalid",
					Reporter:      "Reporter", ReporterID: "acc-r",
					DescriptionADF: emptyADF,
				},
				Comments: []store.Comment{{
					ID: "jira:c-1", ExternalID: "c-1", Author: "Ada", AuthorID: "acc-ada",
					BodyADF: emptyADF, BodyText: "Reproduced with sandbox gateway.",
					CreatedAt: fmtT(t1c.Add(time.Hour)), UpdatedAt: fmtT(t1c.Add(time.Hour)),
				}},
				Changelog: []store.ChangeEntry{
					{ID: "jira:h-1", At: fmtT(t1c.Add(30 * time.Minute)), Author: "Ada",
						Field: "status", FromValue: "To Do", FromID: "1", ToValue: "In Progress", ToID: "3"},
					{ID: "jira:h-2", At: fmtT(t1c.Add(90 * time.Minute)), Author: "Ada",
						Field: "assignee", ToValue: "Ada", ToID: "acc-ada"},
				},
				Links: []store.Link{
					{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"},
				},
			},
			{
				Item: store.Item{
					ID: "jira:10002", SourceID: "jira", Kind: "issue", ExternalID: "10002",
					Key: "NMB-2", Title: "Timeout budget too generous", BodyText: "Cut to 5s.",
					Author: "Reporter", AuthorID: "acc-r",
					CreatedAt: fmtT(t2c), UpdatedAt: fmtT(t2u),
				},
				Issue: store.Issue{
					ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
					Priority: "Medium", Reporter: "Reporter", ReporterID: "acc-r",
					DescriptionADF: emptyADF,
				},
			},
		},
	}
	if o.facetMix {
		// Six more issues, distinct titles, so the source carries the
		// histogram documented on seedOpts.facetMix. NMB-3 and NMB-4 share
		// (Ada, Medium) — two sources in one cell is the shape the shipped
		// fixture had and the rotation has to break up.
		extra := []struct {
			key, title, assignee, assigneeID, priority string
		}{
			{"NMB-3", "Retry storm on the payout worker", "Ada", "acc-ada", "Medium"},
			{"NMB-4", "Webhook signature check is case sensitive", "Ada", "acc-ada", "Medium"},
			{"NMB-5", "Ledger export drops the final page", "", "", "Medium"},
			{"NMB-6", "Stale cursor after a partial sync", "", "", "Medium"},
			{"NMB-7", "Audit log omits the actor on bulk edits", "", "", "Low"},
			{"NMB-8", "Settings page loses focus on save", "Bo", "acc-bo", "Low"},
		}
		for i, e := range extra {
			created := base.Add(time.Duration(48+i*6) * time.Hour)
			rec := store.IssueRecord{
				Item: store.Item{
					ID: fmt.Sprintf("jira:1000%d", i+3), SourceID: "jira", Kind: "issue",
					ExternalID: fmt.Sprintf("1000%d", i+3),
					Key:        e.key, Title: e.title, BodyText: "Seeded for facet rotation.",
					Author: "Reporter", AuthorID: "acc-r",
					CreatedAt: fmtT(created), UpdatedAt: fmtT(created.Add(time.Hour)),
				},
				Issue: store.Issue{
					ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
					Priority: e.priority, Assignee: e.assignee, AssigneeID: e.assigneeID,
					Reporter: "Reporter", ReporterID: "acc-r",
					DescriptionADF: emptyADF,
				},
			}
			if e.assignee != "" {
				rec.Issue.AssigneeEmail = strings.ToLower(e.assignee) + "@example.invalid"
			}
			batch.Records = append(batch.Records, rec)
		}
	}

	if _, err := db.UpsertIssues(context.Background(), batch); err != nil {
		t.Fatal(err)
	}

	if o.withPages {
		if err := db.UpsertSpaces(context.Background(), "confluence", []store.SpaceRow{
			{Key: "ENG", Name: "Engineering", Kind: "global"},
		}); err != nil {
			t.Fatal(err)
		}
		pageBody := "runbookBody covers gateway timeout and NMB-1 recovery."
		pageADF := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"runbookBody covers gateway timeout and NMB-1 recovery."}]}]}`)
		if o.withPageSecret {
			// ATATT-shaped token only in page ADF so issue path stays clean.
			tok := "ATATT" + strings.Repeat("B", 30)
			pageBody = "secret " + tok + " in page"
			pageADF = json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"secret ` + tok + ` in page"}]}]}`)
		}
		cmADF := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"pagecomment confirms"}]}]}`)
		if _, err := db.UpsertPages(context.Background(), []store.PageRecord{
			{
				Item: store.Item{
					ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
					Key: "100", Title: "Runbook: gateway timeout", BodyText: pageBody,
					Author: "Dana", AuthorID: "acc-dana",
					URL:       "https://example.invalid/wiki/spaces/ENG/pages/100",
					CreatedAt: fmtT(t1c), UpdatedAt: fmtT(t1u),
				},
				Page: store.Page{
					SpaceKey: "ENG", ParentID: "", Version: 3, Status: "current",
					Labels:  []string{"runbook"},
					BodyADF: pageADF,
				},
				Comments: []store.Comment{{
					ID: "confluence:c-1", ExternalID: "pc-1", Author: "Lee", AuthorID: "acc-lee",
					BodyADF: cmADF, BodyText: "pagecomment confirms the steps.",
					CreatedAt: fmtT(t1c.Add(2 * time.Hour)), UpdatedAt: fmtT(t1c.Add(2 * time.Hour)),
				}},
			},
			{
				Item: store.Item{
					ID: "confluence:200", SourceID: "confluence", Kind: "page", ExternalID: "200",
					Key: "200", Title: "Architecture overview", BodyText: "platform topology",
					Author: "Pat", AuthorID: "acc-pat",
					URL:       "https://example.invalid/wiki/spaces/ENG/pages/200",
					CreatedAt: fmtT(t2c), UpdatedAt: fmtT(t2u),
				},
				Page: store.Page{
					SpaceKey: "ENG", ParentID: "100", Version: 1, Status: "current",
					BodyADF: emptyADF,
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
		// Explicit issue→page ref (UpsertIssues may or may not extract from body).
		// Page→issue refs are written by UpsertPages from ADF/body text (NMB-1).
		rawRefs, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rawRefs.Exec(`
			INSERT OR IGNORE INTO item_refs (item_id, target_kind, target_key, via)
			VALUES ('jira:10001', 'page', '100', 'body')`); err != nil {
			rawRefs.Close()
			t.Fatal(err)
		}
		rawRefs.Close()
	}

	if err := db.RecordSync(context.Background(), "jira", store.SyncResult{
		Watermark: "2026-06-01T00:00:00.000Z",
		FullSync:  true,
	}); err != nil {
		t.Fatal(err)
	}
	// Inject personal rows and a last_error via raw SQL (store API is partial).
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if o.withPersonal {
		for _, q := range []string{
			`INSERT INTO saved_views (id, name, config, created_at) VALUES ('v1','Mine','{}','2026-01-01T00:00:00.000Z')`,
			`INSERT INTO watches (key, created_at) VALUES ('NMB-1','2026-01-01T00:00:00.000Z')`,
			`INSERT INTO favorites (key, created_at) VALUES ('NMB-2','2026-01-01T00:00:00.000Z')`,
			`INSERT INTO feed_reads (event_id, read_at) VALUES ('e1','2026-01-01T00:00:00.000Z')`,
			`INSERT INTO source_queries (id, source_id, external_id, name, query_text, config, favourite, updated_at)
			 VALUES ('jira:1','jira','1','Open in NMA','project = NMA','{}',1,'2026-01-01T00:00:00.000Z')`,
		} {
			if _, err := raw.Exec(q); err != nil {
				raw.Close()
				t.Fatal(err)
			}
		}
	}
	if _, err := raw.Exec(`UPDATE sync_state SET last_error = 'boom', first_sync_at = '2026-01-01T00:00:00.000Z', sync_count = 9`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()
	db.Close()
	return path
}

func openRO(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// logicalHash dumps copy-target tables in sorted order and hashes the text.
func logicalHash(t *testing.T, path string) string {
	t.Helper()
	db := openRO(t, path)
	defer db.Close()
	tables := []string{
		"sources", "items", "issues", "pages", "spaces", "item_refs",
		"comments", "attachments", "changelog", "links", "sync_state",
	}
	h := sha256.New()
	for _, table := range tables {
		cols, err := columnNames(db, table)
		if err != nil {
			t.Fatal(err)
		}
		// Stable order: all columns sorted by primary-ish keys.
		order := strings.Join(quoteIdents(cols), ",")
		q := fmt.Sprintf(`SELECT %s FROM %s ORDER BY %s`, order, table, order)
		rows, err := db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(h, "#%s\n", table)
		for rows.Next() {
			raw := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			parts := make([]string, len(cols))
			for i, v := range raw {
				parts[i] = fmt.Sprintf("%v", normalize(v))
			}
			fmt.Fprintln(h, strings.Join(parts, "\t"))
		}
		rows.Close()
	}
	// Also hash FTS content via join for search fidelity.
	rows, err := db.Query(`
		SELECT it.key, f.title, f.body_text, f.comments_text
		FROM items_fts f JOIN items it ON it.rowid = f.rowid
		ORDER BY it.key`)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(h, "#items_fts")
	for rows.Next() {
		var k string
		var title, body, ctext sql.NullString
		if err := rows.Scan(&k, &title, &body, &ctext); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		fmt.Fprintf(h, "%s\t%s\t%s\t%s\n", k, title.String, body.String, ctext.String)
	}
	rows.Close()
	return hex.EncodeToString(h.Sum(nil))
}

func TestInsertRowNotNullDefaults(t *testing.T) {
	// GDK-1241: notNullDefaults is the only thing between a source row that
	// has nothing for a NOT NULL column and a rejected INSERT. The scratch
	// tables below carry exactly the NOT NULL columns the destination schema
	// gives no SQLite default — an entry missing from the map fails the
	// constraint, not a later count.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := map[string]string{
		"issues_raw": `CREATE TABLE issues_raw (
			priority_rank INTEGER NOT NULL, reopen_count INTEGER NOT NULL,
			comment_count INTEGER NOT NULL, reopen_reason TEXT NOT NULL,
			cloned_from TEXT NOT NULL, priority_id TEXT NOT NULL)`,
		"pages": `CREATE TABLE pages (
			version INTEGER NOT NULL, parent_id TEXT NOT NULL,
			status TEXT NOT NULL, body_adf TEXT NOT NULL,
			labels TEXT NOT NULL, excerpt TEXT NOT NULL)`,
	}
	want := map[string]map[string]any{
		"issues_raw": {
			"priority_rank": 0, "reopen_count": 0, "comment_count": 0,
			"reopen_reason": "", "cloned_from": "", "priority_id": "",
		},
		"pages": {
			"version": 1, "parent_id": "", "status": "current",
			"body_adf": "", "labels": "[]", "excerpt": "",
		},
	}
	for table, ddl := range schema {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		var cols []string
		for c := range want[table] {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := insertRow(tx, table, cols, map[string]any{}); err != nil {
			tx.Rollback()
			t.Fatalf("%s: empty source row rejected: %v", table, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		for c, w := range want[table] {
			var got sql.NullString
			q := fmt.Sprintf(`SELECT CAST(%s AS TEXT) FROM %s`, c, table)
			if err := db.QueryRow(q).Scan(&got); err != nil {
				t.Fatalf("%s.%s: %v", table, c, err)
			}
			switch w.(type) {
			case string:
				if got.String != w.(string) {
					t.Errorf("%s.%s = %q, want %q", table, c, got.String, w)
				}
			default:
				if got.String != fmt.Sprint(w) {
					t.Errorf("%s.%s = %q, want %v", table, c, got.String, w)
				}
			}
		}
	}
}

// GDK-1680: status_catalog is a sync artifact, so nothing in the snapshot
// pipeline ever wrote it and the committed fixture shipped with zero rows.
// `gadak retro` resolves changelog status ids through that table, so on the
// demo fixture `closed` was a dash in every week and `in progress` and the
// wip-age rows had a value only for the current one — the retro screen showed
// nothing true about the only mirror most people ever open. The catalog is now
// derived from the statuses the destination's own issues carry.
func TestSnapshotDerivesStatusCatalog(t *testing.T) {
	src := seedSource(t, seedOpts{})
	out := filepath.Join(t.TempDir(), "snap.db")
	if _, err := Build(Options{From: src, Out: out, Seed: 1, Now: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	dstDB := openRO(t, out)
	defer dstDB.Close()

	// Every (status_id, category) an issue holds is in the catalog, keyed the
	// way retro reads it: by the item's source, never by display name.
	rows, err := dstDB.Query(`
		SELECT DISTINCT it.source_id, i.status_id, i.status_category
		FROM issues_raw i JOIN items it ON it.id = i.item_id
		WHERE i.status_id != '' AND i.status_category != ''`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var source, id, cat string
		if err := rows.Scan(&source, &id, &cat); err != nil {
			t.Fatal(err)
		}
		seen++
		var got string
		if err := dstDB.QueryRow(
			`SELECT category FROM status_catalog WHERE source_id = ? AND status_id = ?`, source, id,
		).Scan(&got); err != nil {
			t.Fatalf("status_catalog has no row for %s/%s (category %s): %v", source, id, cat, err)
		}
		if got != cat {
			t.Errorf("status_catalog[%s/%s] = %q, want the issues row's %q", source, id, got, cat)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("the seed carries no status ids — this test would pass vacuously")
	}
}
