package store

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// GDK-1438: an agent asks "what is mine?" without substituting a :me
// parameter, so identity has to live in the database. The views are the
// contract; identityMatch is the rule person-match.ts applies, in SQL.
func TestMyOpenAndHandedOffAnswerWithoutAMeParameter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := db.RecordWhoAmI(config.WhoAmI{AccountID: "acc-me", Email: "Me@Example.com"}); err != nil {
		t.Fatalf("record identity: %v", err)
	}
	if got := db.WhoAmI(); got.AccountID != "acc-me" || got.Email != "Me@Example.com" {
		t.Fatalf("identity not read back: %+v", got)
	}

	if _, err := db.sql.Exec(`INSERT INTO sources (id, kind) VALUES ('s1', 'jira')`); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	seed := func(key, statusCat, assigneeID, assigneeEmail, reporterID, reporterEmail string) {
		t.Helper()
		seedIdentityIssue(t, db, "s1", key, statusCat, assigneeID, assigneeEmail, reporterID, reporterEmail)
	}
	// Mine by id, open.
	seed("STD-1", "inprogress", "acc-me", "", "acc-other", "")
	// Mine by email, case-insensitive, open.
	seed("STD-2", "new", "", "ME@EXAMPLE.COM", "acc-other", "")
	// Mine but finished — status_category, never a display name.
	seed("STD-3", "done", "acc-me", "", "acc-other", "")
	// Somebody else's.
	seed("STD-4", "inprogress", "acc-other", "other@example.com", "acc-other", "")
	// Reported by me, held by someone else: handed off.
	seed("STD-5", "inprogress", "acc-other", "", "acc-me", "")
	// Reported by me and held by me: not handed off.
	seed("STD-6", "inprogress", "acc-me", "", "acc-me", "")
	// Reported by me, unassigned: still handed off (nobody is holding it).
	seed("STD-7", "new", "", "", "acc-me", "")

	keys := func(view string) []string {
		t.Helper()
		rows, err := db.sql.Query(`SELECT key FROM ` + view + ` ORDER BY key`)
		if err != nil {
			t.Fatalf("select %s: %v", view, err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, k)
		}
		return out
	}

	want := func(view string, exp ...string) {
		t.Helper()
		got := keys(view)
		if len(got) != len(exp) {
			t.Fatalf("%s = %v, want %v", view, got, exp)
		}
		for i := range exp {
			if got[i] != exp[i] {
				t.Fatalf("%s = %v, want %v", view, got, exp)
			}
		}
	}
	want("my_open", "STD-1", "STD-2", "STD-6")
	want("handed_off", "STD-5", "STD-7")

	// An identity that cannot be answered matches nothing — the honest
	// answer. '' must never match '' (localSchemaV8 stores blanks, not NULL).
	if err := db.RecordWhoAmI(config.WhoAmI{}); err != nil {
		t.Fatalf("clear identity: %v", err)
	}
	if got := keys("my_open"); len(got) != 0 {
		t.Fatalf("my_open with no identity = %v, want none", got)
	}
	if got := keys("handed_off"); len(got) != 0 {
		t.Fatalf("handed_off with no identity = %v, want none", got)
	}
}

// The actor slug is the built-in tracker's identity: it is stamped into the
// id column, not into an email, so it must match there.
func TestIdentityViewsMatchTheBuiltInActorSlug(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.RecordWhoAmI(config.WhoAmI{ActorSlug: "claude:code"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := db.sql.Exec(`INSERT INTO sources (id, kind) VALUES ('s1', 'builtin')`); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	seedIdentityIssue(t, db, "s1", "NMB-1", "inprogress", "claude:code", "", "someone", "")
	var n int
	if err := db.sql.QueryRow(`SELECT count(*) FROM my_open`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("my_open matched %d rows on the actor slug, want 1", n)
	}
}

// seedIdentityIssue writes one issue through the base tables the `issues`
// view is built from (items + issues_raw), the same shape flow_test.go seeds.
func seedIdentityIssue(t *testing.T, db *DB, sourceID, key, statusCat, assigneeID, assigneeEmail, reporterID, reporterEmail string) {
	t.Helper()
	itemID := sourceID + ":" + key
	if _, err := db.sql.Exec(`
		INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
		VALUES (?, ?, 'issue', ?, ?, ?, '2026-01-01', '2026-01-01', '2026-01-02')`,
		itemID, sourceID, key, key, key); err != nil {
		t.Fatalf("seed item %s: %v", key, err)
	}
	if _, err := db.sql.Exec(`
		INSERT INTO issues_raw (item_id, key, project_key, status_category, assignee_id, assignee_email, reporter_id, reporter_email, priority_rank, reopen_count, comment_count, raw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, '{}')`,
		itemID, key, key[:strings.Index(key, "-")], statusCat, assigneeID, assigneeEmail, reporterID, reporterEmail); err != nil {
		t.Fatalf("seed issue %s: %v", key, err)
	}
}
