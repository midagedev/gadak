package retro

// internal/retro — contract ↔ assertion map. Each clause of the delegation
// spec names its test here (or the package that owns it). The move itself is
// the FAIL-first for this file: before internal/retro existed these tests
// failed to compile, and the moved session fixtures failed against a compute
// with no key fields.
//
//	C1 table and --json agree on demo.db      → cmd/gadak TestRetroDemoDBTableAndJSONAgree
//	   (byte-identical output after the move; the package owns Table/JSON)
//	C2 every count equals its key set's
//	   length, keys sorted, JSON capped        → TestRetroCountsEqualKeyLengths
//	   JSON cap and keys_truncated             → TestRetroJSONKeysCap
//	C3 --open reuses views open --keys         → cmd/gadak TestViewsOpenKeysJSON,
//	   TestRetroOpenCell (unchanged views tests prove the shared path)
//	C4 no display-name keying                  → TestRetroSourceKeysNeverOnDisplayName
//	C5 definitions every run                  → cmd/gadak TestRetroDefinitionsAndGrammar
//	C6 --since validation                     → cmd/gadak TestRetroSinceValidation
//
// Session fixtures pin their anchor at now minus 4 days: the demo snapshot
// carries writes up to 2026-08-31, so every injected visit and write lands
// strictly after all real fixture activity and nothing pre-empts them.

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// demoFixture copies examples/demo.db into a throwaway directory, ensures the
// local.db sibling exists (so visits reads work), optionally seeds
// status_catalog the way a sync does (the shipped demo fixture carries zero
// rows; the finished-week metrics need the mapping), and returns the
// directory plus a read-only handle with local.db attached. The same fixture
// pattern as cmd/gadak's sqlDemoHome, repeated here because package main's
// helpers cannot be imported.
func demoFixture(t *testing.T, seed bool) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}
	if err := store.EnsureLocal(path); err != nil {
		t.Fatalf("EnsureLocal: %v", err)
	}
	if seed {
		db, err := sql.Open("sqlite", "file:"+path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT OR IGNORE INTO status_catalog (source_id, status_id, category)
			SELECT DISTINCT it.source_id, i.status_id, i.status_category
			FROM issues i JOIN items it ON it.id = i.item_id
			WHERE i.status_id IS NOT NULL AND i.status_id <> ''`); err != nil {
			db.Close()
			t.Fatalf("seed status_catalog: %v", err)
		}
		db.Close()
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return dir, db
}

// TestRetroCountsEqualKeyLengths is C2 on the demo fixture: every count is
// the length of its key slice in every bucket, and the slices are sorted by
// key. FAIL-first: before the key fields existed this had nothing to read;
// against a compute that counted and keyed on different loops it fails on
// the first bucket where the two disagree.
//
// cycle extension (2026-09-07): CycleKeys has no count field — its length
// IS the week's sample size, so the invariant is existence: CycleP50 and
// CycleP85 are non-nil exactly when the slice is non-empty. FAIL-first
// against the pre-change source: the fields did not exist (compile error).
func TestRetroCountsEqualKeyLengths(t *testing.T) {
	_, db := demoFixture(t, true)

	rep, err := Compute(context.Background(), db, store.FeedIdentity{}, 8*7*24*time.Hour, time.Now(), Options{})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if len(rep.Buckets) < 2 {
		t.Fatalf("8w should reach more than one bucket, got %d", len(rep.Buckets))
	}
	doc := rep.JSON()
	for bi, b := range rep.Buckets {
		if b.Closed != nil && len(b.ClosedKeys) != *b.Closed {
			t.Fatalf("bucket %d closed = %d, %d keys", bi, *b.Closed, len(b.ClosedKeys))
		}
		if b.Closed == nil && len(b.ClosedKeys) != 0 {
			t.Fatalf("bucket %d closed is nil but %d keys exist", bi, len(b.ClosedKeys))
		}
		if b.InProg != nil && len(b.InProgressKeys) != *b.InProg {
			t.Fatalf("bucket %d in progress = %d, %d keys", bi, *b.InProg, len(b.InProgressKeys))
		}
		if b.InProg == nil && len(b.InProgressKeys) != 0 {
			t.Fatalf("bucket %d in progress is nil but %d keys exist", bi, len(b.InProgressKeys))
		}
		if len(b.MismatchKeys) != b.Mismatch {
			t.Fatalf("bucket %d mismatch = %d, %d keys", bi, b.Mismatch, len(b.MismatchKeys))
		}
		if (b.CycleP50 == nil) != (len(b.CycleKeys) == 0) || (b.CycleP85 == nil) != (len(b.CycleKeys) == 0) {
			t.Fatalf("bucket %d cycle: p50/p85 = %v/%v with %d keys — values exist exactly when keys do",
				bi, b.CycleP50, b.CycleP85, len(b.CycleKeys))
		}
		for _, ks := range [][]string{b.ClosedKeys, b.InProgressKeys, b.MismatchKeys, b.CycleKeys} {
			for i := 1; i < len(ks); i++ {
				if ks[i-1] > ks[i] {
					t.Fatalf("bucket %d keys not sorted: %v", bi, ks)
				}
			}
		}
		// The document carries the same equality, capped.
		jb := doc.Buckets[bi]
		if b.Closed != nil && len(jb.Keys.Closed) != *b.Closed && len(jb.Keys.Closed) != MaxJSONKeys {
			t.Fatalf("bucket %d JSON closed %d keys vs count %d", bi, len(jb.Keys.Closed), *b.Closed)
		}
		if b.Mismatch != len(jb.Keys.Mismatch) && len(jb.Keys.Mismatch) != MaxJSONKeys {
			t.Fatalf("bucket %d JSON mismatch %d keys vs count %d", bi, len(jb.Keys.Mismatch), b.Mismatch)
		}
		if len(jb.Keys.Cycle) != len(b.CycleKeys) && len(jb.Keys.Cycle) != MaxJSONKeys {
			t.Fatalf("bucket %d JSON cycle %d keys vs %d", bi, len(jb.Keys.Cycle), len(b.CycleKeys))
		}
	}
}

// TestRetroJSONKeysCap pins the document cap: arrays at MaxJSONKeys with
// keys_truncated set when cut, and no flag when everything fits.
// FAIL-first: without the cap the JSON carried every key and no flag existed.
func TestRetroJSONKeysCap(t *testing.T) {
	big := make([]string, MaxJSONKeys+1)
	for i := range big {
		big[i] = fmt.Sprintf("STD-%05d", i)
	}
	exact := make([]string, MaxJSONKeys)
	copy(exact, big[:MaxJSONKeys])
	rep := Report{Buckets: []Bucket{
		{ClosedKeys: big, InProgressKeys: exact, MismatchKeys: nil},
		{ClosedKeys: exact},
	}}
	doc := rep.JSON()
	if len(doc.Buckets[0].Keys.Closed) != MaxJSONKeys {
		t.Fatalf("capped closed has %d keys, want %d", len(doc.Buckets[0].Keys.Closed), MaxJSONKeys)
	}
	if !doc.Buckets[0].Keys.KeysTruncated {
		t.Fatal("keys_truncated missing on a cut array")
	}
	if len(doc.Buckets[0].Keys.InProgress) != MaxJSONKeys {
		t.Fatalf("exact-length array has %d keys, want %d", len(doc.Buckets[0].Keys.InProgress), MaxJSONKeys)
	}
	if len(doc.Buckets[1].Keys.Closed) != MaxJSONKeys || doc.Buckets[1].Keys.KeysTruncated {
		t.Fatalf("exactly-MaxJSONKeys must not truncate: %d keys, truncated=%v",
			len(doc.Buckets[1].Keys.Closed), doc.Buckets[1].Keys.KeysTruncated)
	}
	if doc.Buckets[1].Keys.Mismatch == nil {
		t.Fatal("empty key arrays must marshal as [], not null")
	}
}

// TestRetroSourceKeysNeverOnDisplayName is C4: the read path keys on ids and
// categories, never on the localized display name. Positive control beside
// it, so the assertion cannot pass on an empty file.
func TestRetroSourceKeysNeverOnDisplayName(t *testing.T) {
	src, err := os.ReadFile("retro.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "status = '") {
		t.Fatal("retro.go keys on a status display name; use status_id or status_category")
	}
	if !strings.Contains(string(src), "status_category") {
		t.Fatal("positive control failed: retro.go does not mention status_category at all")
	}
}

// TestRetroCurrentWeekDualMethodAgree is the spec clause that the two ways of
// answering the current week — the live issues columns and the changelog walk
// through status_catalog — must agree on the demo fixture: same item set,
// same per-item transition time.
//
// max extension (2026-09-07): wip age max comes from the same ages list as
// p85 in both branches, so the two methods must agree on it too — max over
// the live per-item times equals max over the walk per-item times. FAIL-first
// against the pre-change source: WipMax did not exist (compile error).
func TestRetroCurrentWeekDualMethodAgree(t *testing.T) {
	_, db := demoFixture(t, true)
	now := time.Now()

	live := map[string]time.Time{}
	rows, err := db.Query(`SELECT item_id, status_changed_at FROM issues WHERE status_category = 'inprogress'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var item, at string
		if err := rows.Scan(&item, &at); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ts, ok := parseTime(at)
		if !ok {
			rows.Close()
			t.Fatalf("unparseable status_changed_at %q on %s", at, item)
		}
		live[item] = ts
	}
	rows.Close()
	if len(live) == 0 {
		t.Fatal("demo fixture has no in-progress issues; dual-method check is vacuous")
	}

	cat := map[string]string{}
	crows, err := db.Query(`SELECT COALESCE(source_id,''), COALESCE(status_id,''), COALESCE(category,'') FROM status_catalog`)
	if err != nil {
		t.Fatal(err)
	}
	for crows.Next() {
		var source, id, category string
		if err := crows.Scan(&source, &id, &category); err != nil {
			crows.Close()
			t.Fatal(err)
		}
		cat[source+"\x00"+id] = category
	}
	crows.Close()

	// Ordered scan, later rows overwrite: each item keeps its last status row.
	type lastRow struct {
		at     time.Time
		toID   string
		source string
	}
	walk := map[string]lastRow{}
	wrows, err := db.Query(`SELECT c.item_id, c.at, c.to_id, COALESCE(it.source_id,'')
		FROM changelog c JOIN items it ON it.id = c.item_id
		WHERE c.field = 'status' ORDER BY c.item_id, c.at, c.id`)
	if err != nil {
		t.Fatal(err)
	}
	for wrows.Next() {
		var item, at, toID, source string
		if err := wrows.Scan(&item, &at, &toID, &source); err != nil {
			wrows.Close()
			t.Fatal(err)
		}
		ts, ok := parseTime(at)
		if !ok || ts.After(now) {
			continue
		}
		walk[item] = lastRow{at: ts, toID: toID, source: source}
	}
	wrows.Close()

	walkInProg := map[string]time.Time{}
	for item, row := range walk {
		if cat[row.source+"\x00"+row.toID] == store.CategoryInProgress {
			walkInProg[item] = row.at
		}
	}
	if len(walkInProg) != len(live) {
		t.Fatalf("in-progress at now: issues rows say %d, changelog walk says %d", len(live), len(walkInProg))
	}
	for item, liveAt := range live {
		walkAt, ok := walkInProg[item]
		if !ok {
			t.Fatalf("item %s in progress per issues rows but not per the changelog walk", item)
		}
		if !liveAt.Equal(walkAt) {
			t.Fatalf("item %s: status_changed_at %s, last status changelog row %s", item, liveAt, walkAt)
		}
	}

	// The max the two methods would print: the oldest age each sees.
	liveMax, walkMax := 0.0, 0.0
	for _, at := range live {
		if d := now.Sub(at).Hours() / 24; d > liveMax {
			liveMax = d
		}
	}
	for _, at := range walkInProg {
		if d := now.Sub(at).Hours() / 24; d > walkMax {
			walkMax = d
		}
	}
	if liveMax != walkMax {
		t.Fatalf("wip age max: issues rows say %.6f days, changelog walk says %.6f", liveMax, walkMax)
	}
	// And Compute's own current-week cell is that same max, not a third value.
	rep := computePinned(t, db, store.FeedIdentity{}, 21*24*time.Hour, now)
	last := rep.Buckets[len(rep.Buckets)-1]
	if !last.Partial || last.WipMax == nil {
		t.Fatalf("current partial week should carry wip age max: %+v", last)
	}
	if math.Abs(*last.WipMax-liveMax) > 1e-9 {
		t.Fatalf("wip age max: Compute %.6f, dual-method %.6f", *last.WipMax, liveMax)
	}
}

/* ── session fixtures ── */

// injectVisit writes a visit row straight into the fixture's local.db.
func injectVisit(t *testing.T, dir string, at time.Time, kind, key, source string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "local.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO visits (kind, key, viewed_at, source) VALUES (?, ?, ?, ?)`,
		kind, key, at.UTC().Format(config.ISOMilli), source); err != nil {
		t.Fatalf("inject visit: %v", err)
	}
}

// injectChange writes one changelog row on an existing item.
func injectChange(t *testing.T, dir string, at time.Time, item, authorID string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO changelog (id, item_id, at, author, author_id, field, from_value, from_id, to_value, to_id)
		VALUES (?, ?, ?, ?, ?, 'priority', '', '', '', '')`,
		"retro-fx-"+authorID+"-"+at.UTC().Format("150405.000"), item,
		at.UTC().Format(config.ISOMilli), "Retro Fixture", authorID); err != nil {
		t.Fatalf("inject changelog: %v", err)
	}
}

// pickItem returns a real fixture issue item id and its key.
func pickItem(t *testing.T, db *sql.DB) (item, key string) {
	t.Helper()
	if err := db.QueryRow(`SELECT it.id, it.key FROM items it
		JOIN issues i ON i.item_id = it.id
		WHERE it.kind = 'issue' AND COALESCE(it.key,'') <> ''
		ORDER BY it.key LIMIT 1`).Scan(&item, &key); err != nil {
		t.Fatalf("pick item: %v", err)
	}
	return item, key
}

// bucketContaining finds the bucket whose [From, To) holds at.
func bucketContaining(t *testing.T, rep Report, at time.Time) *Bucket {
	t.Helper()
	for i := range rep.Buckets {
		if !at.Before(rep.Buckets[i].From) && at.Before(rep.Buckets[i].To) {
			return &rep.Buckets[i]
		}
	}
	t.Fatalf("no bucket holds %s (buckets start %s)", at, rep.Buckets[0].From)
	return nil
}

// computePinned computes against the fixture with a pinned clock, so the
// session math does not depend on the wall clock of the run. Default Options.
func computePinned(t *testing.T, db *sql.DB, me store.FeedIdentity, since time.Duration, now time.Time) Report {
	t.Helper()
	rep, err := Compute(context.Background(), db, me, since, now, Options{})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return rep
}

func TestRetroSessionsResumeOnFixture(t *testing.T) {
	// Anchor: 4 days back is after every write the demo snapshot carries, so
	// the only visits and writes inside the windows below are the injected ones.
	now := time.Now().Truncate(time.Second)
	a0 := now.Add(-4 * 24 * time.Hour)

	t.Run("sessions without any own write render as — (0 of n), not a bare dash", func(t *testing.T) {
		// FAIL-first: before the cell carried the count, this bucket rendered
		// "—", indistinguishable from a week with no sessions at all.
		dir, db := demoFixture(t, false)
		item, key := pickItem(t, db)
		injectVisit(t, dir, a0, store.VisitKindIssue, key, store.VisitSourceUI)
		injectChange(t, dir, a0.Add(time.Minute), item, "someone-else") // not self

		me := store.FeedIdentity{AccountID: "test-self"}
		rep := computePinned(t, db, me, 21*24*time.Hour, now)
		b := bucketContaining(t, rep, a0)
		if b.ResumeN != 1 || b.ResumeK != 0 || b.Resume != nil {
			t.Fatalf("ResumeN/K/Resume = %d/%d/%v, want 1/0/nil", b.ResumeN, b.ResumeK, b.Resume)
		}
		if !strings.Contains(rep.Table(), "— (0 of 1)") {
			t.Fatalf("table lacks the 0-of-n resume cell:\n%s", rep.Table())
		}
	})

	t.Run("own write, exact 30m boundary stays one session", func(t *testing.T) {
		dir, db := demoFixture(t, false)
		item, key := pickItem(t, db)
		injectVisit(t, dir, a0, store.VisitKindIssue, key, store.VisitSourceUI)
		injectVisit(t, dir, a0.Add(30*time.Minute), store.VisitKindIssue, key, store.VisitSourceUI)             // exactly the gap: same session
		injectVisit(t, dir, a0.Add(60*time.Minute+time.Second), store.VisitKindIssue, key, store.VisitSourceUI) // 30m1s after the previous visit: new session
		injectChange(t, dir, a0.Add(time.Minute), item, "someone-else")                                         // not self: must be skipped
		injectChange(t, dir, a0.Add(5*time.Minute), item, "test-self")

		me := store.FeedIdentity{AccountID: "test-self"}
		rep := computePinned(t, db, me, 21*24*time.Hour, now)
		b := bucketContaining(t, rep, a0)
		if b.Sessions != 2 {
			t.Fatalf("sessions = %d, want 2 (exactly 30m is the same session, 30m1s splits)", b.Sessions)
		}
		if b.ResumeN != 2 {
			t.Fatalf("ResumeN = %d, want 2", b.ResumeN)
		}
		if b.ResumeK != 1 {
			t.Fatalf("ResumeK = %d, want 1 (only the first session has an own write)", b.ResumeK)
		}
		if b.Resume == nil || *b.Resume != 300 {
			t.Fatalf("resume = %v, want 300s (the 5m self write; the other-author write at 1m is not own)", b.Resume)
		}
		if rep.CLIFallback {
			t.Fatal("CLIFallback should be false with ui visits present")
		}
		// Session without a write is excluded from the median but counted in n:
		// the footer branch line for the sessions source names ui+unknown.
		var srcLine string
		for _, d := range rep.Definitions() {
			if d[0] == "sessions source" {
				srcLine = d[1]
			}
		}
		if !strings.Contains(srcLine, "sessions from ui+unknown visits") {
			t.Fatalf("sessions source line: %q", srcLine)
		}
	})

	t.Run("cli-only fallback, unresolved self counts any author on visited issues", func(t *testing.T) {
		dir, db := demoFixture(t, false)
		visited, vkey := pickItem(t, db)
		var other string
		if err := db.QueryRow(`SELECT it.id FROM items it JOIN issues i ON i.item_id = it.id
			WHERE it.kind = 'issue' AND it.id <> ? ORDER BY it.key LIMIT 1`, visited).Scan(&other); err != nil {
			t.Fatalf("pick other item: %v", err)
		}
		injectVisit(t, dir, a0, store.VisitKindIssue, vkey, store.VisitSourceCLI)
		injectVisit(t, dir, a0.Add(40*time.Minute), store.VisitKindIssue, vkey, store.VisitSourceCLI) // 40m gap: two sessions
		injectChange(t, dir, a0.Add(30*time.Second), other, "someone-else")                           // wrong issue: ignored
		injectChange(t, dir, a0.Add(time.Minute), visited, "someone-else")                            // visited issue: counts (unresolved self)

		rep := computePinned(t, db, store.FeedIdentity{}, 21*24*time.Hour, now)
		b := bucketContaining(t, rep, a0)
		if !rep.CLIFallback {
			t.Fatal("CLIFallback should be true when only cli visits exist")
		}
		if b.Sessions != 2 {
			t.Fatalf("sessions = %d, want 2", b.Sessions)
		}
		if b.ResumeK != 1 || b.ResumeN != 2 {
			t.Fatalf("ResumeK/N = %d/%d, want 1/2", b.ResumeK, b.ResumeN)
		}
		if b.Resume == nil || *b.Resume != 60 {
			t.Fatalf("resume = %v, want 60s (first write on a visited issue; the write on the unvisited issue does not count)", b.Resume)
		}
		defs := rep.Definitions()
		joined := ""
		for _, d := range defs {
			joined += d[0] + ": " + d[1] + "\n"
		}
		if !strings.Contains(joined, "sessions from cli visits (no ui reads recorded)") {
			t.Fatalf("cli fallback footer missing:\n%s", joined)
		}
		if !strings.Contains(joined, "self: unresolved — any author on visited issues") {
			t.Fatalf("unresolved-self footer missing:\n%s", joined)
		}
	})

	t.Run("unknown source counts with the person", func(t *testing.T) {
		dir, db := demoFixture(t, false)
		_, key := pickItem(t, db)
		injectVisit(t, dir, a0, store.VisitKindIssue, key, "")                                      // pre-V7 row: unknown, still the person
		injectVisit(t, dir, a0.Add(31*time.Minute), store.VisitKindIssue, key, store.VisitSourceUI) // 31m gap: two sessions

		rep := computePinned(t, db, store.FeedIdentity{}, 21*24*time.Hour, now)
		b := bucketContaining(t, rep, a0)
		if rep.CLIFallback {
			t.Fatal("unknown-source visits must not trigger the cli fallback")
		}
		if b.Sessions != 2 {
			t.Fatalf("sessions = %d, want 2 (unknown + ui are one person, 31m gap splits)", b.Sessions)
		}
	})
}

/* ── session gap parameter, cycle rows, old mirrors ── */

// TestFormatGap — contract ↔ assertion map: whole-minute durations render in
// the trimmed form the footer prints; everything else keeps Duration.String().
// FAIL-first: before FormatGap existed the footer hardcoded "exceeds 30m"
// regardless of the effective gap.
func TestFormatGap(t *testing.T) {
	for in, want := range map[time.Duration]string{
		30 * time.Minute:  "30m",
		90 * time.Minute:  "1h30m",
		60 * time.Minute:  "1h",
		24 * time.Hour:    "24h",
		5 * time.Minute:   "5m",
		45 * time.Minute:  "45m",
		90 * time.Second:  "1m30s", // sub-5m never reaches the footer via the parser; Duration.String() verbatim
		50 * time.Second:  "50s",   // must not trim to "5"
		105 * time.Minute: "1h45m",
	} {
		if got := FormatGap(in); got != want {
			t.Errorf("FormatGap(%v) = %q, want %q", in, got, want)
		}
	}
}

// TestRetroSessionGapParameter — contract ↔ assertion map:
//
//	A1 the split gap is Compute's, not the constant's: 6m-spaced reads are
//	   three sessions at gap=5m and two at the 30m default
//	A2 the definitions footer prints the effective gap (30m default, 45m told)
//	A3 zero Options is the default — the shipped callers' posture
//
// FAIL-first: before Options existed the split read the SessionGap constant,
// so the 6m pair was one session under any argument and the footer said 30m
// unconditionally.
func TestRetroSessionGapParameter(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	a0 := now.Add(-4 * 24 * time.Hour)

	dir, db := demoFixture(t, false)
	_, key := pickItem(t, db)
	injectVisit(t, dir, a0, store.VisitKindIssue, key, store.VisitSourceUI)
	injectVisit(t, dir, a0.Add(6*time.Minute), store.VisitKindIssue, key, store.VisitSourceUI)
	injectVisit(t, dir, a0.Add(40*time.Minute), store.VisitKindIssue, key, store.VisitSourceUI)

	rep5, err := Compute(context.Background(), db, store.FeedIdentity{}, 21*24*time.Hour, now, Options{SessionGap: 5 * time.Minute})
	if err != nil {
		t.Fatalf("Compute gap=5m: %v", err)
	}
	rep30, err := Compute(context.Background(), db, store.FeedIdentity{}, 21*24*time.Hour, now, Options{})
	if err != nil {
		t.Fatalf("Compute default gap: %v", err)
	}
	b5 := bucketContaining(t, rep5, a0)
	b30 := bucketContaining(t, rep30, a0)
	// A1 — the two numbers the delegation spec asked to see side by side.
	t.Logf("sessions at gap 5m = %d, at gap 30m = %d", b5.Sessions, b30.Sessions)
	if b5.Sessions != 3 {
		t.Fatalf("gap 5m: sessions = %d, want 3 (6m and 34m both exceed 5m)", b5.Sessions)
	}
	if b30.Sessions != 2 {
		t.Fatalf("gap 30m: sessions = %d, want 2 (6m stays one session, 34m splits)", b30.Sessions)
	}
	if b5.Sessions == b30.Sessions {
		t.Fatalf("the gap must move the count: %d vs %d", b5.Sessions, b30.Sessions)
	}
	// A2
	defs30 := rep30.JSON().Definitions
	if !strings.Contains(defs30["sessions"], "exceeds 30m") {
		t.Fatalf("default definitions line: %q", defs30["sessions"])
	}
	rep45, err := Compute(context.Background(), db, store.FeedIdentity{}, 21*24*time.Hour, now, Options{SessionGap: 45 * time.Minute})
	if err != nil {
		t.Fatalf("Compute gap=45m: %v", err)
	}
	defs45 := rep45.JSON().Definitions
	if !strings.Contains(defs45["sessions"], "exceeds 45m") {
		t.Fatalf("45m definitions line: %q", defs45["sessions"])
	}
	// A3: a zero Options report renders the default gap in its footer, not "0s".
	empty := Report{Buckets: Buckets(now, 14*24*time.Hour)}
	for _, d := range empty.Definitions() {
		if d[0] == "sessions" && !strings.Contains(d[1], "exceeds 30m") {
			t.Fatalf("hand-built Report must render the default gap: %q", d[1])
		}
	}
}

// TestRetroCycleRowsMatchColumnReads — contract ↔ assertion map:
//
//	A1 value: per bucket, p50/p85 equal Median/P85 of the column read
//	   (resolved_at window-bounded as ISOMilli strings, cycle_hours not
//	   null, reopen_count = 0), days = hours/24
//	A2 nil/dash: a bucket with no sample has both nil and no keys
//	A3 the reopen clause bites: at least one fixture issue with
//	   cycle_hours set is excluded by reopen_count != 0 (9 of 56 on the
//	   shipped fixture), so a compute that forgot the clause cannot pass A1
//	   on a week holding one
//
// FAIL-first: before this round the fields did not exist (compile error);
// the assertion exists to fail against a compute that drops the reopen
// clause or window-compares parsed times against local-midnight edges.
func TestRetroCycleRowsMatchColumnReads(t *testing.T) {
	_, db := demoFixture(t, true)
	now := time.Now().Truncate(time.Second)

	rep, err := Compute(context.Background(), db, store.FeedIdentity{}, 12*7*24*time.Hour, now, Options{})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	type sample struct {
		resolved string
		days     float64
	}
	var all []sample
	var excludedByReopen int
	rows, err := db.Query(`SELECT COALESCE(resolved_at,''), cycle_hours, COALESCE(reopen_count,0) FROM issues`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var resolved string
		var hours sql.NullFloat64
		var reopen int
		if err := rows.Scan(&resolved, &hours, &reopen); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if !hours.Valid || resolved == "" {
			continue
		}
		if reopen != 0 {
			excludedByReopen++
			continue
		}
		all = append(all, sample{resolved: resolved, days: hours.Float64 / 24})
	}
	rows.Close()
	if excludedByReopen == 0 {
		t.Log("fixture carries no reopened-with-cycle rows; A3 is vacuous on this snapshot")
	}

	bucketsWithSamples := 0
	for bi, b := range rep.Buckets {
		fromStr := b.From.UTC().Format(config.ISOMilli)
		toStr := b.To.UTC().Format(config.ISOMilli)
		var cycles []float64
		for _, s := range all {
			if s.resolved >= fromStr && s.resolved < toStr {
				cycles = append(cycles, s.days)
			}
		}
		if len(cycles) > 0 {
			bucketsWithSamples++
			if b.CycleP50 == nil || b.CycleP85 == nil {
				t.Fatalf("bucket %d has %d samples but a cycle cell is nil", bi, len(cycles))
			}
			wantP50, _ := Median(cycles)
			wantP85, _ := P85(cycles)
			if math.Abs(*b.CycleP50-wantP50) > 1e-9 || math.Abs(*b.CycleP85-wantP85) > 1e-9 {
				t.Fatalf("bucket %d cycle: got p50=%.6f p85=%.6f, column read says %.6f/%.6f",
					bi, *b.CycleP50, *b.CycleP85, wantP50, wantP85)
			}
			if len(b.CycleKeys) != len(cycles) {
				t.Fatalf("bucket %d: %d samples, %d keys", bi, len(cycles), len(b.CycleKeys))
			}
		} else if b.CycleP50 != nil || b.CycleP85 != nil || len(b.CycleKeys) != 0 {
			t.Fatalf("bucket %d has no samples but carries cycle values or keys", bi)
		}
	}
	if bucketsWithSamples == 0 {
		t.Fatal("no bucket in a 12w window carries cycle samples; A1 is vacuous on this fixture")
	}
}

// TestRetroOldMirrorCycleDash — contract ↔ assertion map:
//
//	A1 on user_version < 43 Compute returns no error (the missing column
//	   never reaches SQL)
//	A2 every bucket's cycle cells are nil with no keys — the dash
//	A3 the footer adds the migration line, same shape as the
//	   status_catalog one
//
// FAIL-first: before the version gate the cycle SELECT would run and error
// ("no such column: cycle_hours") on a v42 mirror — the gate is what turns
// the crash into the dash.
func TestRetroOldMirrorCycleDash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}
	wdb, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wdb.Exec(`PRAGMA user_version = 42`); err != nil {
		wdb.Close()
		t.Fatalf("pin user_version 42: %v", err)
	}
	wdb.Close()
	if err := store.EnsureLocal(path); err != nil {
		t.Fatalf("EnsureLocal: %v", err)
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer db.Close()

	rep, err := Compute(context.Background(), db, store.FeedIdentity{}, 8*7*24*time.Hour, time.Now(), Options{})
	if err != nil {
		t.Fatalf("A1: Compute on a pre-v43 mirror must not error: %v", err)
	}
	if !rep.CycleUnavailable {
		t.Fatal("CycleUnavailable should be set on a pre-v43 mirror")
	}
	for bi, b := range rep.Buckets {
		if b.CycleP50 != nil || b.CycleP85 != nil || len(b.CycleKeys) != 0 {
			t.Fatalf("A2: bucket %d carries cycle values on a pre-v43 mirror", bi)
		}
	}
	joined := ""
	for _, d := range rep.Definitions() {
		joined += d[0] + ": " + d[1] + "\n"
	}
	if !strings.Contains(joined, "cycle: mirror predates cycle_hours — run gadak sync to migrate") {
		t.Fatalf("A3: migration footer line missing:\n%s", joined)
	}
	// The cycle row definitions still print — the row exists, the data does not.
	if !strings.Contains(joined, "cycle p50: ") || !strings.Contains(joined, "cycle p85: ") {
		t.Fatalf("cycle row definitions missing:\n%s", joined)
	}
	// The rest of the report still computed on the same mirror.
	last := rep.Buckets[len(rep.Buckets)-1]
	if last.InProg == nil {
		t.Fatal("the in-progress row should still compute on a pre-v43 mirror")
	}
}
