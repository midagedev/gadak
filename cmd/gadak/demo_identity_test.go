package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDemoConfigCarriesTheFixtureAccountID closes GDK-1729 on the `gadak demo`
// side, and TestE2EServeDerivesTheAccountID on the e2e side.
//
// store.IsSelfActor matches on the account id (or a display name), and both
// configs carried only an email — so nothing in the served mirror was ever the
// reader's own write and the retro resume cell, one of four summary numbers,
// was a dash in the demo, in every recording and in every e2e run.
//
// Neither side writes the id down: both look it up in the mirror by the same
// email, so a regenerated fixture cannot leave a stale literal behind. That is
// what these two assert — the derivation, not a particular string.
//
// FAIL-first: against the pre-fix source the first failed with
// "demoAccountID undefined" (the helper did not exist and the config was the
// literal {"projects":[...]}) and the second with
// "e2e/serve.sh writes no account_id into the config".
func TestDemoConfigCarriesTheFixtureAccountID(t *testing.T) {
	fixture := filepath.Join("..", "..", "examples", "demo.db")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("demo fixture absent: %v", err)
	}
	id := demoAccountID(fixture)
	if id == "" {
		t.Fatalf("the fixture knows no account id for %s; the demo config would ship without one", demoUserEmail)
	}

	// It is the mirror's own pairing, not a literal: the same email has to
	// carry that id on a row of the fixture.
	db := openFixtureRO(t, fixture)
	var n int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM issues_full
		 WHERE assignee_email = ? AND assignee_id = ?`, demoUserEmail, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Errorf("account id %q is not paired with %s anywhere in the fixture", id, demoUserEmail)
	}

	// And it is the identity a write carries, which is the whole point: the
	// resume row counts comments and changelog entries by this id.
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments WHERE author_id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Errorf("no comment in the fixture is authored by %q, so the resume row stays empty", id)
	}
}

func TestE2EServeDerivesTheAccountID(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "e2e", "serve.sh"))
	if err != nil {
		t.Skipf("serve.sh absent: %v", err)
	}
	sh := string(src)
	if !strings.Contains(sh, `"account_id"`) {
		t.Fatal("e2e/serve.sh writes no account_id into the config")
	}
	// Derived from the served mirror, not written down: a literal id here is
	// the drift this issue is about.
	if !strings.Contains(sh, "E2E_ACCOUNT_ID=\"$(sqlite3") {
		t.Error("e2e/serve.sh does not read the account id out of the mirror it serves")
	}
	if !strings.Contains(sh, `"account_id": "$E2E_ACCOUNT_ID"`) {
		t.Error("e2e/serve.sh does not put the derived id into the config")
	}
}

func openFixtureRO(t *testing.T, path string) *sql.DB {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+abs+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
