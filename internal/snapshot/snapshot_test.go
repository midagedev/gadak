package snapshot

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/scry/internal/store"
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
	for _, table := range []string{"saved_views", "watches", "favorites", "feed_reads"} {
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
	if schema != 5 {
		t.Errorf("schema_version = %d, want 5", schema)
	}
	// Personal tables must not carry rows; deleted_items / enrichments also empty.
	for _, table := range []string{"deleted_items", "enrichments"} {
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
	withPersonal bool
	withSecret   bool
	spreadish    bool
}

func seedSource(t *testing.T, o seedOpts) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(store.Source{
		ID: "jira", Kind: "jira", BaseURL: "https://example.invalid",
	}); err != nil {
		t.Fatal(err)
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
		return tm.UTC().Format("2006-01-02T15:04:05.000Z")
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
	if _, err := db.UpsertIssues(batch); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync("jira", store.SyncResult{
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
	tables := []string{"sources", "items", "issues", "comments", "attachments", "changelog", "links", "sync_state"}
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
