package main

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

// `gadak sprint show` is the CLI half of GDK-1710. The endpoint and the verb
// are one reconstruction (store.SprintBurnup) on purpose — CLI-first parity,
// decisions/0008 — so what this file pins is that the verb prints *that*
// series and nothing it recomputed:
//
//	C1 every day the store answers is a row, in order, with the store's
//	   own three numbers                                → TestSprintShowPrintsStoreSeries
//	C2 an id the mirror does not hold is a refusal that points at
//	   `sprint list`, not a table of zeros              → TestSprintShowRefusals
//	C3 a non-numeric id is a usage refusal before the store is opened
//	                                                    → TestSprintShowRefusals
//
// FAIL-first: on the pre-change tree cmd/gadak did not build at all —
// sprint.go's dispatch named an undefined sprintShow (`undefined:
// sprintShow`), so every test in this package was red.
func TestSprintShowPrintsStoreSeries(t *testing.T) {
	sqlDemoHome(t)
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	want, err := db.SprintBurnup(context.Background(), 41, time.Now())
	db.Close()
	if err != nil {
		t.Fatalf("store burn-up: %v", err)
	}
	if len(want.Days) == 0 {
		t.Fatalf("fixture sprint 41 has no days — the parity assertion would be vacuous")
	}

	out, err := capture(t, func() error { return cmdSprint([]string{"show", "41"}) })
	if err != nil {
		t.Fatalf("sprint show: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// header line, column header, then one row per day — no more, no fewer.
	if got, wantN := len(lines), len(want.Days)+2; got != wantN {
		t.Fatalf("printed %d lines, want %d (2 headers + %d days):\n%s", got, wantN, len(want.Days), out)
	}
	if !strings.Contains(lines[0], "sprint 41") {
		t.Errorf("first line does not name the sprint: %q", lines[0])
	}
	if lines[1] != "date\tscope\tstarted\tdone" {
		t.Errorf("column header = %q", lines[1])
	}
	for i, d := range want.Days {
		want := strings.Join([]string{
			d.Date, strconv.Itoa(d.Scope), strconv.Itoa(d.Started), strconv.Itoa(d.Completed),
		}, "\t")
		if lines[i+2] != want {
			t.Errorf("day %d: printed %q, store says %q", i, lines[i+2], want)
		}
	}
}

func TestSprintShowRefusals(t *testing.T) {
	sqlDemoHome(t)

	// C2: an id shaped right but absent — the mirror is asked, and the
	// answer points at where the ids are.
	out, err := capture(t, func() error { return cmdSprint([]string{"show", "999999"}) })
	if err == nil {
		t.Fatalf("show of an absent sprint succeeded:\n%s", out)
	}
	if !strings.Contains(err.Error(), "sprint list") {
		t.Errorf("refusal does not point at `gadak sprint list`: %v", err)
	}

	// C3: not a number, and not an empty argument list either.
	for _, arg := range []string{"seven", ""} {
		if _, err := capture(t, func() error { return cmdSprint([]string{"show", arg}) }); err == nil {
			t.Errorf("show %q succeeded, want a usage refusal", arg)
		}
	}
	if _, err := capture(t, func() error { return cmdSprint([]string{"show"}) }); err == nil {
		t.Error("show with no id succeeded, want a usage refusal")
	}
}
