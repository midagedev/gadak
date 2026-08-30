package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The contract the ui-focus poll leans on (GDK-1170): the version moves when
// the mirror is written and holds still when it is only read.
//
// The trap this pins is WAL. The mirror is opened journal_mode=WAL, so while a
// process holds it open a write lands in gadak.db-wal and never touches
// gadak.db. A version keyed on the database file alone passes nothing here.
func TestMirrorVersionMovesOnWriteAndHoldsOnRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mirror.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatal(err)
	}

	before := db.MirrorVersion()
	if before == "" {
		t.Fatal("an open mirror must have a non-empty version")
	}
	// Same bytes, same answer — the poll only ever compares.
	if again := db.MirrorVersion(); again != before {
		t.Fatalf("version not stable across two reads: %q then %q", before, again)
	}

	// A read must not look like a change, or every poll pulls a delta.
	if _, err := db.SyncState(ctx, "jira"); err != nil {
		t.Fatal(err)
	}
	if got := db.MirrorVersion(); got != before {
		t.Fatalf("a read moved the version: %q -> %q", before, got)
	}

	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", ExternalID: "1", Key: "AAA-1",
				Title: "first", CreatedAt: "2026-08-01T00:00:00.000Z",
				UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: Issue{ProjectKey: "AAA", IssueType: "Task", IssueTypeID: "1", Status: "To Do", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	after := db.MirrorVersion()
	if after == before {
		t.Fatalf("a write did not move the version (WAL trap): %q", after)
	}
	if again := db.MirrorVersion(); again != after {
		t.Fatalf("version not stable after the write: %q then %q", after, again)
	}
}

// The poll must survive states that are not errors: no path, and a mirror that
// does not exist yet. Both have to compare equal to themselves.
func TestMirrorVersionDegradesQuietly(t *testing.T) {
	if got := MirrorVersion(""); got != "" {
		t.Fatalf("empty path must be the empty version, got %q", got)
	}
	missing := filepath.Join(t.TempDir(), "nope.db")
	got := MirrorVersion(missing)
	if got == "" {
		t.Fatal("a missing mirror must still yield a comparable version")
	}
	if again := MirrorVersion(missing); again != got {
		t.Fatalf("missing mirror not stable: %q then %q", got, again)
	}
	if MirrorChangedAt(missing).IsZero() != true {
		t.Fatal("a missing mirror has no change time")
	}
	if !MirrorChangedAt("").IsZero() {
		t.Fatal("no path has no change time")
	}
}

// MirrorChangedAt is the diagnostic half: it names the newest of the files the
// version keys on, so `gadak doctor` can say when the mirror last moved.
func TestMirrorChangedAtTracksTheNewestSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mirror.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before := MirrorChangedAt(path)
	if before.IsZero() {
		t.Fatal("an open mirror has a change time")
	}
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatal(err)
	}
	after := MirrorChangedAt(path)
	if !after.After(before) {
		t.Fatalf("a write did not move the change time: %v -> %v", before, after)
	}
	// It reads the sidecar, not just the database file: under WAL that is the
	// only one a write touches while the mirror stays open.
	wal, err := os.Stat(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Equal(wal.ModTime()) {
		t.Fatalf("change time %v is not the -wal mtime %v", after, wal.ModTime())
	}
}
