package main

// gadak list / gadak ready. The default-list tests ride examples/demo.db
// (sqlDemoHome); the ready tests ride a real local-origin workspace the same
// way TestLocalOriginCreateSyncSQL does — init --standalone, create through
// the origin, link, transition — because ready's blocker filter resolves
// the link type against that origin's catalog, which is the path the
// product actually takes.
//
// Contract ↔ assertion map (r3 flow layer):
//   C2  blocking type by catalog, 'Blocks' fallback documented —
//       TestReadyDropsBlockedIssueAndRecovers (local origin fills link_types
//       via the write-through path; blockingLinkTypeNames' fallback half is
//       unit-tested in internal/store/flow_test.go)
//   C3  ready key set on the demo fixture unchanged by the column filter —
//       TestReadyColumnFilterMatchesOldRuleOnDemoFixture (FAIL-first: before
//       schemaV43 the new half of the query is "no such column:
//       open_blockers", and the old live path could not even run without a
//       credential — it degraded)
//   C4  open_blockers moves when a blocker's status changes without the
//       blocked issue being in the batch — TestReadyDropsBlockedIssueAndRecovers
//       (the write-through refresh carries the blocker only; the widening in
//       recomputeOpenBlockers must move the blocked row)
//   degradation — TestListReadySchemaLagDegradesToOpenList (FAIL-first
//       against pre-v43 code: the notice text and the guard did not exist;
//       the old code degraded with "blocking link type unresolved" instead)

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

const listHeader = "key\tpriority\tpriority_rank\tstatus\tage_days\tupdated_at\tsummary"

func listKeys(t *testing.T, out string) []string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 1 || lines[0] != listHeader {
		t.Fatalf("header = %q, want %q:\n%s", firstLine(out), listHeader, out)
	}
	keys := make([]string, 0, len(lines)-1)
	for _, ln := range lines[1:] {
		f := strings.Split(ln, "\t")
		if len(f) != 7 {
			t.Fatalf("row %q does not have 7 tab-separated fields:\n%s", ln, out)
		}
		keys = append(keys, f[0])
	}
	return keys
}

// doneAmong counts how many of keys are status_category 'done' in the
// mirror, read through a second connection the way a next gadak process
// would see it.
func doneAmong(t *testing.T, keys []string) int {
	t.Helper()
	if len(keys) == 0 {
		return 0
	}
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = "'" + strings.ReplaceAll(k, "'", "''") + "'"
	}
	var n int
	if err := localDB(t).QueryRow(
		`SELECT COUNT(*) FROM issues_full WHERE key IN (` + strings.Join(quoted, ",") + `) AND status_category = 'done'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestListDefaultIsOpenSortedByPriorityRank(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error { return cmdList(nil) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	keys := listKeys(t, out)
	if len(keys) != defaultListLimit {
		t.Fatalf("list printed %d rows, want the default %d:\n%s", len(keys), defaultListLimit, out)
	}
	if n := doneAmong(t, keys); n != 0 {
		t.Fatalf("%d done rows in the default list — done must be hidden:\n%s", n, out)
	}
	prev := -1
	for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n")[1:] {
		rank, err := strconv.Atoi(strings.Split(ln, "\t")[2])
		if err != nil {
			t.Fatalf("priority_rank column is not an integer in %q:\n%s", ln, out)
		}
		if rank < prev {
			t.Fatalf("priority_rank went %d → %d down the rows — the sort is broken:\n%s", prev, rank, out)
		}
		prev = rank
	}
}

func TestListLimitAllAndJSON(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error { return cmdList([]string{"--limit", "3"}) })
	if err != nil {
		t.Fatalf("list --limit 3: %v\n%s", err, out)
	}
	if got := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); got != 4 {
		t.Fatalf("--limit 3 printed %d lines, want header + 3 rows:\n%s", got, out)
	}

	all, err := capture(t, func() error { return cmdList([]string{"--all", "--limit", "500"}) })
	if err != nil {
		t.Fatalf("list --all: %v\n%s", err, all)
	}
	if n := doneAmong(t, listKeys(t, all)); n == 0 {
		t.Fatalf("--all showed no done issue — it must include them:\n%s", all)
	}

	jsonOut, err := capture(t, func() error { return cmdList([]string{"--json", "--limit", "2"}) })
	if err != nil {
		t.Fatalf("list --json: %v\n%s", err, jsonOut)
	}
	// --json is one object per row (the sql/recipes contract), not an array.
	dec := json.NewDecoder(strings.NewReader(jsonOut))
	var rows []map[string]any
	for {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("list --json is not a row stream: %v\n%s", err, jsonOut)
		}
		rows = append(rows, row)
	}
	if len(rows) != 2 {
		t.Fatalf("--json gave %d rows, want 2:\n%s", len(rows), jsonOut)
	}
	if _, ok := rows[0]["priority"].(string); !ok {
		t.Fatalf("priority missing from the JSON row — the sort must be checkable from the output:\n%v", rows[0])
	}
}

func TestListRejectsBadArgs(t *testing.T) {
	sqlDemoHome(t)
	for _, args := range [][]string{
		{"--limit", "0"},
		{"--limit", "-1"},
		{"--ready", "--all"},
		{"NMB-1"},
	} {
		_, err := capture(t, func() error { return cmdList(args) })
		if err == nil {
			t.Fatalf("list %v must be a usage error", args)
		}
	}
	_, err := capture(t, func() error { return cmdReady([]string{"--all"}) })
	if err == nil {
		t.Fatal("ready --all must be a usage error (ready is the open list minus blocked)")
	}
}

// localOriginHome is the TestLocalOriginCreateSyncSQL pattern: a throwaway
// GADAK_HOME with a real local-origin workspace behind it.
func localOriginHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
}

// createLocalOrigin creates one issue through the local-origin origin and
// returns its key. create refreshes the mirror, so the key is readable
// immediately after.
func createLocalOrigin(t *testing.T, summary string) string {
	t.Helper()
	out, err := capture(t, func() error { return cmdCreate([]string{summary}) })
	if err != nil {
		t.Fatalf("create %q: %v\n%s", summary, err, out)
	}
	return strings.Split(strings.TrimSpace(strings.Split(out, "\n")[0]), "\t")[0]
}

func TestReadyDropsBlockedIssueAndRecovers(t *testing.T) {
	localOriginHome(t)
	blocker := createLocalOrigin(t, "the blocker")
	blocked := createLocalOrigin(t, "the blocked one")
	if _, err := capture(t, func() error {
		return cmdLink([]string{blocker, blocked, "--type", "blocks"})
	}); err != nil {
		t.Fatalf("link %s blocks %s: %v", blocker, blocked, err)
	}

	list, err := capture(t, func() error { return cmdList(nil) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, list)
	}
	for _, key := range []string{blocker, blocked} {
		if !strings.Contains(list, key) {
			t.Fatalf("list missing %s — the seed drifted:\n%s", key, list)
		}
	}

	ready, _, err := captureBoth(t, func() error { return cmdReady(nil) })
	if err != nil {
		t.Fatalf("ready: %v\n%s", err, ready)
	}
	if !strings.Contains(ready, blocker) {
		t.Fatalf("ready dropped the unblocked issue %s:\n%s", blocker, ready)
	}
	if strings.Contains(ready, blocked) {
		t.Fatalf("ready showed %s, which %s blocks while unfinished:\n%s", blocked, blocker, ready)
	}

	// The blocker finishing is the whole point of ready: the blocked issue
	// re-enters the list without anything else changing.
	if _, err := capture(t, func() error { return cmdTransition([]string{blocker, "done"}) }); err != nil {
		t.Fatalf("transition %s done: %v", blocker, err)
	}
	readyAfter, _, err := captureBoth(t, func() error { return cmdReady(nil) })
	if err != nil {
		t.Fatalf("ready after done: %v\n%s", err, readyAfter)
	}
	if !strings.Contains(readyAfter, blocked) {
		t.Fatalf("%s is still missing from ready after its blocker finished:\n%s", blocked, readyAfter)
	}
}

func TestListReadyFlagMatchesReadyAlias(t *testing.T) {
	localOriginHome(t)
	createLocalOrigin(t, "unblocked either way")
	byFlag, _, err := captureBoth(t, func() error { return cmdList([]string{"--ready"}) })
	if err != nil {
		t.Fatalf("list --ready: %v\n%s", err, byFlag)
	}
	byAlias, _, err := captureBoth(t, func() error { return cmdReady(nil) })
	if err != nil {
		t.Fatalf("ready: %v\n%s", err, byAlias)
	}
	if byFlag != byAlias {
		t.Fatalf("list --ready and ready differ:\n--ready:\n%s\nready:\n%s", byFlag, byAlias)
	}
}

// sqlDemoHome saves no config, so on pre-v43 code origin.Client could not
// produce a catalog and ready degraded. Since v43 the filter is the
// open_blockers column; the one remaining degradation is a mirror whose
// schema predates the column (user_version < 43). The fixture is forced to
// that state — a genuine pre-v43 mirror takes the same branch through the
// same PRAGMA user_version gate — and stdout must be exactly the plain open
// list: "blockers not filtered", never "nothing is ready".
func TestListReadySchemaLagDegradesToOpenList(t *testing.T) {
	sqlDemoHome(t)
	path := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	w, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	var have int
	if err := w.QueryRow(`PRAGMA user_version`).Scan(&have); err != nil {
		t.Fatal(err)
	}
	if have >= openBlockersVersion {
		if _, err := w.Exec(`PRAGMA user_version = 42`); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()

	plain, err := capture(t, func() error { return cmdList([]string{"--limit", "5"}) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, plain)
	}
	ready, stderr, err := captureBoth(t, func() error { return cmdList([]string{"--ready", "--limit", "5"}) })
	if err != nil {
		t.Fatalf("list --ready must not fail on a pre-v43 mirror: %v", err)
	}
	if !strings.Contains(stderr, "mirror schema predates open_blockers") {
		t.Fatalf("stderr missing the schema-lag notice:\n%s", stderr)
	}
	if ready != plain {
		t.Fatalf("degraded ready must equal the plain open list:\nplain:\n%s\nready:\n%s", plain, ready)
	}
}

// TestReadyColumnFilterMatchesOldRuleOnDemoFixture is C3's proof that the
// column filter answers what the old live-catalog filter answered: the key
// set over the whole demo fixture is identical. The "old" half is the exact
// SQL the pre-v43 ready emitted once its catalog read resolved (a standard
// site's Blocks type); the fixture ships the standard vocabulary, so the
// comparison is exact, not approximate. Non-vacuous: the fixture must hold
// at least one blocked open issue, or the equality would hold on nothing.
// FAIL-first: before this round the migrated copy had no open_blockers
// column (the new half was "no such column") and the old verb degraded to
// the unfiltered list because no credential could reach a catalog.
func TestReadyColumnFilterMatchesOldRuleOnDemoFixture(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "gadak.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	// store.Open migrates the copy (v42→v43 runs backfillFlow) the way the
	// user's next `gadak sync` will; the CLI reads through openReadOnly,
	// which serves the file as-is.
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	oldRule := `select f.key from issues_full f
	  where f.status_category != 'done'
	    and not exists (select 1 from links l join issues_full b on b.key = l.target_key
	                    where l.item_id = f.item_id and l.type = 'Blocks'
	                      and l.direction = 'inward' and b.status_category != 'done')
	  order by f.key`
	newRule := `select f.key from issues_full f
	  where f.status_category != 'done' and f.open_blockers = 0
	  order by f.key`
	oldKeys := queryKeys(t, db, ctx, oldRule)
	newKeys := queryKeys(t, db, ctx, newRule)
	if fmt.Sprint(oldKeys) != fmt.Sprint(newKeys) {
		t.Fatalf("ready key set changed:\nold rule: %d keys %v\nnew rule: %d keys %v",
			len(oldKeys), oldKeys, len(newKeys), newKeys)
	}

	var blockedOpen int
	if err := db.QueryRowContext(ctx, `select count(*) from issues_full where status_category != 'done' and open_blockers > 0`).Scan(&blockedOpen); err != nil {
		t.Fatal(err)
	}
	if blockedOpen == 0 {
		t.Fatal("fixture has no blocked open issue — the equality above held on nothing")
	}
	db.Close()

	// The verb itself must produce the same set (wiring, not just SQL). The
	// CLI orders by priority_rank — set equality, not slice equality.
	out, err := capture(t, func() error { return cmdReady([]string{"--limit", "600"}) })
	if err != nil {
		t.Fatalf("ready: %v\n%s", err, out)
	}
	cliKeys := listKeys(t, out)
	sort.Strings(cliKeys)
	if fmt.Sprint(cliKeys) != fmt.Sprint(newKeys) {
		t.Fatalf("gadak ready key set differs from the column rule:\nready: %d keys %v\nrule: %d keys %v",
			len(cliKeys), cliKeys, len(newKeys), newKeys)
	}
}

func queryKeys(t *testing.T, db *store.DB, ctx context.Context, q string) []string {
	t.Helper()
	rows, err := db.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// next/pick on a fresh local-origin workspace: rows, not an error, with the
// notice on stderr. This is the completion criterion for the fallback.
func TestPickOnFreshLocalOriginReturnsRows(t *testing.T) {
	localOriginHome(t)
	key := createLocalOrigin(t, "pick fodder")
	stdout, stderr, err := captureBoth(t, func() error { return cmdNext(nil) })
	if err != nil {
		t.Fatalf("pick on a fresh workspace must answer: %v", err)
	}
	if !strings.Contains(stdout, key) {
		t.Fatalf("pick output missing the created issue %s:\n%s", key, stdout)
	}
	if !strings.Contains(stderr, `no saved recipe "next"`) {
		t.Fatalf("pick stderr missing the fallback notice:\n%s", stderr)
	}
}
