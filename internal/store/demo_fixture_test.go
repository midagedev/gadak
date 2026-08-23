package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestCommittedDemoDBMatchesCurrentSchema is the GDK-671 lockstep gate.
// examples/demo.db is opened read-only on a throwaway byte copy — never via
// Open — because Open migrates and would hide a lagging committed file
// (e2e/serve.sh did that, and go tests that copy-then-Open still do).
//
// Schema owner is len(migrations) / SchemaVersion of a fresh Open, not a
// literal. Derived-table counts are measured on the same unread-migrated
// copy so a snapshot regen that drops item_refs (GDK-114) fails here
// instead of in CI Playwright.
func TestCommittedDemoDBMatchesCurrentSchema(t *testing.T) {
	src := filepath.Join("..", "..", "examples", "demo.db")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("examples/demo.db is part of the tree: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(path, in, 0o600); err != nil {
		t.Fatal(err)
	}

	// mode=ro: no WAL sidecars, no migrate, no FTS repair.
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open demo copy read-only: %v", err)
	}
	defer raw.Close()

	var have int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&have); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}

	fresh, err := Open(filepath.Join(t.TempDir(), "level.db"))
	if err != nil {
		t.Fatalf("open empty mirror for current schema: %v", err)
	}
	want := fresh.SchemaVersion()
	_ = fresh.Close()
	if want != len(migrations) {
		t.Fatalf("SchemaVersion() = %d, len(migrations) = %d — they are the same owner", want, len(migrations))
	}

	if have != want {
		t.Errorf("examples/demo.db PRAGMA user_version = %d, want %d (this binary's migration level). Open() on a copy hides the lag; rebaseline the committed file", have, want)
	}

	// GDK-114 class: derived tables/columns filled by migrate-time backfill
	// or by the snapshot copy set. Zero rows means the fixture lost them.
	for _, table := range []string{"item_refs"} {
		var n int
		if err := raw.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Errorf("%s: %v", table, err)
			continue
		}
		if n == 0 {
			t.Errorf("%s has 0 rows on examples/demo.db — derived backfill/copy was lost (GDK-114)", table)
		}
	}
	var excerpts int
	if err := raw.QueryRow(`SELECT count(*) FROM pages WHERE excerpt IS NOT NULL AND excerpt != ''`).Scan(&excerpts); err != nil {
		t.Errorf("pages.excerpt: %v", err)
	} else if excerpts == 0 {
		t.Errorf("pages.excerpt is empty on every page — v15 backfill was lost")
	}
}
