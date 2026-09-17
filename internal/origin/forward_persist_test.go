package origin

// GDK-1967 wiring tests: the dev-lockout open policy covers the built-in
// origin's persist too. The store package owns the policy's semantics on
// the mirror (forward_migration_test.go); these pin that the same process
// default — the one main installs for gadak.db — refuses to migrate
// issuetap.db forward on a dev build, leaves the file exactly as the
// release wrote it, and still migrates under GADAK_DEV_MIGRATE=1 or on a
// release build. The persist is the original record ("영속은 origin의
// 몫", CLAUDE.md); the incident was a dev build that refused gadak.db and
// in the same boot moved issuetap.db 1→3, crash-looping the installed
// release.

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
	issuetap "github.com/midagedev/issuetap"
)

// persistAt produces a real issuetap persist at user_version=level: a fresh
// one is opened through issuetap (the only writer of the file's shape), then
// the post-v1 objects the head file carries are dropped and the stamp is
// rewound with database/sql. A bare stamp rewind is not enough — the v1→v2
// migration creates attachment_blobs and v2→v3 creates boards/sprints, so a
// head-shaped file stamped 1 dies on "table attachment_blobs already
// exists". This is issuetap's own migrate_test.go v1 fixture, built from
// gadak's side of the pin. If a future pin grows a v4, the drop list lags
// and the migration fails loudly here — the fixture cannot silently stop
// being v1-shaped.
func persistAt(t *testing.T, path string, level int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	emb, err := issuetap.NewEmbedded(issuetap.EmbeddedConfig{PersistPath: path, WallClock: true})
	if err != nil {
		t.Fatalf("seed persist: %v", err)
	}
	if err := emb.Close(); err != nil {
		t.Fatalf("close seeded persist: %v", err)
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS attachment_blobs", // created by v1→v2
		"DROP TABLE IF EXISTS boards",           // created by v2→v3 (agile)
		"DROP TABLE IF EXISTS sprints",
		"PRAGMA user_version = " + strconv.Itoa(level),
	} {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	raw.Close()
}

func readPersistVersion(t *testing.T, path string) int {
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

// unsetDevMigrate clears the operator override for the refusal case and
// puts back whatever the environment had.
func unsetDevMigrate(t *testing.T) {
	t.Helper()
	old, had := os.LookupEnv("GADAK_DEV_MIGRATE")
	if had {
		t.Cleanup(func() { os.Setenv("GADAK_DEV_MIGRATE", old) })
	}
	os.Unsetenv("GADAK_DEV_MIGRATE")
}

// TestClientRefusesForwardPersistOnDevBuild: a dev build's process default
// (RefuseForward) must refuse the persist the same way it refuses the
// mirror — typed refusal, stamp unchanged, no pre-v2 backup — because the
// built-in origin opens the persist before the store's refusal ever fires.
func TestClientRefusesForwardPersistOnDevBuild(t *testing.T) {
	home := t.TempDir()
	cfg := builtInCfg(t, home)
	path := PersistPath(home)
	persistAt(t, path, 1)

	prev := store.DefaultOpenOptions()
	store.SetDefaultOpenOptions(store.OpenOptions{ForwardMigration: store.RefuseForward, BuildVersion: "0.0.0-dev"})
	t.Cleanup(func() { store.SetDefaultOpenOptions(prev) })
	unsetDevMigrate(t)

	_, err := Client(cfg)
	var refused *PersistForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error is %T (%v), want *origin.PersistForwardRefusedError", err, err)
	}
	if refused.Path != path {
		t.Errorf("Path = %q, want %q", refused.Path, path)
	}
	if refused.Have != 1 || refused.Want != issuetap.PersistSchemaVersion() {
		t.Errorf("Have/Want = %d/%d, want 1/%d", refused.Have, refused.Want, issuetap.PersistSchemaVersion())
	}
	if refused.BuildVersion != "0.0.0-dev" {
		t.Errorf("BuildVersion = %q, want 0.0.0-dev — the error must say which build refused", refused.BuildVersion)
	}
	// The message names the same facts the mirror refusal does: the dev
	// build, the lockout it prevents, and both ways out.
	msg := refused.Error()
	for _, want := range []string{
		"dev build",
		path,
		"GADAK_DEV_MIGRATE=1",
		"cp -R " + home + " " + home + "-dev",
		"GADAK_HOME=" + home + "-dev",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
	if got := readPersistVersion(t, path); got != 1 {
		t.Fatalf("persist user_version = %d after a refused open, want 1 — the release's file must be left alone", got)
	}
	if _, err := os.Stat(path + ".pre-v2.bak"); !os.IsNotExist(err) {
		t.Fatalf("a refused open left a pre-v2 backup (err=%v) — no write may happen before the refusal", err)
	}
}

// TestClientMigratesPersistUnderDevOverride: GADAK_DEV_MIGRATE=1 is the one
// operator override, and it covers the persist — the same escape hatch the
// mirror refusal names.
func TestClientMigratesPersistUnderDevOverride(t *testing.T) {
	home := t.TempDir()
	cfg := builtInCfg(t, home)
	path := PersistPath(home)
	persistAt(t, path, 1)

	prev := store.DefaultOpenOptions()
	store.SetDefaultOpenOptions(store.OpenOptions{ForwardMigration: store.RefuseForward, BuildVersion: "0.0.0-dev"})
	t.Cleanup(func() { store.SetDefaultOpenOptions(prev) })
	t.Setenv("GADAK_DEV_MIGRATE", "1")

	if _, err := Client(cfg); err != nil {
		t.Fatalf("open under GADAK_DEV_MIGRATE=1: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if got := readPersistVersion(t, path); got != issuetap.PersistSchemaVersion() {
		t.Fatalf("persist user_version = %d under GADAK_DEV_MIGRATE=1, want %d", got, issuetap.PersistSchemaVersion())
	}
}

// TestClientMigratesPersistOnReleaseBuild: a release build's process
// default (MigrateForward) migrates the persist on open, as it always did —
// an installed upgrade must keep working with no prompt.
func TestClientMigratesPersistOnReleaseBuild(t *testing.T) {
	home := t.TempDir()
	cfg := builtInCfg(t, home)
	path := PersistPath(home)
	persistAt(t, path, 1)

	prev := store.DefaultOpenOptions()
	store.SetDefaultOpenOptions(store.OpenOptions{ForwardMigration: store.MigrateForward, BuildVersion: "0.21.0"})
	t.Cleanup(func() { store.SetDefaultOpenOptions(prev) })
	unsetDevMigrate(t)

	if _, err := Client(cfg); err != nil {
		t.Fatalf("release open: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if got := readPersistVersion(t, path); got != issuetap.PersistSchemaVersion() {
		t.Fatalf("persist user_version = %d after a release open, want %d", got, issuetap.PersistSchemaVersion())
	}
}
