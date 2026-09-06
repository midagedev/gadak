package main

// gadak retro — contract ↔ assertion map. Each clause of the delegation spec
// names its test here (or the package that owns it), two assertions minimum:
// the happy path and the violation or boundary beside it.
//
//	C1 RecordVisit source contract            → internal/store/local_visits_source_test.go
//	   (TestRecordVisitSourcePersisted, TestRecordVisitSourceRejected)
//	C2 table and --json agree on demo.db      → TestRetroDemoDBTableAndJSONAgree
//	C3 closed + wip age p85 equal the RECIPES
//	   hand SQL on the demo fixture           → TestRetroClosedAndWipMatchHandSQL
//	C4 no display-name keying in the new code → TestRetroSourceKeysNeverOnDisplayName
//	C5 definitions every run, no second
//	   person in the output                   → TestRetroDefinitionsAndGrammar
//	C6 --since validation, --json shape,
//	   unknown flag is a usage error          → TestRetroSinceValidation (+ shape in C2)
//	C7 a V6 local.db migrates to 7            → internal/store TestLocalV6MigratesToV7KeepsOldRows
//	C8 help and usage cover retro             → help_test.go TestHelpCoversAllCommands,
//	                                            TestUsageListsEveryCommand
//
// The session fixtures below pin their anchor at now minus 4 days: the demo
// snapshot carries writes up to 2026-08-31, so every injected visit and write
// lands strictly after all real fixture activity and nothing pre-empts them.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// retroSixRows are the row names of the table and the keys of one JSON bucket,
// in print order.
var retroSixRows = []string{
	"sessions", "resume (median)", "wip age p85", "in progress", "closed", "mismatch",
}

// retroSeedCatalog fills status_catalog the way a sync does, from the issues
// rows themselves. The shipped demo fixture carries zero rows; the C3 and
// dual-method tests need the mapping.
func retroSeedCatalog(t *testing.T) {
	t.Helper()
	path := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO status_catalog (source_id, status_id, category)
		SELECT DISTINCT it.source_id, i.status_id, i.status_category
		FROM issues i JOIN items it ON it.id = i.item_id
		WHERE i.status_id IS NOT NULL AND i.status_id <> ''`); err != nil {
		t.Fatalf("seed status_catalog: %v", err)
	}
}

// retroOpenRO is a read-only handle on the fixture mirror for hand SQL.
func retroOpenRO(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// retroJSON runs gadak retro --json and decodes the document.
func retroJSON(t *testing.T, args []string) (buckets []map[string]any, defs map[string]string) {
	t.Helper()
	out, err := capture(t, func() error { return cmdRetro(args) })
	if err != nil {
		t.Fatalf("retro %v: %v\n%s", args, err, out)
	}
	var doc struct {
		Buckets     []map[string]any  `json:"buckets"`
		Definitions map[string]string `json:"definitions"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("retro --json decode: %v\n%s", err, out)
	}
	if len(doc.Buckets) == 0 {
		t.Fatalf("no buckets in --json output:\n%s", out)
	}
	return doc.Buckets, doc.Definitions
}

// retroTable runs the text table.
func retroTable(t *testing.T, args []string) string {
	t.Helper()
	out, err := capture(t, func() error { return cmdRetro(args) })
	if err != nil {
		t.Fatalf("retro %v: %v\n%s", args, err, out)
	}
	return out
}

func TestRetroDemoDBTableAndJSONAgree(t *testing.T) {
	sqlDemoHome(t)

	table := retroTable(t, []string{})
	jsonBuckets, jsonDefs := retroJSON(t, []string{"--json"})

	lines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	defIdx := -1
	for i, l := range lines {
		if l == "definitions:" {
			defIdx = i
		}
	}
	if defIdx < 0 {
		t.Fatalf("table has no definitions footer:\n%s", table)
	}
	grid := make([][]string, 0, defIdx)
	for _, l := range lines[:defIdx] {
		grid = append(grid, regexp.MustCompile(`\s{2,}`).Split(strings.TrimRight(l, " "), -1))
	}
	if len(grid) != 1+len(retroSixRows) {
		t.Fatalf("want header + 6 metric rows, got %d rows:\n%s", len(grid), table)
	}

	header := grid[0]
	if header[0] != "metric" || header[len(header)-1] != "change" {
		t.Fatalf("header must start with metric and end with change: %v", header)
	}
	if got := len(header) - 2; got != len(jsonBuckets) {
		t.Fatalf("table has %d bucket columns, JSON has %d buckets", got, len(jsonBuckets))
	}
	// The last bucket is the current partial week: 09-01..now.
	if !strings.HasSuffix(header[len(header)-2], "..now") {
		t.Fatalf("last bucket column should end in ..now: %v", header)
	}
	// Row order is part of the contract.
	for i, name := range retroSixRows {
		if grid[i+1][0] != name {
			t.Fatalf("row %d is %q, want %q", i+1, grid[i+1][0], name)
		}
	}
	// Every bucket starts on a Monday, local time.
	for _, b := range jsonBuckets {
		from, ok := b["from"].(string)
		if !ok {
			t.Fatalf("bucket from missing: %v", b)
		}
		ts, err := time.Parse(time.RFC3339, from)
		if err != nil {
			t.Fatalf("bucket from %q is not RFC3339: %v", from, err)
		}
		if ts.Weekday() != time.Monday {
			t.Fatalf("bucket from %s is a %s, want Monday", from, ts.Weekday())
		}
	}
	// JSON bucket shape: exactly the nine keys, six named after the rows.
	first := jsonBuckets[0]
	if len(first) != 9 {
		t.Fatalf("bucket object has %d keys, want 9: %v", len(first), first)
	}
	for _, name := range append([]string{"from", "to", "partial"}, retroSixRows...) {
		if _, ok := first[name]; !ok {
			t.Fatalf("bucket object lacks key %q: %v", name, first)
		}
	}

	// Cell-by-cell: the table and the JSON document are the same numbers.
	resumeSuffix := regexp.MustCompile(` \([0-9]+ of [0-9]+\)$`)
	rowByName := map[string][]string{}
	for _, row := range grid[1:] {
		rowByName[row[0]] = row
	}
	for bi, b := range jsonBuckets {
		cell := func(row string) string { return rowByName[row][bi+1] }
		num := func(key string) (float64, bool) {
			v, ok := b[key].(float64)
			return v, ok
		}
		if v, ok := num("sessions"); !ok || cell("sessions") != strconv.Itoa(int(v)) {
			t.Fatalf("bucket %d sessions: table %q vs JSON %v", bi, cell("sessions"), b["sessions"])
		}
		if v, ok := num("mismatch"); !ok || cell("mismatch") != strconv.Itoa(int(v)) {
			t.Fatalf("bucket %d mismatch: table %q vs JSON %v", bi, cell("mismatch"), b["mismatch"])
		}
		resumeCell := resumeSuffix.ReplaceAllString(cell("resume (median)"), "")
		if v, ok := num("resume (median)"); ok && resumeCell != formatRetroSeconds(v) ||
			!ok && resumeCell != "—" {
			t.Fatalf("bucket %d resume: table %q vs JSON %v", bi, resumeCell, b["resume (median)"])
		}
		if v, ok := num("wip age p85"); ok && cell("wip age p85") != fmt.Sprintf("%.1fd", v) ||
			!ok && cell("wip age p85") != "—" {
			t.Fatalf("bucket %d wip: table %q vs JSON %v", bi, cell("wip age p85"), b["wip age p85"])
		}
		for _, row := range []string{"in progress", "closed"} {
			c := cell(row)
			if v, ok := num(row); ok && c != strconv.Itoa(int(v)) || !ok && c != "—" {
				t.Fatalf("bucket %d %s: table %q vs JSON %v", bi, row, c, b[row])
			}
		}
	}

	// The footer defines every row, every run (C5), with the same strings the
	// JSON document carries.
	footer := lines[defIdx+1:]
	for _, name := range retroSixRows {
		found := false
		for _, l := range footer {
			if strings.HasPrefix(l, name+": ") {
				found = true
				if jsonDefs[name] != strings.TrimPrefix(l, name+": ") {
					t.Fatalf("definition of %q differs between table and JSON", name)
				}
			}
		}
		if !found {
			t.Fatalf("definitions footer lacks a line for %q:\n%s", name, table)
		}
	}
}

func TestRetroClosedAndWipMatchHandSQL(t *testing.T) {
	sqlDemoHome(t)
	retroSeedCatalog(t)

	// The two hand queries of docs/RECIPES.md ## Retro, kept character for
	// character beside the retro implementation that must agree with them.
	const handClosedSQL = `select count(distinct c.item_id) as closed
from changelog c
join items it on it.id = c.item_id
join status_catalog done
  on done.source_id = it.source_id and done.status_id = c.to_id
 and done.category = 'done'
left join status_catalog prev
  on prev.source_id = it.source_id and prev.status_id = c.from_id
 and prev.category = 'done'
where c.field = 'status'
  and c.at >= ?
  and c.at <  ?
  and prev.status_id is null`
	const handWipSQL = `with ages as (
  select julianday('now') - julianday(status_changed_at) as days
  from issues_full
  where status_category = 'inprogress'
)
select round(days, 1) as wip_age_p85
from ages
order by days
limit 1 offset ((85 * (select count(*) from ages) + 99) / 100 - 1)`

	buckets, _ := retroJSON(t, []string{"--since", "8w", "--json"})
	if len(buckets) < 2 {
		t.Fatalf("8w should reach more than one bucket, got %d", len(buckets))
	}
	db := retroOpenRO(t)
	for bi, b := range buckets {
		from, err := time.Parse(time.RFC3339, b["from"].(string))
		if err != nil {
			t.Fatalf("bucket %d from: %v", bi, err)
		}
		to, err := time.Parse(time.RFC3339, b["to"].(string))
		if err != nil {
			t.Fatalf("bucket %d to: %v", bi, err)
		}
		// Week edges are local midnight; the mirror stores UTC, so the hand
		// query states the bound in UTC ISOMilli.
		var hand int
		if err := db.QueryRow(handClosedSQL,
			from.UTC().Format(config.ISOMilli), to.UTC().Format(config.ISOMilli)).Scan(&hand); err != nil {
			t.Fatalf("bucket %d hand closed query: %v", bi, err)
		}
		got, ok := b["closed"].(float64)
		if !ok {
			t.Fatalf("bucket %d closed is null in JSON, hand SQL says %d", bi, hand)
		}
		if int(got) != hand {
			t.Fatalf("bucket %d closed: retro %d, RECIPES hand SQL %d", bi, int(got), hand)
		}
	}
	// Current week wip age p85 against the hand query, one decimal.
	var handWip sql.NullFloat64
	if err := db.QueryRow(handWipSQL).Scan(&handWip); err != nil || !handWip.Valid {
		t.Fatalf("hand wip query: %v valid=%v", err, handWip.Valid)
	}
	last := buckets[len(buckets)-1]
	gotWip, ok := last["wip age p85"].(float64)
	if !ok {
		t.Fatalf("current bucket wip age p85 is null in JSON: %v", last)
	}
	if fmt.Sprintf("%.1f", gotWip) != fmt.Sprintf("%.1f", handWip.Float64) {
		t.Fatalf("wip age p85: retro %.1f, RECIPES hand SQL %.1f", gotWip, handWip.Float64)
	}
}

// TestRetroCurrentWeekDualMethodAgree is the spec clause that the two ways of
// answering the current week — the live issues columns and the changelog walk
// through status_catalog — must agree on the demo fixture: same item set,
// same per-item transition time.
func TestRetroCurrentWeekDualMethodAgree(t *testing.T) {
	sqlDemoHome(t)
	retroSeedCatalog(t)
	db := retroOpenRO(t)
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
		ts, ok := parseRetroTime(at)
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
		ts, ok := parseRetroTime(at)
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
}

// TestRetroSourceKeysNeverOnDisplayName is C4: the new read path keys on ids
// and categories, never on the localized display name. Positive control
// beside it, so the assertion cannot pass on an empty file.
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

func TestRetroDefinitionsAndGrammar(t *testing.T) {
	sqlDemoHome(t)
	table := retroTable(t, []string{"--since", "8w"})

	// C5 second person: the instrument speaks about the work, never to a person.
	second := regexp.MustCompile(`(?i)\b(you|your)\b`)
	if loc := second.FindString(table); loc != "" {
		t.Fatalf("table output contains %q: %s", loc, table)
	}
	if !strings.Contains(table, "definitions:") {
		t.Fatalf("definitions footer missing:\n%s", table)
	}
	// Every row name carries a definition, printed under the numbers.
	for _, name := range retroSixRows {
		if !strings.Contains(table, name+": ") {
			t.Fatalf("definition for %q missing from the footer:\n%s", name, table)
		}
	}
	// The heuristic row says it is one, in the footer, every run.
	if !strings.Contains(table, "(heuristic: done-words in comments on unfinished issues)") {
		t.Fatalf("mismatch heuristic footer missing:\n%s", table)
	}
}

func TestRetroSinceValidation(t *testing.T) {
	valid := map[string]time.Duration{
		"1d":   24 * time.Hour,
		"14d":  14 * 24 * time.Hour,
		"1w":   7 * 24 * time.Hour,
		"52w":  364 * 24 * time.Hour,
		"365d": 365 * 24 * time.Hour,
		" 8w ": 56 * 24 * time.Hour, // surrounding space is trimmed
	}
	for in, want := range valid {
		got, err := parseRetroSince(in)
		if err != nil || got != want {
			t.Errorf("parseRetroSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"3x", "", "0d", "366d", "53w", "14", "d", "-1d", "1.5d", "w"} {
		if got, err := parseRetroSince(in); err == nil {
			t.Errorf("parseRetroSince(%q) = %v, want an error", in, got)
		}
	}

	sqlDemoHome(t)
	out, err := capture(t, func() error { return cmdRetro([]string{"--since", "3x"}) })
	if err == nil || !strings.Contains(err.Error(), `run "gadak retro --help" for examples`) {
		t.Fatalf("--since 3x should be a usage error pointing at --help, got %v (out %q)", err, out)
	}
	out, err = capture(t, func() error { return cmdRetro([]string{"--bogus"}) })
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--bogus should be an unknown-flag error, got %v (out %q)", err, out)
	}
	// A positional argument is refused too: retro reads the workspace, not args.
	out, err = capture(t, func() error { return cmdRetro([]string{"week"}) })
	if err == nil || !strings.Contains(err.Error(), retroUsageLine) {
		t.Fatalf("positional arg should hit the usage line, got %v (out %q)", err, out)
	}
}

/* ── session fixtures ── */

// retroFixtureHome prepares a demo fixture home with a local.db present.
func retroFixtureHome(t *testing.T) string {
	t.Helper()
	sqlDemoHome(t)
	mirror := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	if err := store.EnsureLocal(mirror); err != nil {
		t.Fatalf("EnsureLocal: %v", err)
	}
	return mirror
}

// retroInjectVisit writes a visit row straight into local.db.
func retroInjectVisit(t *testing.T, at time.Time, kind, key, source string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(os.Getenv("GADAK_HOME"), "local.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO visits (kind, key, viewed_at, source) VALUES (?, ?, ?, ?)`,
		kind, key, at.UTC().Format(config.ISOMilli), source); err != nil {
		t.Fatalf("inject visit: %v", err)
	}
}

// retroInjectChange writes one changelog row on an existing item.
func retroInjectChange(t *testing.T, at time.Time, item, authorID string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
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

// retroPickItem returns a real fixture issue item id and its key.
func retroPickItem(t *testing.T) (item, key string) {
	t.Helper()
	db := retroOpenRO(t)
	if err := db.QueryRow(`SELECT it.id, it.key FROM items it
		JOIN issues i ON i.item_id = it.id
		WHERE it.kind = 'issue' AND COALESCE(it.key,'') <> ''
		ORDER BY it.key LIMIT 1`).Scan(&item, &key); err != nil {
		t.Fatalf("pick item: %v", err)
	}
	return item, key
}

// retroBucketContaining finds the bucket whose [From, To) holds at.
func retroBucketContaining(t *testing.T, rep retroReport, at time.Time) *retroBucket {
	t.Helper()
	for i := range rep.buckets {
		if !at.Before(rep.buckets[i].From) && at.Before(rep.buckets[i].To) {
			return &rep.buckets[i]
		}
	}
	t.Fatalf("no bucket holds %s (buckets start %s)", at, rep.buckets[0].From)
	return nil
}

// retroComputePinned opens the fixture read-only and computes with a pinned
// clock, so the session math does not depend on the wall clock of the run.
func retroComputePinned(t *testing.T, me store.FeedIdentity, since time.Duration, now time.Time) retroReport {
	t.Helper()
	db, err := openReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rep, err := computeRetro(db, me, since, now)
	if err != nil {
		t.Fatalf("computeRetro: %v", err)
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
		retroFixtureHome(t)
		item, key := retroPickItem(t)
		retroInjectVisit(t, a0, store.VisitKindIssue, key, store.VisitSourceUI)
		retroInjectChange(t, a0.Add(time.Minute), item, "someone-else") // not self

		me := store.FeedIdentity{AccountID: "test-self"}
		rep := retroComputePinned(t, me, 21*24*time.Hour, now)
		b := retroBucketContaining(t, rep, a0)
		if b.ResumeN != 1 || b.ResumeK != 0 || b.Resume != nil {
			t.Fatalf("ResumeN/K/Resume = %d/%d/%v, want 1/0/nil", b.ResumeN, b.ResumeK, b.Resume)
		}
		if !strings.Contains(rep.table(), "— (0 of 1)") {
			t.Fatalf("table lacks the 0-of-n resume cell:\n%s", rep.table())
		}
	})

	t.Run("own write, exact 30m boundary stays one session", func(t *testing.T) {
		retroFixtureHome(t)
		item, key := retroPickItem(t)
		retroInjectVisit(t, a0, store.VisitKindIssue, key, store.VisitSourceUI)
		retroInjectVisit(t, a0.Add(30*time.Minute), store.VisitKindIssue, key, store.VisitSourceUI)             // exactly the gap: same session
		retroInjectVisit(t, a0.Add(60*time.Minute+time.Second), store.VisitKindIssue, key, store.VisitSourceUI) // 30m1s after the previous visit: new session
		retroInjectChange(t, a0.Add(time.Minute), item, "someone-else")                                         // not self: must be skipped
		retroInjectChange(t, a0.Add(5*time.Minute), item, "test-self")

		me := store.FeedIdentity{AccountID: "test-self"}
		rep := retroComputePinned(t, me, 21*24*time.Hour, now)
		b := retroBucketContaining(t, rep, a0)
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
		if rep.cliFallback {
			t.Fatal("cliFallback should be false with ui visits present")
		}
		// Session without a write is excluded from the median but counted in n:
		// the footer branch line for the sessions source names ui+unknown.
		var srcLine string
		for _, d := range rep.definitions() {
			if d[0] == "sessions source" {
				srcLine = d[1]
			}
		}
		if !strings.Contains(srcLine, "sessions from ui+unknown visits") {
			t.Fatalf("sessions source line: %q", srcLine)
		}
	})

	t.Run("cli-only fallback, unresolved self counts any author on visited issues", func(t *testing.T) {
		retroFixtureHome(t)
		visited, vkey := retroPickItem(t)
		var other string
		db := retroOpenRO(t)
		if err := db.QueryRow(`SELECT it.id FROM items it JOIN issues i ON i.item_id = it.id
			WHERE it.kind = 'issue' AND it.id <> ? ORDER BY it.key LIMIT 1`, visited).Scan(&other); err != nil {
			t.Fatalf("pick other item: %v", err)
		}
		retroInjectVisit(t, a0, store.VisitKindIssue, vkey, store.VisitSourceCLI)
		retroInjectVisit(t, a0.Add(40*time.Minute), store.VisitKindIssue, vkey, store.VisitSourceCLI) // 40m gap: two sessions
		retroInjectChange(t, a0.Add(30*time.Second), other, "someone-else")                           // wrong issue: ignored
		retroInjectChange(t, a0.Add(time.Minute), visited, "someone-else")                            // visited issue: counts (unresolved self)

		rep := retroComputePinned(t, store.FeedIdentity{}, 21*24*time.Hour, now)
		b := retroBucketContaining(t, rep, a0)
		if !rep.cliFallback {
			t.Fatal("cliFallback should be true when only cli visits exist")
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
		defs := rep.definitions()
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
		retroFixtureHome(t)
		_, key := retroPickItem(t)
		retroInjectVisit(t, a0, store.VisitKindIssue, key, "")                                      // pre-V7 row: unknown, still the person
		retroInjectVisit(t, a0.Add(31*time.Minute), store.VisitKindIssue, key, store.VisitSourceUI) // 31m gap: two sessions

		rep := retroComputePinned(t, store.FeedIdentity{}, 21*24*time.Hour, now)
		b := retroBucketContaining(t, rep, a0)
		if rep.cliFallback {
			t.Fatal("unknown-source visits must not trigger the cli fallback")
		}
		if b.Sessions != 2 {
			t.Fatalf("sessions = %d, want 2 (unknown + ui are one person, 31m gap splits)", b.Sessions)
		}
	})
}
