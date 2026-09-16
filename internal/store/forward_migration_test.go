package store

// GDK-1687: a dev build must not migrate a release-written mirror forward.
// Two binaries share one mirror file — an installed release and a 0.0.0-dev
// build from a checkout — and one Open from the dev side used to move the
// file to the dev schema, locking the release out with
// SchemaTooNewError. These tests pin the RefuseForward policy at the open
// boundary: refusal leaves the file byte-identical in user_version terms,
// the override env is the only way through, and a brand-new file (no
// release ever wrote it) is still created at head.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// mirrorAt writes a mirror whose schema is exactly what a build whose head
// was `level` would have produced: the first `level` migrations applied to a
// fresh file, user_version stamped. Same idiom as
// TestMigrateV40RecomputesClonedFrom (derive_test.go).
func mirrorAt(t *testing.T, path string, level int) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < level; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec("PRAGMA user_version = " + strconv.Itoa(level)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()
}

// userVersion reads PRAGMA user_version straight from the file, the way an
// installed release (or sqlite3) would see it after this process exits.
func userVersion(t *testing.T, path string) int {
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

func refuserOptions(path string) OpenOptions {
	// A named-profile layout so the copy hint exercises the --workspace form.
	return OpenOptions{ForwardMigration: RefuseForward, BuildVersion: DevVersionForTest}
}

const DevVersionForTest = "0.0.0-dev"

func TestOpenWithRefuseForwardLeavesReleaseMirrorAtItsVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles", "x", "gadak.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	mirrorAt(t, path, 44)

	_, err := OpenWith(path, refuserOptions(path))
	var refused *SchemaForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error is %T (%v), want *SchemaForwardRefusedError", err, err)
	}
	if refused.Path != path {
		t.Errorf("Path = %q, want %q", refused.Path, path)
	}
	if refused.Have != 44 {
		t.Errorf("Have = %d, want 44", refused.Have)
	}
	if refused.Head != len(migrations) {
		t.Errorf("Head = %d, want %d", refused.Head, len(migrations))
	}
	if refused.BuildVersion != DevVersionForTest {
		t.Errorf("BuildVersion = %q, want %q", refused.BuildVersion, DevVersionForTest)
	}
	if got := userVersion(t, path); got != 44 {
		t.Fatalf("user_version = %d after a refused open, want 44 — the refusal must leave the release's file alone", got)
	}
	// The message names all three facts the incident made necessary: what
	// happened, why it refused, and both ways out.
	msg := refused.Error()
	for _, want := range []string{
		"dev build",
		path,
		"schema 44",
		"lock the installed release out of this workspace",
		"GADAK_DEV_MIGRATE=1",
		"cp -R " + filepath.Dir(path) + " " + filepath.Dir(path) + "-dev",
		"--workspace x-dev",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

// localAt writes a local.db whose schema is exactly what a build whose local
// head was `level` would have produced — mirrorAt's sibling, for the other
// versioned file in a workspace.
func localAt(t *testing.T, mirrorPath string, level int) {
	t.Helper()
	path := LocalPath(mirrorPath)
	raw, err := sql.Open("sqlite", localDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < level; i++ {
		if _, err := raw.Exec(localMigrations[i]); err != nil {
			raw.Close()
			t.Fatalf("local migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec("PRAGMA user_version = " + strconv.Itoa(level)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
}

// walize switches a mirror to WAL and closes it again — the journal mode
// every gadak build opens it with, so a byte comparison across an open is
// about what that open wrote, not about the conversion. `main.` is load
// bearing: an unqualified journal_mode applies to every ATTACHed database,
// and attachLocalHook has local.db on this connection — which gadak keeps on
// a rollback journal.
func walize(t *testing.T, path string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := raw.QueryRow("PRAGMA main.journal_mode = WAL").Scan(&mode); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
}

// localUserVersion reads local.db's stamp the way an installed release would.
func localUserVersion(t *testing.T, mirrorPath string) int {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+LocalPath(mirrorPath)+"?mode=ro")
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

// TestOpenWithRefuseForwardLeavesReleaseLocalDBAlone is GDK-1806: a workspace
// has two versioned files, and "a dev build does not decide for the release"
// was enforced on one of them. EnsureLocal ran before enforceForwardPolicy and
// took no OpenOptions, so the command whose whole purpose is to leave the
// release's workspace alone migrated local.db anyway — measured on a real
// workspace (GDK-1801): mirror stayed 49 with the correct refusal while
// local.db went 7 → 11 and changed bytes, after which the installed release
// warned on every command and, unable to maintain localSchemaV10, appended to
// visits while sessions stopped moving.
//
// local.db is the file the origin cannot rebuild, so the assertion is bytes,
// not just the stamp. Both files are compared as their main files only: a
// refused open still has to read the mirror's user_version, which creates WAL
// sidecars beside it; the sidecars are SQLite's scratch, the main files are
// the release's data.
func TestOpenWithRefuseForwardLeavesReleaseLocalDBAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles", "x", "gadak.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// The release's shape: a mirror genuinely behind what this build would
	// apply (44, with local.db one level behind so the v53 hold puts the
	// target at apiUsageCopyVersion-1, not head), and a local.db the same
	// open must not migrate. "One level behind" on the mirror no longer
	// refuses since GDK-1933 — a file resting at this build's held target is
	// not moved by opening it — so a refusal test needs the wider gap.
	// local.db first: attachLocalHook creates one at head on any open of a
	// gadak.db that has none, mirrorAt's included.
	localAt(t, path, len(localMigrations)-1)
	mirrorAt(t, path, 44)
	// A release leaves its mirror in WAL (mirrorDSN). Put the file in that
	// shape before the byte snapshot, or the comparison measures SQLite's
	// journal-mode conversion on first open instead of this policy.
	walize(t, path)

	mirrorBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	localBefore, err := os.ReadFile(LocalPath(path))
	if err != nil {
		t.Fatal(err)
	}

	_, err = OpenWith(path, refuserOptions(path))
	var refused *SchemaForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error is %T (%v), want *SchemaForwardRefusedError", err, err)
	}
	// The refusal with a hold in effect (GDK-1933): Head is the held target
	// this build would actually write, and the message says why that is not
	// the build's head — the mirror is waiting on the local database.
	if refused.Head != apiUsageCopyVersion-1 {
		t.Errorf("Head = %d, want %d (the held target, not len(migrations))", refused.Head, apiUsageCopyVersion-1)
	}
	if refused.FullHead != len(migrations) {
		t.Errorf("FullHead = %d, want %d", refused.FullHead, len(migrations))
	}
	if msg := refused.Error(); !strings.Contains(msg, "waiting on the local database") {
		t.Errorf("refusal under a hold must say the mirror is waiting on the local database:\n%s", msg)
	}

	if got, want := userVersion(t, path), 44; got != want {
		t.Errorf("mirror user_version = %d after a refused open, want %d", got, want)
	}
	if got, want := localUserVersion(t, path), len(localMigrations)-1; got != want {
		t.Errorf("local.db user_version = %d after a refused open, want %d — the refusal must leave the release's personal history where it found it", got, want)
	}
	mirrorAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mirrorBefore, mirrorAfter) {
		t.Errorf("gadak.db changed bytes across a refused open (%d → %d)", len(mirrorBefore), len(mirrorAfter))
	}
	localAfter, err := os.ReadFile(LocalPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(localBefore, localAfter) {
		for i := range localBefore {
			if i < len(localAfter) && localBefore[i] != localAfter[i] {
				t.Logf("first differing byte at offset %d: %d → %d", i, localBefore[i], localAfter[i])
				break
			}
		}
		t.Errorf("local.db changed bytes across a refused open (%d → %d) — the one file the origin cannot rebuild", len(localBefore), len(localAfter))
	}
}

// TestEnsureLocalWithRefuseForwardStillCreatesANewFile keeps the mirror's own
// exception on the local side: a file no release ever wrote is created at
// head, so a dev build in a fresh workspace is not left without the tables it
// queries. GADAK_DEV_MIGRATE stays the operator override for an existing file.
func TestEnsureLocalWithRefuseForwardStillCreatesANewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	if err := EnsureLocalWith(path, OpenOptions{ForwardMigration: RefuseForward, BuildVersion: DevVersionForTest}); err != nil {
		t.Fatalf("EnsureLocalWith on a workspace with no local.db: %v", err)
	}
	if got, want := localUserVersion(t, path), len(localMigrations); got != want {
		t.Fatalf("local.db user_version = %d, want %d — a brand-new file never belonged to a release", got, want)
	}

	// An existing file one level behind is left alone...
	older := filepath.Join(t.TempDir(), "gadak.db")
	localAt(t, older, len(localMigrations)-1)
	if err := EnsureLocalWith(older, OpenOptions{ForwardMigration: RefuseForward, BuildVersion: DevVersionForTest}); err != nil {
		t.Fatalf("EnsureLocalWith on a behind local.db: %v", err)
	}
	if got, want := localUserVersion(t, older), len(localMigrations)-1; got != want {
		t.Fatalf("local.db user_version = %d, want %d", got, want)
	}
	// ...until the operator says otherwise, the same override the mirror takes.
	t.Setenv("GADAK_DEV_MIGRATE", "1")
	if err := EnsureLocalWith(older, OpenOptions{ForwardMigration: RefuseForward, BuildVersion: DevVersionForTest}); err != nil {
		t.Fatalf("EnsureLocalWith under GADAK_DEV_MIGRATE: %v", err)
	}
	if got, want := localUserVersion(t, older), len(localMigrations); got != want {
		t.Fatalf("local.db user_version = %d under GADAK_DEV_MIGRATE, want %d", got, want)
	}
}

func TestOpenWithMigrateForwardStillMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	mirrorAt(t, path, 44)

	db, err := OpenWith(path, OpenOptions{ForwardMigration: MigrateForward, BuildVersion: "0.21.0"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version %d, want %d", got, len(migrations))
	}
	if got := userVersion(t, path); got != len(migrations) {
		t.Fatalf("user_version = %d, want %d", got, len(migrations))
	}
}

// Open keeps today's signature and behaviour: tests, tools, and every caller
// outside the policy still get a forward-migrating open.
func TestOpenIsMigrateForward(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	mirrorAt(t, path, 44)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if got := userVersion(t, path); got != len(migrations) {
		t.Fatalf("user_version = %d, want %d", got, len(migrations))
	}
}

// TestForwardMigrationFromSeededV44PreservesRows (GDK-1786): every other
// v44→head test in this file opens an EMPTY mirror — five mirrorAt(_, 44)
// call sites, zero rows between them — so a migration that dropped or
// drained rows would have passed them all. This seeds one synthetic row
// into every v44 table and requires each table's row count to survive to
// head, plus a SchemaAudit for structural parity with a fresh build.
//
// The seeding fills every PK column and every NOT NULL column that has no
// default, reading the column list from PRAGMA table_info rather than
// naming tables by hand — the same lesson as the GDK-1785 doc gate: a gate
// that lists is a property of its list. Foreign keys are unchecked for the
// seeding connection on purpose: the claim under test is row preservation
// through migration, not referential integrity of synthetic rows (Open's
// own connections keep foreign_keys=ON; nothing revalidates old rows).
//
// items_fts is excluded: it is contentless FTS the sync rebuilds, and the
// v50 tokenizer change is repaired wholesale by repairItemsFTS at Open —
// its rows are derived data, never migrated.
func TestForwardMigrationFromSeededV44PreservesRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	mirrorAt(t, path, 44)

	seeded := map[string]int{}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var tables []string
	{
		rows, err := raw.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if strings.HasPrefix(n, "sqlite_") || strings.HasPrefix(n, "items_fts") {
				continue
			}
			tables = append(tables, n)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
	}
	for _, tbl := range tables {
		info, err := raw.Query("PRAGMA table_info(" + quoteIdent(tbl) + ")")
		if err != nil {
			t.Fatalf("%s: %v", tbl, err)
		}
		var cols, vals []string
		for info.Next() {
			var cid int
			var name, typ string
			var notnull int
			var dflt *string
			var pk int
			if err := info.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				info.Close()
				t.Fatal(err)
			}
			if pk == 0 && (notnull == 0 || dflt != nil) {
				continue
			}
			cols = append(cols, name)
			if strings.Contains(strings.ToUpper(typ), "INT") || strings.Contains(strings.ToUpper(typ), "REAL") {
				vals = append(vals, "1")
			} else {
				vals = append(vals, "'seed-"+tbl+"'")
			}
		}
		if err := info.Err(); err != nil {
			info.Close()
			t.Fatal(err)
		}
		info.Close()

		var stmt string
		if len(cols) == 0 {
			stmt = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", quoteIdent(tbl))
		} else {
			stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
				quoteIdent(tbl), strings.Join(cols, ", "), strings.Join(vals, ", "))
		}
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("seed %s: %v\n  %s", tbl, err, stmt)
		}
		seeded[tbl] = 1
	}
	if len(seeded) < 20 {
		t.Fatalf("seeded only %d tables — the sqlite_master walk broke", len(seeded))
	}
	t.Logf("seeded 1 row into each of %d v44 tables; migrating to head", len(seeded))

	db, err := Open(path)
	if err != nil {
		t.Fatalf("migrating a seeded v44 mirror: %v", err)
	}
	defer db.Close()
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version %d, want %d", got, len(migrations))
	}
	for tbl, want := range seeded {
		var got int
		if err := db.sql.QueryRowContext(context.Background(),
			fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(tbl))).Scan(&got); err != nil {
			t.Fatalf("%s: %v", tbl, err)
		}
		if got < want {
			t.Errorf("%s held %d seeded row(s) at v44, has %d at head — a v44→head migration lost rows", tbl, want, got)
		}
	}
	audit, err := db.SchemaAudit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !audit.OK() {
		t.Errorf("migrated mirror is missing what a fresh build has: %v", audit.Missing)
	}
}

// A brand-new file is not "a release's mirror": a dev build may create one
// at head (`gadak --workspace demo init` on a fresh machine).
func TestOpenWithRefuseForwardCreatesBrandNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := OpenWith(path, refuserOptions(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version %d, want %d", got, len(migrations))
	}
	if _, err := db.TableCount(context.Background(), "issues"); err != nil {
		t.Errorf("issues table missing: %v", err)
	}
}

// user_version 0 with tables present is ambiguous provenance, not a fresh
// file — the refusal applies.
func TestOpenWithRefuseForwardRefusesVersionZeroWithTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("CREATE TABLE leftover (id TEXT)"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	_, err = OpenWith(path, refuserOptions(path))
	var refused *SchemaForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error is %T (%v), want *SchemaForwardRefusedError", err, err)
	}
	if refused.Have != 0 {
		t.Errorf("Have = %d, want 0", refused.Have)
	}
}

func TestGADAKDevMigrateOverridesRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	mirrorAt(t, path, 44)
	t.Setenv("GADAK_DEV_MIGRATE", "1")

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	db, err := OpenWith(path, refuserOptions(path))
	log.SetOutput(orig)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if got := userVersion(t, path); got != len(migrations) {
		t.Fatalf("user_version = %d, want %d", got, len(migrations))
	}
	want := "store: migrating " + path + " 44→" + strconv.Itoa(len(migrations)) + " under GADAK_DEV_MIGRATE"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("stderr log missing %q; got:\n%s", want, buf.String())
	}
}

// TestOpenWithRefuseForwardReopensMirrorItHeldBelowHead is GDK-1933: the
// first open of a brand-new mirror under the dev policy holds the file one
// version below apiUsageCopyVersion when local.db is not ready for the copy
// (the incident's local side: a local.db the same dev policy keeps at an
// older level), and the second open of that same file was then refused by
// the build that wrote it — enforceForwardPolicy compared against
// len(migrations) instead of against what this build would actually apply.
// The policy must refuse only when opening the file would move it.
func TestOpenWithRefuseForwardReopensMirrorItHeldBelowHead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	// Schema 7 carries saved_views (the v26 probe answers ready) but not
	// api_usage (the v53 probe does not) — the incident's exact local shape.
	localAt(t, path, 7)

	// First open: the dev build writes the mirror at the held level, and the
	// hold says so on stderr.
	var first bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&first)
	db, err := OpenWith(path, refuserOptions(path))
	log.SetOutput(orig)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if got, want := db.SchemaVersion(), apiUsageCopyVersion-1; got != want {
		t.Fatalf("first open schema version = %d, want %d (held below the copy)", got, want)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("copy migration %d is waiting on the local database", apiUsageCopyVersion); !strings.Contains(first.String(), want) {
		t.Errorf("first open stderr missing the hold line %q; got:\n%s", want, first.String())
	}

	// Second open of the file this build just wrote — the defect's own shape.
	var second bytes.Buffer
	log.SetOutput(&second)
	db2, err := OpenWith(path, refuserOptions(path))
	log.SetOutput(orig)
	if err != nil {
		t.Fatalf("re-opening the mirror this dev build wrote: %v", err)
	}
	if got, want := db2.SchemaVersion(), apiUsageCopyVersion-1; got != want {
		t.Fatalf("second open schema version = %d, want %d", got, want)
	}
	// Seed the frozen mirror-side counters so the later copy has something to
	// carry: the hold must be a hold, not a silent skip of v53.
	if _, err := db2.sql.Exec(`INSERT INTO api_usage (day, requests, throttled, server_errors, retries, wait_ms)
		VALUES ('2026-09-15', 60, 2, 0, 1, 300)`); err != nil {
		t.Fatal(err)
	}
	if err := db2.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second.String(), "waiting on the local database") {
		t.Errorf("second open re-logged the hold — an already-held file must stay quiet:\n%s", second.String())
	}

	// Once local answers, the copy still runs and takes the file to head.
	if err := EnsureLocalWith(path, OpenOptions{}); err != nil {
		t.Fatal(err)
	}
	db3, err := OpenWith(path, OpenOptions{ForwardMigration: MigrateForward})
	if err != nil {
		t.Fatalf("open after local recovered: %v", err)
	}
	defer db3.Close()
	if got := db3.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version after local recovered = %d, want %d (the held copy must run)", got, len(migrations))
	}
	days, err := db3.APIUsage(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Day != "2026-09-15" || days[0].Requests != 60 {
		t.Errorf("api_usage after local recovered = %+v, want the seeded 2026-09-15/60 — v53 must not be silently skipped", days)
	}
}

// The too-new refusal is a different fact (the file is ahead of this build)
// and keeps its own error under both policies.
func TestOpenWithTooNewStillRefusedUnderRefuseForward(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	_, err = OpenWith(path, refuserOptions(path))
	var tooNew *SchemaTooNewError
	if !errors.As(err, &tooNew) {
		t.Fatalf("error is %T (%v), want *SchemaTooNewError", err, err)
	}
}

// The process default is what closes the class: the workspace registry, the
// MCP server and originbind call plain Open with no version in sight, so a
// dev main installs the refusal once and every one of them inherits it.
func TestSetDefaultOpenOptionsMakesPlainOpenRefuse(t *testing.T) {
	prev := defaultOpenOptions()
	t.Cleanup(func() { SetDefaultOpenOptions(prev) })

	path := filepath.Join(t.TempDir(), "profiles", "x", "gadak.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	mirrorAt(t, path, 44)

	SetDefaultOpenOptions(refuserOptions(path))
	_, err := Open(path)
	var refused *SchemaForwardRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("plain Open under a RefuseForward default: error is %T (%v), want *SchemaForwardRefusedError", err, err)
	}
	if got := userVersion(t, path); got != 44 {
		t.Fatalf("user_version = %d, want 44", got)
	}

	// An explicit MigrateForward still wins over the default — that is how
	// commands working on a temp copy opt out.
	db, err := OpenWith(path, OpenOptions{ForwardMigration: MigrateForward})
	if err != nil {
		t.Fatalf("explicit MigrateForward: %v", err)
	}
	db.Close()
	if got := userVersion(t, path); got != len(migrations) {
		t.Fatalf("user_version = %d after explicit MigrateForward, want %d", got, len(migrations))
	}
}
