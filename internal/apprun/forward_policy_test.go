package apprun

// GDK-1687: the desktop main has no openStore of its own — openDB's fallback
// is its path, so the fallback must apply the same policy serve gets: a dev
// version refuses to migrate a release-written mirror forward.

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// rewoundRootMirror seeds the root profile's mirror at head, then rewinds
// user_version to 44. The tables are head-shaped, which the refusal never
// reads — it fires first; that ordering is the point under test.
func rewoundRootMirror(t *testing.T) string {
	t.Helper()
	testHome(t)
	home, ok := os.LookupEnv("GADAK_HOME")
	if !ok {
		t.Fatal("testHome left no GADAK_HOME")
	}
	path := filepath.Join(home, "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("seed mirror: %v", err)
	}
	db.Close()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 44"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()
	return path
}

func TestOpenDevVersionRefusesForwardMigration(t *testing.T) {
	path := rewoundRootMirror(t)
	_, err := Open(Options{Version: "0.0.0-dev"})
	var refused *store.SchemaForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error is %T (%v), want *store.SchemaForwardRefusedError", err, err)
	}
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := raw.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if v != 44 {
		t.Fatalf("user_version = %d after a refused boot, want 44", v)
	}
}

// A release version must not refuse. The rewound fixture's head-shaped
// tables make the actual 44→head migration fail for unrelated reasons, so
// the assertion is exactly the policy fact: whatever error comes back, it is
// not the refusal.
func TestOpenDBReleaseVersionDoesNotRefuse(t *testing.T) {
	rewoundRootMirror(t)
	_, err := openDB(Options{Version: "0.21.0"})
	var refused *store.SchemaForwardRefusedError
	if errors.As(err, &refused) {
		t.Fatalf("release version refused a forward migration: %v", err)
	}
	if err == nil {
		t.Fatal("expected the rewound fixture's migration to fail for table-shape reasons, got nil")
	}
}

// The zero Version (a caller that stamped nothing) is untrusted provenance —
// skillinstall.IsDevBuild("") is true — so the safe side is the refusal.
func TestOpenDBUnversionedIsDevShaped(t *testing.T) {
	rewoundRootMirror(t)
	_, err := openDB(Options{})
	var refused *store.SchemaForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error is %T (%v), want *store.SchemaForwardRefusedError", err, err)
	}
}
