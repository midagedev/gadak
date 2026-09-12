package store

// blocked_test.go — GDK-1449, the flagged half of the derive layer: the
// Derive rules for blocked_hours / blocked_since, the v51 migration backfill,
// and the read surface. Clauses, one test each:
//
//	B1 closed intervals sum, the open one starts — TestDeriveBlockedFlaggedIntervals
//	B2 a v50 mirror derives the columns on the way to v51 —
//	     TestMigrateV50DerivesBlockedColumns
//	     ① flagged rows → blocked_hours / blocked_since written
//	     ② no flagged rows → NULL, not 0 (2026-09-12, GDK-1805: clause ②
//	       used to read "0 (never flagged)", which the migration is not in a
//	       position to claim — nothing had normalised the field id it reads.
//	       0 stays the sync path's answer, where the rows are normalised)
//	     ③ a linear row stays NULL — no changelog, no answer
//	     ④ a pre-v51 customfield_ row is NOT guessed at — no join key
//	       exists (schemaV51's documented decision)
//	B2b the migration claims nothing it could not read —
//	     TestMigrateV50DoesNotClaimNeverFlagged (GDK-1805)
//	B3 the read wire keeps nil / 0 / value distinct —
//	     TestIssueLiteCarriesBlockedColumns
//
// FAIL-first evidence: against the pre-change tree (1e8993ab, extracted
// read-only via git archive) every test here fails — B1/B3 on the missing
// Derived fields / IssueLite columns (compile), B2 on "no such column:
// blocked_hours" at the assertion SELECT; output in the round scratchpad,
// blocked-failfirst.out.

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"

	_ "modernc.org/sqlite"
)

// blockedAgo is an ISOMilli stamp `h` hours before a frozen base, so interval
// sums are exact decimal hours. config.ISOMilli is the layout by name — a
// hand-typed layout is how year-11116 stamps once made this test pass at 0.
func blockedAgo(base time.Time, h float64) string {
	return base.Add(-time.Duration(h * float64(time.Hour))).UTC().Format(config.ISOMilli)
}

func TestDeriveBlockedFlaggedIntervals(t *testing.T) {
	base := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	set := func(id string, at string) ChangeEntry {
		return ChangeEntry{ID: id, At: at, Field: "flagged", ToValue: "Impediment"}
	}
	clear := func(id string, at string) ChangeEntry {
		return ChangeEntry{ID: id, At: at, Field: "flagged", FromValue: "Impediment"}
	}

	// One closed interval (10:00→11:30) and one open (started 2:00 before
	// base, still up): hours sum the closed one only, since names the open.
	d := Derive(DeriveInput{Changelog: []ChangeEntry{
		set("h1", blockedAgo(base, 5)), clear("h2", blockedAgo(base, 3.5)),
		set("h3", blockedAgo(base, 2)),
	}})
	if d.BlockedHours == nil || *d.BlockedHours != 1.5 {
		t.Fatalf("blocked_hours = %v, want 1.5 (the closed interval only)", fl0(d.BlockedHours))
	}
	if d.BlockedSince == nil || *d.BlockedSince != blockedAgo(base, 2) {
		t.Fatalf("blocked_since = %v, want the open interval's start", d.BlockedSince)
	}

	// Everything closed: hours are the sum, since is nil.
	d = Derive(DeriveInput{Changelog: []ChangeEntry{
		set("h1", blockedAgo(base, 10)), clear("h2", blockedAgo(base, 8)),
		set("h3", blockedAgo(base, 5)), clear("h4", blockedAgo(base, 4)),
	}})
	if d.BlockedHours == nil || *d.BlockedHours != 3 {
		t.Fatalf("blocked_hours = %v, want 3 (2 + 1)", fl0(d.BlockedHours))
	}
	if d.BlockedSince != nil {
		t.Fatalf("blocked_since = %v with no flag up, want nil", *d.BlockedSince)
	}

	// A set while already up does not restart the interval — there is no gap
	// to measure (Jira can re-set with a different option value).
	d = Derive(DeriveInput{Changelog: []ChangeEntry{
		set("h1", blockedAgo(base, 4)), set("h2", blockedAgo(base, 2)), clear("h3", blockedAgo(base, 1)),
	}})
	if d.BlockedHours == nil || *d.BlockedHours != 3 {
		t.Fatalf("blocked_hours = %v, want 3 from the FIRST set (no restart)", fl0(d.BlockedHours))
	}

	// A clear with nothing up closes nothing: the set predates the changelog's
	// horizon and the span would be a guess.
	d = Derive(DeriveInput{Changelog: []ChangeEntry{
		clear("h1", blockedAgo(base, 1)),
	}})
	if d.BlockedHours == nil || *d.BlockedHours != 0 {
		t.Fatalf("blocked_hours = %v after an orphan clear, want 0", fl0(d.BlockedHours))
	}
	if d.BlockedSince != nil {
		t.Fatalf("blocked_since = %v after an orphan clear, want nil", *d.BlockedSince)
	}

	// Never flagged, with a history: 0, set — never blocked is an answer.
	d = Derive(DeriveInput{Changelog: []ChangeEntry{
		{ID: "h1", At: blockedAgo(base, 1), Field: "status", FromID: "1", ToID: "3"},
	}})
	if d.BlockedHours == nil || *d.BlockedHours != 0 {
		t.Fatalf("blocked_hours = %v with history and no flags, want 0", fl0(d.BlockedHours))
	}

	// No history at all (Linear): NULL is the honest answer, both columns.
	d = Derive(DeriveInput{NoHistory: true, Changelog: []ChangeEntry{
		set("h1", blockedAgo(base, 2)),
	}})
	if d.BlockedHours != nil {
		t.Fatalf("blocked_hours = %v under NoHistory, want nil — cannot be read is not never blocked", *d.BlockedHours)
	}
	if d.BlockedSince != nil {
		t.Fatalf("blocked_since = %v under NoHistory, want nil", *d.BlockedSince)
	}

	// Entries out of order: the pass sorts, so a clear stored before its set
	// still closes the right interval.
	d = Derive(DeriveInput{Changelog: []ChangeEntry{
		clear("h2", blockedAgo(base, 3)), set("h1", blockedAgo(base, 4)),
	}})
	if d.BlockedHours == nil || *d.BlockedHours != 1 {
		t.Fatalf("blocked_hours = %v from unsorted entries, want 1", fl0(d.BlockedHours))
	}
}

// fl0 formats a *float64 for failure messages without nil-dereferencing.
func fl0(v *float64) any {
	if v == nil {
		return "nil"
	}
	return *v
}

// TestMigrateV50DerivesBlockedColumns is the v51 migration gate (the v48
// carryover shape): a v50 mirror with flagged history derives the two columns
// when Open runs schemaV51's hook — through Derive, the same single owner the
// sync path uses. The customfield_ row is the documented gap: nothing joins a
// checkbox field id inside the mirror, so it stays unread until the issue's
// next sync.
func TestMigrateV50DerivesBlockedColumns(t *testing.T) {
	base := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "gadak.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 50`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sources (id, kind) VALUES ('jira','jira'), ('lin','linear')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	ins := func(itemID, source, key string) {
		t.Helper()
		if _, err := raw.Exec(`
			INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
			VALUES (?, ?, 'issue', ?, ?, ?, '2026-01-01', '2026-02-01', '2026-02-01')`,
			itemID, source, key, key, key); err != nil {
			raw.Close()
			t.Fatal(err)
		}
		if _, err := raw.Exec(`
			INSERT INTO issues_raw (item_id, key, project_key, status_id, status_category, priority_rank, reopen_count, comment_count, raw)
			VALUES (?, ?, 'STD', '3', 'inprogress', 0, 0, 0, '{}')`, itemID, key); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	flag := func(itemID, id, at, from, to string) {
		t.Helper()
		if _, err := raw.Exec(`
			INSERT INTO changelog (id, item_id, at, field, from_value, to_value)
			VALUES (?, ?, ?, 'flagged', ?, ?)`, id, itemID, at, from, to); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}

	// STD-1: one closed interval (2h) and a flag still up (since 1h before base).
	ins("jira:1", "jira", "STD-1")
	flag("jira:1", "f1", blockedAgo(base, 6), "", "Impediment")
	flag("jira:1", "f2", blockedAgo(base, 4), "Impediment", "")
	flag("jira:1", "f3", blockedAgo(base, 1), "", "Impediment")
	// STD-2: jira, never flagged — 0, not NULL.
	ins("jira:2", "jira", "STD-2")
	// STD-3: linear — NULL, not 0.
	ins("lin:3", "lin", "STD-3")
	// STD-4: the pre-v51 shape — flagged history under the site's own custom
	// field id, which no mirror-side join can name.
	ins("jira:4", "jira", "STD-4")
	if _, err := raw.Exec(`
		INSERT INTO changelog (id, item_id, at, field, from_value, to_value)
		VALUES ('f9', 'jira:4', ?, 'customfield_10021', '', 'Impediment')`, blockedAgo(base, 2)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	// Open runs the v51 hook, which is backfillBlocked.
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var hours sql.NullFloat64
	var since sql.NullString
	if err := db.QueryRow(`SELECT blocked_hours, blocked_since FROM issues_raw WHERE key = 'STD-1'`).
		Scan(&hours, &since); err != nil {
		t.Fatal(err)
	}
	if !hours.Valid || hours.Float64 != 2 {
		t.Errorf("STD-1 blocked_hours = %v, want 2 (the closed interval only)", hours)
	}
	if !since.Valid || since.String != blockedAgo(base, 1) {
		t.Errorf("STD-1 blocked_since = %v, want the open interval's start %s", since, blockedAgo(base, 1))
	}
	// 2026-09-12, GDK-1805: this row used to demand 0 — "jira supplies a
	// changelog, never flagged is an answer". The migration cannot make that
	// claim: it reads a changelog written before any build normalised the
	// flagged field id, so no rows means not readable yet. 0 remains the
	// answer on the sync path (TestDeriveBlockedFlaggedIntervals) and in the
	// snapshot backfill (BackfillBlockedTx), where the rows are normalised.
	if err := db.QueryRow(`SELECT blocked_hours FROM issues_raw WHERE key = 'STD-2'`).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if hours.Valid {
		t.Errorf("STD-2 blocked_hours = %v, want NULL — the migration may not claim never flagged for a row it could not read", hours)
	}
	if err := db.QueryRow(`SELECT blocked_hours FROM issues_raw WHERE key = 'STD-3'`).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if hours.Valid {
		t.Errorf("STD-3 blocked_hours = %v, want NULL — linear supplies no changelog", hours)
	}
	// The customfield_ row is the documented gap, and the gate pins it: if a
	// future change starts counting these rows, it must do it by discovering
	// the site's field id — not by a value-shape guess this assertion would
	// silently bless.
	if err := db.QueryRow(`SELECT blocked_hours, blocked_since FROM issues_raw WHERE key = 'STD-4'`).
		Scan(&hours, &since); err != nil {
		t.Fatal(err)
	}
	// 2026-09-12, GDK-1805: this was `hours.Valid && hours.Float64 != 0`,
	// which passed on the 0.0 the bug wrote and on NULL alike — it pinned
	// nothing. NULL is now the contract: no mirror-side join can name that
	// field, so the migration writes no answer for the row.
	if hours.Valid {
		t.Errorf("STD-4 blocked_hours = %v from a customfield_ row, want NULL — no mirror-side join can name that field", hours)
	}
	if since.Valid {
		t.Errorf("STD-4 blocked_since = %v from a customfield_ row, want NULL", since)
	}
}

// TestIssueLiteCarriesBlockedColumns is the read half (the carryover gate's
// shape): the wire must keep nil, 0 and a value distinct — 0 means never
// flagged, nil means the origin cannot answer.
func TestIssueLiteCarriesBlockedColumns(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sources (id, kind) VALUES ('jira','jira'), ('lin','linear');
			INSERT INTO items (id, source_id, kind, key, title) VALUES
			  ('i1','jira','issue','ENG-1','blocked'), ('i2','jira','issue','ENG-2','never'),
			  ('i3','lin','issue','ENG-3','unreadable');
			INSERT INTO issues_raw (item_id, key, project_key, status_category, blocked_hours, blocked_since, priority_rank, reopen_count, comment_count) VALUES
			  ('i1','ENG-1','ENG','new',4.5,'2026-09-10T10:00:00.000Z',0,0,0),
			  ('i2','ENG-2','ENG','new',0,NULL,0,0,0),
			  ('i3','ENG-3','ENG','new',NULL,NULL,0,0,0)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.IssueLitesByKeys(ctx, []string{"ENG-1", "ENG-2", "ENG-3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].BlockedHours == nil || *rows[0].BlockedHours != 4.5 {
		t.Errorf("ENG-1 blocked_hours = %v, want 4.5", fl0(rows[0].BlockedHours))
	}
	if rows[0].BlockedSince == nil || *rows[0].BlockedSince != "2026-09-10T10:00:00.000Z" {
		t.Errorf("ENG-1 blocked_since = %v, want the open start", rows[0].BlockedSince)
	}
	if rows[1].BlockedHours == nil || *rows[1].BlockedHours != 0 {
		t.Errorf("ENG-2 blocked_hours = %v, want 0", fl0(rows[1].BlockedHours))
	}
	if rows[1].BlockedSince != nil {
		t.Errorf("ENG-2 blocked_since = %v, want nil", *rows[1].BlockedSince)
	}
	if rows[2].BlockedHours != nil {
		t.Errorf("ENG-3 blocked_hours = %v, want nil — an origin with no changelog cannot answer", *rows[2].BlockedHours)
	}
}

// TestMigrateV50DoesNotClaimNeverFlagged is GDK-1805's gate: the v51 backfill
// may not write 0 — the documented value for *never flagged* — on a mirror
// whose changelog it cannot read.
//
// Normalisation of the site's own flagged field id to `flagged` happens at
// sync write time (internal/sync changelogField), and a mirror still below 51
// has never been written by a build that does it. So at migration time every
// flagged transition is still under `customfield_NNNNN`, the backfill's
// `WHERE field = 'flagged'` matches nothing, and Derive's "a jira row with a
// changelog and no flags is 0 hours" rule answers a question the migration was
// never able to ask. Measured on a real 624 MB mirror (GDK-1801): 7,177 of
// 7,177 rows at 0.0 against 13 `customfield_10021` Impediment transitions in
// the same file.
//
// The contract this pins: the migration writes a value only for rows it could
// actually read and leaves NULL — "this origin cannot say" — for the rest.
// `gadak sync --full` rewrites the changelog under the stable name and the
// next Derive fills the column in.
func TestMigrateV50DoesNotClaimNeverFlagged(t *testing.T) {
	base := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "gadak.db")
	mirrorAt(t, path, 50)

	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sources (id, kind) VALUES ('jira','jira')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	for _, key := range []string{"FLG-1", "FLG-2"} {
		id := "jira:" + key
		if _, err := raw.Exec(`
			INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
			VALUES (?, 'jira', 'issue', ?, ?, ?, '2026-01-01', '2026-02-01', '2026-02-01')`,
			id, key, key, key); err != nil {
			raw.Close()
			t.Fatal(err)
		}
		if _, err := raw.Exec(`
			INSERT INTO issues_raw (item_id, key, project_key, status_id, status_category, priority_rank, reopen_count, comment_count, raw)
			VALUES (?, ?, 'FLG', '3', 'inprogress', 0, 0, 0, '{}')`, id, key); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	// FLG-1 was flagged for two hours — under the site's own field id, the
	// only shape a pre-v51 mirror has.
	for _, e := range []struct{ id, at, from, to string }{
		{"f1", blockedAgo(base, 6), "", "Impediment"},
		{"f2", blockedAgo(base, 4), "Impediment", ""},
	} {
		if _, err := raw.Exec(`
			INSERT INTO changelog (id, item_id, at, field, from_value, to_value)
			VALUES (?, 'jira:FLG-1', ?, 'customfield_10021', ?, ?)`, e.id, e.at, e.from, e.to); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	// FLG-2 has a changelog too — a status move, nothing flagged-shaped. The
	// migration still cannot tell "never flagged" from "flagged under an id it
	// cannot recognise", so it may not answer for this row either.
	if _, err := raw.Exec(`
		INSERT INTO changelog (id, item_id, at, field, from_id, to_id)
		VALUES ('s1', 'jira:FLG-2', ?, 'status', '1', '3')`, blockedAgo(base, 9)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version %d, want %d", got, len(migrations))
	}

	for _, key := range []string{"FLG-1", "FLG-2"} {
		var hours sql.NullFloat64
		var since sql.NullString
		if err := db.QueryRow(`SELECT blocked_hours, blocked_since FROM issues_raw WHERE key = ?`, key).
			Scan(&hours, &since); err != nil {
			t.Fatal(err)
		}
		if hours.Valid {
			t.Errorf("%s blocked_hours = %v after the v51 migration, want NULL — 0 is the documented value for *never flagged*, and the migration reads a changelog no build had normalised (docs/DERIVE.md, GDK-1805)", key, hours.Float64)
		}
		if since.Valid {
			t.Errorf("%s blocked_since = %v, want NULL", key, since.String)
		}
	}
}
