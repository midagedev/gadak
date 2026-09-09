package sync

import (
	"context"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/atlhttp"
)

// TestFormatRequestBreakdown pins the pass line's exact shape (GDK-1672):
// canonical kind order, zero kinds omitted, one-decimal seconds, and the
// politeness sleep appended in the same seconds unit (GDK-1685).
func TestFormatRequestBreakdown(t *testing.T) {
	snap := atlhttp.BreakdownSnapshot{
		Kinds: []atlhttp.KindTally{
			{Kind: atlhttp.KindSearchPages, Count: 34, WallMS: 61200},
			{Kind: atlhttp.KindOther, Count: 7, WallMS: 1400},
		},
		Total:   41,
		WallMS:  62600,
		SleepMS: 45700,
	}
	want := "sync: requests  search-pages=34 (61.2s)  other=7 (1.4s)  total=41 (62.6s)  sleep=45.7s"
	if got := formatRequestBreakdown(snap); got != want {
		t.Errorf("line =\n%s\nwant\n%s", got, want)
	}

	noSleep := atlhttp.BreakdownSnapshot{
		Kinds:  []atlhttp.KindTally{{Kind: atlhttp.KindAgile, Count: 2, WallMS: 2100}},
		Total:  2,
		WallMS: 2100,
	}
	want = "sync: requests  agile=2 (2.1s)  total=2 (2.1s)"
	if got := formatRequestBreakdown(noSleep); got != want {
		t.Errorf("line =\n%s\nwant\n%s", got, want)
	}
}

// TestRunPrintsRequestBreakdown drives one full Jira pass against the fake
// site and asserts the request mix line landed in the pass log, taken from
// the client (a second pass prints a fresh line, not a cumulative one).
func TestRunPrintsRequestBreakdown(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	c := site.start()

	var logs []string
	log := func(s string) { logs = append(logs, s) }
	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: c, Log: log}); err != nil {
		t.Fatal(err)
	}
	line := findBreakdownLine(t, logs)
	if !strings.Contains(line, "search-pages=") {
		t.Errorf("line has no search-pages row: %s", line)
	}
	if !strings.Contains(line, "total=") {
		t.Errorf("line has no total: %s", line)
	}

	// Taken at pass end: a quiet second pass over the same watermark must
	// print its own line, not the first pass's counts again.
	logs = nil
	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Client: c, Log: log}); err != nil {
		t.Fatal(err)
	}
	second := findBreakdownLine(t, logs)
	var firstCount, secondCount string
	for _, f := range strings.Fields(second) {
		if strings.HasPrefix(f, "total=") {
			secondCount = f
		}
	}
	for _, f := range strings.Fields(line) {
		if strings.HasPrefix(f, "total=") {
			firstCount = f
		}
	}
	if firstCount == secondCount {
		t.Errorf("second pass total %s equals first pass %s — tally was not taken", secondCount, firstCount)
	}
}

func findBreakdownLine(t *testing.T, logs []string) string {
	t.Helper()
	for _, l := range logs {
		if strings.HasPrefix(l, "sync: requests") {
			return l
		}
	}
	t.Fatalf("no request breakdown line in %d log lines:\n%s", len(logs), strings.Join(logs, "\n"))
	return ""
}
