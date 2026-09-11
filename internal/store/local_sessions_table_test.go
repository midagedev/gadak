package store

// local.sessions (GDK-1439) — the maintained table that owns the session
// boundary. The clauses, one test each:
//
//	T1 maintenance folds/extends — TestRecordVisitMaintainsSessions
//	     ① two person reads within the gap → one row, visits=2, ended extends
//	     ② a cli read never touches the table
//	     ③ a person read past the gap opens a new row
//	T2 V10 backfill chains, not one-row-per-visit —
//	     TestLocalV9MigratesToV10BackfillsSessions
//	     ① two person clusters + cli noise → two chained session rows
//	     ② visits counts and ended_at are the cluster's
//	     ③ first_write_at stays empty (the ledger did not exist then)
//	T3 the boundary read answers from the table —
//	     TestLastSessionEndReadsSessionsTable
//	     ① newest session open → boundary is the session before it
//	     ② newest closed → boundary is the newest session's end
//	     ③ table answer == walk answer over the same visit stream
//	T4 the write ledger + first_write_at — TestRecordAgentWriteLedgerAndFirstWrite
//	     ① one row per accepted write (key, verb, source)
//	     ② first_write_at stamps the open session, earliest wins
//	     ③ a write with no open session never invents a session row
//	T5 validation — TestRecordAgentWriteValidation
//	     ① empty key / empty verb / bad source → error, no rows left
//	T6 retention — TestPruneLocalHistoryDropsOldSessionsAndWrites
//	     ① sessions prune by ended_at, agent_writes by at, recent stay
//
// FAIL-first evidence (pre-change tree at 1e8993ab, extracted read-only via
// git archive): every test here fails there with "no such table:
// local.sessions" / "no such table: local.agent_writes" (recordAgentWrite
// does not exist) — outputs in the round scratchpad,
// sessions-table-failfirst.out.

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// sessionsOf reads every session row of the current epoch, oldest first.
func sessionsOf(t *testing.T, db *DB) []Session {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(),
		`SELECT started_at, ended_at, first_write_at, visits FROM local.sessions
			WHERE origin_epoch = `+currentEpochSQLOnLocal+` ORDER BY started_at`)
	if err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.StartedAt, &s.EndedAt, &s.FirstWriteAt, &s.Visits); err != nil {
			t.Fatalf("scan session: %v", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	return out
}

func TestRecordVisitMaintainsSessions(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	// ① Two person reads back-to-back: one session row, visits=2, the second
	// read's stamp is the row's end. (RecordVisit refuses the legacy ""
	// source — that spelling is a historical row value, exercised in the V9
	// backfill test below through raw SQL.)
	if _, err := db.RecordVisit(ctx, VisitKindIssue, "STD-1", VisitSourceUI); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(ctx, VisitKindIssue, "STD-2", VisitSourceUI); err != nil {
		t.Fatal(err)
	}
	rows := sessionsOf(t, db)
	if len(rows) != 1 {
		t.Fatalf("session rows = %d, want 1 (two reads within the gap fold)", len(rows))
	}
	if rows[0].Visits != 2 {
		t.Fatalf("visits = %d, want 2", rows[0].Visits)
	}
	if rows[0].EndedAt == rows[0].StartedAt {
		t.Fatalf("ended_at = started_at (%s) — the second read did not extend the row", rows[0].EndedAt)
	}

	// ② An agent read: the ledger of *person* sessions does not move.
	if _, err := db.RecordVisit(ctx, VisitKindIssue, "STD-3", VisitSourceCLI); err != nil {
		t.Fatal(err)
	}
	after := sessionsOf(t, db)
	if len(after) != 1 || after[0].Visits != 2 {
		t.Fatalf("cli read changed sessions: %+v", after)
	}

	// ③ A person read past the gap opens a new row. RecordVisit stamps
	// Now(), so the gap is created by aging the existing row backwards.
	old := time.Now().UTC().Add(-2 * time.Hour).Format(config.ISOMilli)
	if _, err := db.sql.ExecContext(ctx,
		`UPDATE local.sessions SET started_at = ?, ended_at = ?`, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(ctx, VisitKindIssue, "STD-4", VisitSourceUI); err != nil {
		t.Fatal(err)
	}
	rows = sessionsOf(t, db)
	if len(rows) != 2 {
		t.Fatalf("session rows = %d, want 2 (a read past the gap opens a row)", len(rows))
	}
	if rows[1].Visits != 1 || rows[1].FirstWriteAt != "" {
		t.Fatalf("new row = %+v, want visits=1 and an empty first_write_at", rows[1])
	}
}

// TestLocalV9MigratesToV10BackfillsSessions builds the file a pre-sessions
// build leaves behind — migrations 1..9 applied, person and agent visits —
// and runs the EnsureLocal path Open uses. The chaining is the point: a
// backfill that writes one row per visit would make the boundary read
// answer "second-newest read" (the first version of this backfill did
// exactly that; the test was written to fail on it).
func TestLocalV9MigratesToV10BackfillsSessions(t *testing.T) {
	dir := t.TempDir()
	mirror := filepath.Join(dir, "gadak.db")
	local := LocalPath(mirror)

	raw, err := sql.Open("sqlite", "file:"+local)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range localMigrations[:9] {
		if _, err := raw.Exec(m); err != nil {
			t.Fatalf("apply pre-V10 migration: %v", err)
		}
	}
	// Two person clusters separated by a >gap silence, with agent reads
	// sprinkled in — agent reads must neither join nor split a session.
	ins := func(at, source string) {
		t.Helper()
		if _, err := raw.Exec(`INSERT INTO visits (kind, key, viewed_at, source) VALUES ('issue','STD-1',?,?)`, at, source); err != nil {
			t.Fatal(err)
		}
	}
	ins("2026-09-01T10:00:00.000Z", "ui")
	ins("2026-09-01T10:05:00.000Z", "")
	ins("2026-09-01T10:20:00.000Z", "cli")
	ins("2026-09-01T10:25:00.000Z", "ui")
	ins("2026-09-01T14:00:00.000Z", "cli")
	ins("2026-09-01T16:00:00.000Z", "ui")
	ins("2026-09-01T16:10:00.000Z", "ui")
	// A retired-epoch person read: not this origin's session.
	if _, err := raw.Exec(`INSERT INTO visits (kind, key, viewed_at, source, origin_epoch) VALUES ('issue','STD-2','2026-09-01T12:00:00.000Z','ui',99)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 9`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := EnsureLocal(mirror); err != nil {
		t.Fatalf("EnsureLocal on a V9 local.db: %v", err)
	}
	db, err := Open(mirror)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sessionsOf(t, db)
	if len(rows) != 2 {
		t.Fatalf("session rows after backfill = %d, want 2 (clusters chained, not one row per visit):\n%+v", len(rows), rows)
	}
	want := []Session{
		{StartedAt: "2026-09-01T10:00:00.000Z", EndedAt: "2026-09-01T10:25:00.000Z", FirstWriteAt: "", Visits: 3},
		{StartedAt: "2026-09-01T16:00:00.000Z", EndedAt: "2026-09-01T16:10:00.000Z", FirstWriteAt: "", Visits: 2},
	}
	for i, w := range want {
		if rows[i] != w {
			t.Fatalf("session[%d] = %+v, want %+v", i, rows[i], w)
		}
	}
}

func TestLastSessionEndReadsSessionsTable(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	gap := 30 * time.Minute

	seedSessions := func(t *testing.T, db *DB, rows ...[2]string) {
		t.Helper()
		for _, r := range rows {
			if _, err := db.sql.ExecContext(ctx,
				`INSERT INTO local.sessions (started_at, ended_at, visits) VALUES (?,?,1)`, r[0], r[1]); err != nil {
				t.Fatal(err)
			}
		}
	}

	// ① Newest session still open (ended a minute ago): the boundary is the
	// session before it, not the open session's own end.
	db := openTemp(t)
	prev := now.Add(-2 * time.Hour)
	seedSessions(t, db,
		[2]string{stamp(now.Add(-72 * time.Hour)), stamp(now.Add(-72*time.Hour + 5*time.Minute))},
		[2]string{stamp(prev), stamp(prev.Add(5 * time.Minute))},
		[2]string{stamp(now.Add(-6 * time.Minute)), stamp(now.Add(-time.Minute))},
	)
	end, err := db.LastSessionEnd(ctx, now, gap)
	if err != nil {
		t.Fatal(err)
	}
	want := prev.Add(5 * time.Minute)
	if end == nil || !end.Equal(want) {
		t.Fatalf("boundary = %v, want %v (the session before the open one)", end, want)
	}

	// ② Newest session closed (its end is past the gap): the boundary is
	// that session's own end.
	db2 := openTemp(t)
	closed := now.Add(-3 * time.Hour)
	seedSessions(t, db2,
		[2]string{stamp(now.Add(-72 * time.Hour)), stamp(now.Add(-72*time.Hour + 5*time.Minute))},
		[2]string{stamp(closed), stamp(closed.Add(5 * time.Minute))},
	)
	end2, err := db2.LastSessionEnd(ctx, now, gap)
	if err != nil {
		t.Fatal(err)
	}
	want2 := closed.Add(5 * time.Minute)
	if end2 == nil || !end2.Equal(want2) {
		t.Fatalf("boundary = %v, want %v (the newest closed session's end)", end2, want2)
	}

	// ③ Equivalence: over the same visit stream the table's answer equals
	// the walk's. First ask with no session rows (the fallback walk), then
	// backfill and ask again.
	db3 := openTemp(t)
	addVisit(t, db3, now.Add(-72*time.Hour), VisitSourceUI)
	addVisit(t, db3, now.Add(-72*time.Hour+5*time.Minute), "")
	aEnd := now.Add(-41 * time.Minute)
	addVisit(t, db3, now.Add(-66*time.Minute), VisitSourceUI)
	addVisit(t, db3, aEnd, VisitSourceUI)
	addVisit(t, db3, now.Add(-time.Hour), VisitSourceCLI)
	addVisit(t, db3, now.Add(-time.Minute), VisitSourceUI)
	walkEnd, err := db3.LastSessionEnd(ctx, now, gap)
	if err != nil {
		t.Fatal(err)
	}
	if walkEnd == nil || !walkEnd.Equal(aEnd) {
		t.Fatalf("walk boundary = %v, want %v", walkEnd, aEnd)
	}
	if err := db3.withLocalWrite(ctx, func(tx *sql.Tx) error {
		return backfillSessionsTx(ctx, tx)
	}); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if got := sessionsOf(t, db3); len(got) != 3 {
		t.Fatalf("backfill over the equivalence stream made %d sessions, want 3 (09-03 pair, 10:54–11:19, and the 11:59 read 40m later):\n%+v", len(got), got)
	}
	tableEnd, err := db3.LastSessionEnd(ctx, now, gap)
	if err != nil {
		t.Fatal(err)
	}
	if tableEnd == nil || !tableEnd.Equal(*walkEnd) {
		t.Fatalf("table boundary = %v, want %v (the walk's answer over the same stream)", tableEnd, walkEnd)
	}
}

func TestRecordAgentWriteLedgerAndFirstWrite(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	// ① A landed write is one ledger row with the verb and source intact.
	if _, err := db.RecordVisit(ctx, VisitKindIssue, "STD-1", VisitSourceUI); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordAgentWrite(ctx, "STD-1", "comment", VisitSourceCLI); err != nil {
		t.Fatal(err)
	}
	var key, at, verb, source string
	if err := db.sql.QueryRowContext(ctx,
		`SELECT key, at, verb, source FROM local.agent_writes`).Scan(&key, &at, &verb, &source); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if key != "STD-1" || verb != "comment" || source != VisitSourceCLI || at == "" {
		t.Fatalf("ledger row = %q %q %q %q", key, at, verb, source)
	}

	// ② first_write_at stamps the open session, and the earliest wins.
	first := at
	if err := db.RecordAgentWrite(ctx, "STD-1", "transition", VisitSourceCLI); err != nil {
		t.Fatal(err)
	}
	rows := sessionsOf(t, db)
	if len(rows) != 1 {
		t.Fatalf("session rows = %d, want 1", len(rows))
	}
	if rows[0].FirstWriteAt != first {
		t.Fatalf("first_write_at = %q, want %q (earliest write)", rows[0].FirstWriteAt, first)
	}

	// ③ A write with no open session is still a ledger row — and never
	// invents a session row for itself.
	old := time.Now().UTC().Add(-2 * time.Hour).Format(config.ISOMilli)
	if _, err := db.sql.ExecContext(ctx,
		`UPDATE local.sessions SET started_at = ?, ended_at = ?`, old, old); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordAgentWrite(ctx, "STD-2", "edit", VisitSourceMCP); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM local.sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("sessions after a write with no open session = %d, want 1 (no invented row)", n)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM local.agent_writes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("ledger rows = %d, want 3", n)
	}
}

func TestRecordAgentWriteValidation(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for _, tc := range []struct {
		key, verb, source, want string
	}{
		{"", "comment", VisitSourceCLI, "key required"},
		{"STD-1", "", VisitSourceCLI, "verb required"},
		{"STD-1", "comment", "web", `source must be "cli", "ui" or "mcp"`},
	} {
		err := db.RecordAgentWrite(ctx, tc.key, tc.verb, tc.source)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("RecordAgentWrite(%q,%q,%q) err = %v, want %q", tc.key, tc.verb, tc.source, err, tc.want)
		}
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM local.agent_writes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rejected writes left %d rows, want 0", n)
	}
}

func TestPruneLocalHistoryDropsOldSessionsAndWrites(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-200 * 24 * time.Hour).Format(config.ISOMilli)
	recent := Now()
	if _, err := db.sql.ExecContext(ctx,
		`INSERT INTO local.sessions (started_at, ended_at, first_write_at, visits) VALUES (?,?,?,1),(?,?,?,2)`,
		old, old, old, recent, recent, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx,
		`INSERT INTO local.agent_writes (key, at, verb, source) VALUES ('OLD-1', ?, 'edit', 'cli'), ('NEW-1', ?, 'edit', 'cli')`,
		old, recent); err != nil {
		t.Fatal(err)
	}
	if err := db.PruneLocalHistory(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM local.sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("sessions after prune = %d, want 1 (old ended_at gone, recent stays)", n)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM local.agent_writes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("agent_writes after prune = %d, want 1 (old at gone, recent stays)", n)
	}
}

// stamp is the test's ISOMilli spelling of t.
func stamp(t time.Time) string { return t.UTC().Format(config.ISOMilli) }
