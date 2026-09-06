package retro

// ClaimStands and the mismatch recency guard — contract ↔ assertion map.
//
// The rule (retro.go ClaimStands, mirrored by web/src/lib/done-words.ts):
//
//	1. no usable comment stamp        → false (cannot be shown to be newer)
//	2. no recorded status change      → true  (nothing answered the claim)
//	   (empty or unparseable stamp)
//	3. comment newer than the change  → true  (strictly; equal is not newer)
//	   comment equal to or older      → false (the change answered it)
//
// FAIL-first: the pre-guard mismatch loop counted every done-word comment on
// a not-done issue, so the "older" row below — a claim the issue's own status
// change already answered — was counted as a live mismatch. That is exactly
// the stale-claim false positive the guard removes; the wiring test pins it
// end to end on the demo fixture.

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// TestClaimStandsTruthTable walks every row of the rule above.
func TestClaimStandsTruthTable(t *testing.T) {
	at := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		commentOK bool
		commentAt time.Time
		changedAt string
		want      bool
	}{
		// rule 1
		{"no usable comment stamp", false, at, at.Format(config.ISOMilli), false},
		// rule 2
		{"empty status change", true, at, "", true},
		{"unparseable status change", true, at, "yesterday-ish", true},
		// rule 3
		{"comment after the change", true, at.Add(time.Hour), at.Format(config.ISOMilli), true},
		{"comment before the change", true, at.Add(-time.Hour), at.Format(config.ISOMilli), false},
		{"comment equal to the change", true, at, at.Format(config.ISOMilli), false},
		// mixed formats parse through the same ladder
		{"rfc3339 status stamp", true, at.Add(time.Minute), at.Format(time.RFC3339), true},
	}
	for _, c := range cases {
		if got := ClaimStands(c.commentAt, c.commentOK, c.changedAt); got != c.want {
			t.Errorf("%s: ClaimStands = %v, want %v", c.name, got, c.want)
		}
	}
}

// injectComment writes one comment row on an existing item, the fixture
// sibling of injectVisit/injectChange.
func injectComment(t *testing.T, dir string, at time.Time, item, body string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO comments (id, item_id, created_at, author, author_id, body_text)
		VALUES (?, ?, ?, 'Retro Fixture', 'retro-fixture', ?)`,
		"retro-fx-comment-"+at.UTC().Format("150405.000"), item, at.UTC().Format(config.ISOMilli), body); err != nil {
		t.Fatalf("inject comment: %v", err)
	}
}

// pickInProgressWithChange returns an in-progress issue whose
// status_changed_at parses and sits inside the pinned window with an hour of
// headroom on both sides, so a comment can land before and after it.
func pickInProgressWithChange(t *testing.T, db *sql.DB, first, now time.Time) (item string, changed time.Time) {
	t.Helper()
	rows, err := db.Query(`SELECT i.item_id, i.status_changed_at FROM issues i
		WHERE i.status_category = 'inprogress' AND COALESCE(i.status_changed_at,'') <> ''
		ORDER BY i.status_changed_at DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, at string
		if err := rows.Scan(&id, &at); err != nil {
			t.Fatal(err)
		}
		ts, ok := parseTime(at)
		if !ok {
			continue
		}
		if ts.Before(first.Add(2*time.Hour)) || ts.After(now.Add(-2*time.Hour)) {
			continue
		}
		return id, ts
	}
	t.Fatal("no in-progress issue with an in-window status change on this fixture")
	return
}

// TestMismatchRecencyGuardWiring — contract ↔ assertion map:
//
//	A1 stale claim: a done-word comment 1h before the issue's last status
//	   change adds nothing to mismatch (delta 0 against the same fixture
//	   without the injection)
//	A2 live claim: the same comment 1h after the change adds exactly 1
//	A3 the footer names the rule
//
// FAIL-first: A1 is the row the pre-guard loop miscounted — it counted the
// stale comment (delta would be 1), which is the defect this round closes.
func TestMismatchRecencyGuardWiring(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	since := 21 * 24 * time.Hour
	first := Buckets(now, since)[0].From

	baseline := func(t *testing.T, dir string, db *sql.DB, at time.Time) int {
		t.Helper()
		rep := computePinned(t, db, store.FeedIdentity{}, since, now)
		b := bucketContaining(t, rep, at)
		return b.Mismatch
	}

	t.Run("stale claim predating the status change does not count", func(t *testing.T) {
		dir, db := demoFixture(t, false)
		item, changed := pickInProgressWithChange(t, db, first, now)
		stale := changed.Add(-time.Hour)
		before := baseline(t, dir, db, stale)
		injectComment(t, dir, stale, item, "done — shipped to staging")
		rep := computePinned(t, db, store.FeedIdentity{}, since, now)
		got := bucketContaining(t, rep, stale).Mismatch
		if got != before {
			t.Fatalf("stale comment moved mismatch %d → %d; the pre-change loop would have counted it (the FAIL-first row)", before, got)
		}
	})

	t.Run("live claim newer than the status change counts once", func(t *testing.T) {
		dir, db := demoFixture(t, false)
		item, changed := pickInProgressWithChange(t, db, first, now)
		fresh := changed.Add(time.Hour)
		before := baseline(t, dir, db, fresh)
		injectComment(t, dir, fresh, item, "done — shipped to staging")
		rep := computePinned(t, db, store.FeedIdentity{}, since, now)
		got := bucketContaining(t, rep, fresh).Mismatch
		if got != before+1 {
			t.Fatalf("fresh comment moved mismatch %d → %d, want +1", before, got)
		}
		if !HasDoneWord("done — shipped to staging") {
			t.Fatal("control failed: the injected body must be a done-word claim")
		}
	})

	t.Run("the footer names the recency rule", func(t *testing.T) {
		_, db := demoFixture(t, false)
		rep := computePinned(t, db, store.FeedIdentity{}, since, now)
		var line string
		for _, d := range rep.Definitions() {
			if d[0] == "mismatch" {
				line = d[1]
			}
		}
		if !strings.Contains(line, "only comments newer than the issue's last status change count") {
			t.Fatalf("mismatch definition lacks the recency clause: %q", line)
		}
	})
}

// The guard must not change HasDoneWord's own verdicts — the two layers sit
// beside each other, not inside each other: the word matcher carries the
// negation guards (doneword_test.go), this file's guard is about time.
