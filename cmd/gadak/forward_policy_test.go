package main

// GDK-1687 wiring tests: the CLI's open policy — a dev build refuses to
// migrate a release-written mirror forward. The store package tests the
// policy's semantics on real old-schema mirrors (forward_migration_test.go);
// these pin the wiring in this package (openStore, doctor) using a head
// mirror whose user_version is rewound, which is sufficient because the
// refusal fires before any table is read.

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// rewoundMirror seeds a fresh head-level mirror, then rewinds its
// user_version to level — the shape an installed release leaves behind once
// this checkout grows past it.
func rewoundMirror(t *testing.T, level int) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
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
	if _, err := raw.Exec("PRAGMA user_version = " + strconv.Itoa(level)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()
	return path
}

func mirrorUserVersion(t *testing.T, path string) int {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var v int
	if err := raw.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// The test binary carries the dev version (no release ldflags), which is the
// incident's own shape; a release string flips the policy.
func TestStoreOpenOptionsFollowsBuildKind(t *testing.T) {
	if got := storeOpenOptions(); got.ForwardMigration != store.RefuseForward {
		t.Fatalf("policy = %v, want RefuseForward for version %q", got.ForwardMigration, version)
	}
	if got := storeOpenOptions(); got.BuildVersion != version {
		t.Errorf("BuildVersion = %q, want %q", got.BuildVersion, version)
	}
	old := version
	version = "0.21.0"
	defer func() { version = old }()
	if got := storeOpenOptions(); got.ForwardMigration != store.MigrateForward {
		t.Fatalf("policy = %v, want MigrateForward for a release version", got.ForwardMigration)
	}
}

func TestOpenStoreRefusesForwardOnDevBuild(t *testing.T) {
	path := rewoundMirror(t, 44)
	_, err := openStore()
	var refused *store.SchemaForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error is %T (%v), want *store.SchemaForwardRefusedError", err, err)
	}
	if got := mirrorUserVersion(t, path); got != 44 {
		t.Fatalf("user_version = %d after a refused openStore, want 44 — the release's file must be left alone", got)
	}
}

// doctor is what someone runs when the mirror stopped opening: the refusal
// must be named with its way out, not reported as a generic open failure.
func TestDoctorNamesForwardRefusal(t *testing.T) {
	rewoundMirror(t, 44)
	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor must still report on a mirror it refuses to open: %v\n%s", err, out)
	}
	if !strings.Contains(out, "schema_forward_refused") {
		t.Fatalf("doctor must name the refusal:\n%s", out)
	}
	if !strings.Contains(out, "GADAK_DEV_MIGRATE") {
		t.Fatalf("doctor's diagnosis must name the override:\n%s", out)
	}
	if strings.Contains(out, "open failed") {
		t.Fatalf("doctor must not report a nameable cause as a generic open failure:\n%s", out)
	}
}
