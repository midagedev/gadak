package main

// gadak list / gadak ready. The default-list tests ride examples/demo.db
// (sqlDemoHome); the ready tests ride a real standalone workspace the same
// way TestStandaloneCreateSyncSQL does — init --standalone, create through
// the origin, link, transition — because ready's blocker filter resolves
// the link type against that origin's catalog, which is the path the
// product actually takes.

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

const listHeader = "key\tpriority\tpriority_rank\tstatus\tupdated_at\tsummary"

func listKeys(t *testing.T, out string) []string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 1 || lines[0] != listHeader {
		t.Fatalf("header = %q, want %q:\n%s", firstLine(out), listHeader, out)
	}
	keys := make([]string, 0, len(lines)-1)
	for _, ln := range lines[1:] {
		f := strings.Split(ln, "\t")
		if len(f) != 6 {
			t.Fatalf("row %q does not have 6 tab-separated fields:\n%s", ln, out)
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

// standaloneHome is the TestStandaloneCreateSyncSQL pattern: a throwaway
// GADAK_HOME with a real standalone workspace behind it.
func standaloneHome(t *testing.T) {
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

// createStandalone creates one issue through the standalone origin and
// returns its key. create refreshes the mirror, so the key is readable
// immediately after.
func createStandalone(t *testing.T, summary string) string {
	t.Helper()
	out, err := capture(t, func() error { return cmdCreate([]string{summary}) })
	if err != nil {
		t.Fatalf("create %q: %v\n%s", summary, err, out)
	}
	return strings.Split(strings.TrimSpace(strings.Split(out, "\n")[0]), "\t")[0]
}

func TestReadyDropsBlockedIssueAndRecovers(t *testing.T) {
	standaloneHome(t)
	blocker := createStandalone(t, "the blocker")
	blocked := createStandalone(t, "the blocked one")
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
	standaloneHome(t)
	createStandalone(t, "unblocked either way")
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

// sqlDemoHome saves no config, so origin.Client cannot produce a catalog:
// the announced degradation path. stdout must be exactly the plain open
// list — "blockers not filtered", not "nothing is ready".
func TestListReadyUnresolvedDegradesToOpenList(t *testing.T) {
	sqlDemoHome(t)
	plain, err := capture(t, func() error { return cmdList([]string{"--limit", "5"}) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, plain)
	}
	ready, stderr, err := captureBoth(t, func() error { return cmdList([]string{"--ready", "--limit", "5"}) })
	if err != nil {
		t.Fatalf("list --ready must not fail on an unresolvable catalog: %v", err)
	}
	if !strings.Contains(stderr, "blocking link type unresolved") {
		t.Fatalf("stderr missing the unresolved-type notice:\n%s", stderr)
	}
	if ready != plain {
		t.Fatalf("degraded ready must equal the plain open list:\nplain:\n%s\nready:\n%s", plain, ready)
	}
}

// next/pick on a fresh standalone workspace: rows, not an error, with the
// notice on stderr. This is the completion criterion for the fallback.
func TestPickOnFreshStandaloneReturnsRows(t *testing.T) {
	standaloneHome(t)
	key := createStandalone(t, "pick fodder")
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
