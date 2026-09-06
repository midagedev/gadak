package main

// gadak retro — contract ↔ assertion map. Each clause of the delegation spec
// names its test here (or the package that owns it), two assertions minimum:
// the happy path and the violation or boundary beside it.
//
//	C1 table and --json agree on demo.db, byte-identical
//	   after the move to internal/retro (plus the keys
//	   object)                               → TestRetroDemoDBTableAndJSONAgree
//	C2 every count equals its key set's
//	   length                                → internal/retro TestRetroCountsEqualKeyLengths
//	   (and per-bucket in JSON here)
//	C3 --open reuses the views open --keys path → TestRetroOpenCell +
//	   views_test.go TestViewsOpenKeysJSON (unchanged and green)
//	C4 no display-name keying                 → internal/retro TestRetroSourceKeysNeverOnDisplayName
//	C5 definitions every run, no second
//	   person in the output                   → TestRetroDefinitionsAndGrammar
//	C6 --since validation, --json shape,
//	   unknown flag is a usage error          → TestRetroSinceValidation (+ shape in C1)
//	C7 a V6 local.db migrates to 7             → internal/store TestLocalV6MigratesToV7KeepsOldRows
//	C8 help and usage cover retro              → help_test.go TestHelpCoversAllCommands,
//	                                            TestUsageListsEveryCommand

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
	"github.com/midagedev/gadak/internal/retro"
)

// retroSixRows are the row names of the table and the keys of one JSON bucket,
// in print order.
var retroSixRows = []string{
	"sessions", "resume (median)", "wip age p85", "in progress", "closed", "mismatch",
}

// retroSeedCatalog fills status_catalog the way a sync does, from the issues
// rows themselves. The shipped demo fixture carries zero rows; the closed and
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
	// JSON bucket shape: from/to/partial + the six rows + the keys object.
	first := jsonBuckets[0]
	if len(first) != 10 {
		t.Fatalf("bucket object has %d keys, want 10: %v", len(first), first)
	}
	for _, name := range append([]string{"from", "to", "partial", "keys"}, retroSixRows...) {
		if _, ok := first[name]; !ok {
			t.Fatalf("bucket object lacks key %q: %v", name, first)
		}
	}
	keysObj, ok := first["keys"].(map[string]any)
	if !ok {
		t.Fatalf("keys is not an object: %v", first["keys"])
	}
	for _, name := range []string{"closed", "in progress", "mismatch"} {
		if _, ok := keysObj[name].([]any); !ok {
			t.Fatalf("keys object lacks array %q: %v", name, keysObj)
		}
	}

	// Cell-by-cell: the table and the JSON document are the same numbers,
	// and every count is the length of its key array (C2 over the wire).
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
		if v, ok := num("resume (median)"); ok && resumeCell != retro.FormatSeconds(v) ||
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
		keys := b["keys"].(map[string]any)
		keyLen := func(name string) int { return len(keys[name].([]any)) }
		if v, ok := num("closed"); ok && keyLen("closed") != int(v) && keyLen("closed") != retro.MaxJSONKeys {
			t.Fatalf("bucket %d closed = %d, %d keys", bi, int(v), keyLen("closed"))
		}
		if v, ok := num("in progress"); ok && keyLen("in progress") != int(v) && keyLen("in progress") != retro.MaxJSONKeys {
			t.Fatalf("bucket %d in progress = %d, %d keys", bi, int(v), keyLen("in progress"))
		}
		if v, ok := num("mismatch"); !ok || keyLen("mismatch") != int(v) && keyLen("mismatch") != retro.MaxJSONKeys {
			t.Fatalf("bucket %d mismatch = %d, %d keys", bi, int(v), keyLen("mismatch"))
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
	// The heuristic row says it is one, in the footer, every run. The
	// wording follows the rule: since the guards landed (2026-09-06) the
	// match is a standing-alone done word with negations and quotes
	// excluded, not plain containment, and a reader deciding whether to
	// trust the count needs the footer to say so.
	if !strings.Contains(table, "(heuristic: ") || !strings.Contains(table, "negations and quoted text excluded)") {
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
		got, err := retro.ParseSince(in)
		if err != nil || got != want {
			t.Errorf("retro.ParseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"3x", "", "0d", "366d", "53w", "14", "d", "-1d", "1.5d", "w"} {
		if got, err := retro.ParseSince(in); err == nil {
			t.Errorf("retro.ParseSince(%q) = %v, want an error", in, got)
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

// TestRetroOpenCell is Part B: --open follows one cell to the issues behind
// it through the views open --keys path (C3). FAIL-first: before this round
// the flag did not exist and every subtest died on the unknown flag.
func TestRetroOpenCell(t *testing.T) {
	t.Run("--week without --open is a usage error", func(t *testing.T) {
		out, err := capture(t, func() error { return cmdRetro([]string{"--week", "1"}) })
		if err == nil || !strings.Contains(err.Error(), "--week only applies to --open") {
			t.Fatalf("--week alone: %v (out %q)", err, out)
		}
	})
	t.Run("--no-open without --open is a usage error", func(t *testing.T) {
		out, err := capture(t, func() error { return cmdRetro([]string{"--no-open"}) })
		if err == nil || !strings.Contains(err.Error(), "--no-open only applies to --open") {
			t.Fatalf("--no-open alone: %v (out %q)", err, out)
		}
	})
	t.Run("--open value is validated naming the three cells", func(t *testing.T) {
		out, err := capture(t, func() error { return cmdRetro([]string{"--open", "bogus"}) })
		if err == nil || !strings.Contains(err.Error(), "closed, in-progress, mismatch") {
			t.Fatalf("--open bogus: %v (out %q)", err, out)
		}
	})
	t.Run("week out of range names the range", func(t *testing.T) {
		sqlDemoHome(t)
		out, err := capture(t, func() error { return cmdRetro([]string{"--open", "closed", "--week", "5"}) })
		// The range's top depends on the weekday the test runs on; the error
		// must name the range and the offending week, whatever the top is.
		if err == nil || !strings.Contains(err.Error(), "--week 5 is out of range 0..") {
			t.Fatalf("out of range: %v (out %q)", err, out)
		}
	})
	t.Run("--json --open prints the keys document only", func(t *testing.T) {
		sqlDemoHome(t)
		out, err := capture(t, func() error {
			return cmdRetro([]string{"--open", "in-progress", "--json"})
		})
		if err != nil {
			t.Fatalf("open --json: %v\n%s", err, out)
		}
		var doc struct {
			Metric string   `json:"metric"`
			Week   int      `json:"week"`
			Keys   []string `json:"keys"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("keys document decode: %v\n%s", err, out)
		}
		if doc.Metric != "in-progress" || doc.Week != 0 {
			t.Fatalf("metric/week = %q/%d, want in-progress/0\n%s", doc.Metric, doc.Week, out)
		}
		if len(doc.Keys) == 0 {
			t.Fatalf("current partial week has in-progress keys on the demo fixture:\n%s", out)
		}
		if strings.Contains(out, "hash") {
			t.Fatalf("--json --open must not open or print a hash:\n%s", out)
		}
	})
	t.Run("--open --no-open writes the hash through the views open path", func(t *testing.T) {
		sqlDemoHome(t)
		out, err := capture(t, func() error {
			return cmdRetro([]string{"--open", "in-progress", "--week", "0", "--no-open"})
		})
		if err != nil {
			t.Fatalf("open --no-open: %v\n%s", err, out)
		}
		if !strings.Contains(out, "hash\t") {
			t.Fatalf("no hash line:\n%s", out)
		}
	})
	t.Run("empty cell says so on stderr and exits 0", func(t *testing.T) {
		sqlDemoHome(t)
		// Ask the report which mismatch cell is empty instead of asserting a
		// fact about the fixture's newest comment. The old premise — "the
		// fixture's comments end 2026-08-31, so week 0 never carries one" —
		// was both calendar-transient and incompatible with `make
		// demo-fixture`, which every schema migration runs: the v43
		// regeneration moved the newest comment into week 0 and turned this
		// green test red without a line of retro code changing. (2026-09-06,
		// r3-flow-layer review.)
		week := emptyMismatchWeek(t)
		stdout, stderr, err := captureBoth(t, func() error {
			return cmdRetro([]string{"--open", "mismatch", "--week", strconv.Itoa(week)})
		})
		if err != nil {
			t.Fatalf("empty cell must exit 0, got %v (out %q, err %q)", err, stdout, stderr)
		}
		if !strings.Contains(stderr, "retro: mismatch in week ") || !strings.Contains(stderr, "has no issues") {
			t.Fatalf("empty-cell stderr missing: %q (stdout %q)", stderr, stdout)
		}
		if stdout != "" {
			t.Fatalf("empty cell must open nothing and print nothing: %q", stdout)
		}
	})
}

// emptyMismatchWeek returns a week index whose mismatch bucket holds no keys,
// read from `retro --json` on whatever fixture is in the tree. Skips when
// every week carries one — that is a fixture worth knowing about, not a
// failure of the empty-cell behaviour under test.
func emptyMismatchWeek(t *testing.T) int {
	t.Helper()
	out, err := capture(t, func() error { return cmdRetro([]string{"--json"}) })
	if err != nil {
		t.Fatalf("retro --json: %v\n%s", err, out)
	}
	var doc struct {
		Buckets []struct {
			Keys map[string][]string `json:"keys"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("retro --json is not a document: %v\n%s", err, out)
	}
	// Same mapping cmdRetro uses: week 0 is the last bucket, 1 the one before.
	for i := len(doc.Buckets) - 1; i >= 0; i-- {
		if len(doc.Buckets[i].Keys["mismatch"]) == 0 {
			return len(doc.Buckets) - 1 - i
		}
	}
	t.Skip("every week in this fixture carries a mismatch; no empty cell to exercise")
	return 0
}
