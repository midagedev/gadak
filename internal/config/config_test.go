package config

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestDirForDefaultAndNamed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })

	root, err := DirFor("")
	if err != nil {
		t.Fatal(err)
	}
	if root != home {
		t.Fatalf("DirFor(\"\") = %q, want %q", root, home)
	}
	root2, err := DirFor("default")
	if err != nil {
		t.Fatal(err)
	}
	if root2 != home {
		t.Fatalf("DirFor(\"default\") = %q, want %q", root2, home)
	}

	named, err := DirFor("work")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "profiles", "work")
	if named != want {
		t.Fatalf("DirFor(\"work\") = %q, want %q", named, want)
	}

	db, err := DBPathFor("work")
	if err != nil {
		t.Fatal(err)
	}
	if db != filepath.Join(want, "gadak.db") {
		t.Fatalf("DBPathFor: %q", db)
	}
	att, err := AttachmentDirFor("work")
	if err != nil {
		t.Fatal(err)
	}
	if att != filepath.Join(want, "attachments") {
		t.Fatalf("AttachmentDirFor: %q", att)
	}

	// Dir() still tracks the global profile.
	SetProfile("work")
	d, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if d != want {
		t.Fatalf("Dir() with profile work = %q, want %q", d, want)
	}
	SetProfile("")
	d, err = Dir()
	if err != nil {
		t.Fatal(err)
	}
	if d != home {
		t.Fatalf("Dir() default = %q, want %q", d, home)
	}
}

func TestDirForRejectsPathEscape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)

	for _, name := range []string{"../..", "../../.ssh", ".."} {
		got, err := DirFor(name)
		if err == nil {
			t.Errorf("DirFor(%q) = %q, want error (path escapes %q)", name, got, home)
			continue
		}
		if !strings.Contains(err.Error(), "invalid workspace name") {
			t.Errorf("DirFor(%q) error %v, want invalid workspace name", name, err)
		}
		if got != "" {
			t.Errorf("DirFor(%q) returned path %q with error, want empty", name, got)
		}
	}
	if _, err := LoadFor("../../.ssh"); err == nil {
		t.Fatal("LoadFor(\"../../.ssh\") succeeded, want error")
	}
	escaped := filepath.Join(filepath.Dir(home), ".ssh")
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		t.Fatalf("escaped path %q appeared: %v", escaped, err)
	}

	for _, name := range []string{"work", "demo-1"} {
		got, err := DirFor(name)
		if err != nil {
			t.Errorf("DirFor(%q) unexpected error: %v", name, err)
			continue
		}
		want := filepath.Join(home, "profiles", name)
		if got != want {
			t.Errorf("DirFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestEnvPrefersNewPrefixThenLegacy(t *testing.T) {
	os.Unsetenv("GADAK_TOKEN")
	t.Setenv("SCRY_TOKEN", "legacy")
	if got := Env("TOKEN"); got != "legacy" {
		t.Fatalf("Env(TOKEN) without GADAK_ = %q, want legacy", got)
	}
	t.Setenv("GADAK_TOKEN", "new")
	if got := Env("TOKEN"); got != "new" {
		t.Fatalf("Env(TOKEN) with both = %q, want new", got)
	}
	// D2: an empty GADAK_* export is unset, not a value that hides SCRY_*.
	t.Setenv("GADAK_TOKEN", "")
	t.Setenv("SCRY_TOKEN", "legacy")
	if got := Env("TOKEN"); got != "legacy" {
		t.Fatalf("Env(TOKEN) with empty GADAK_ = %q, want legacy", got)
	}
}

func TestEnvEmptyNewPrefixFallsBackAcrossSuffixes(t *testing.T) {
	suffixes := []string{"TOKEN", "HOME", "PROFILE", "SITE", "EMAIL", "PROJECTS"}
	for _, suffix := range suffixes {
		t.Setenv(EnvPrefix+suffix, "")
		t.Setenv(LegacyEnvPrefix+suffix, "legacy-"+suffix)
		if got := Env(suffix); got != "legacy-"+suffix {
			t.Errorf("Env(%s) with empty %s = %q, want %q", suffix, EnvPrefix+suffix, got, "legacy-"+suffix)
		}
	}
}

func TestHomeRootWarnsWhenBothDirsExist(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("GADAK_HOME", "")
	t.Setenv("SCRY_HOME", "")
	dualHomeWarnOnce = sync.Once{}

	legacy := filepath.Join(root, LegacyDirName)
	next := filepath.Join(root, DirName)
	if err := os.Mkdir(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.json"), []byte(`{"site":"https://legacy.example.invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(next, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(next, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		got, err := homeRoot()
		if err != nil {
			t.Fatal(err)
		}
		if got != next {
			t.Fatalf("homeRoot() = %q, want %q", got, next)
		}
	})
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy dir must be left in place: %v", err)
	}
	if !strings.Contains(stderr, legacy) {
		t.Fatalf("warning must name leftover path %q, got %q", legacy, stderr)
	}
	if !strings.Contains(stderr, next) {
		t.Fatalf("warning must name the path in use %q, got %q", next, stderr)
	}
	if !strings.Contains(stderr, "ignor") {
		t.Fatalf("warning must say the leftover is ignored, got %q", stderr)
	}
}

func TestHomeRootSkipsDualWarnWhenGADAKHOMESet(t *testing.T) {
	root := t.TempDir()
	override := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("GADAK_HOME", override)
	t.Setenv("SCRY_HOME", "")
	if err := os.Mkdir(filepath.Join(root, LegacyDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, DirName), 0o700); err != nil {
		t.Fatal(err)
	}
	dualHomeWarnOnce = sync.Once{}
	stderr := captureStderr(t, func() {
		got, err := homeRoot()
		if err != nil {
			t.Fatal(err)
		}
		if got != override {
			t.Fatalf("homeRoot() = %q, want GADAK_HOME %q", got, override)
		}
	})
	if stderr != "" {
		t.Fatalf("GADAK_HOME override must not warn about ~/.scry: %q", stderr)
	}
}

func TestHomeRootSilentWhenOnlyOneExists(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("GADAK_HOME", "")
	t.Setenv("SCRY_HOME", "")

	next := filepath.Join(root, DirName)
	if err := os.Mkdir(next, 0o700); err != nil {
		t.Fatal(err)
	}
	stderr := captureStderr(t, func() {
		got, err := homeRoot()
		if err != nil {
			t.Fatal(err)
		}
		if got != next {
			t.Fatalf("homeRoot() = %q, want %q", got, next)
		}
	})
	if stderr != "" {
		t.Fatalf("only %s exists; want no warning, got %q", next, stderr)
	}
}

func TestHomeRootMigratesLegacyDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	os.Unsetenv("GADAK_HOME")
	os.Unsetenv("SCRY_HOME")

	legacy := filepath.Join(root, LegacyDirName)
	if err := os.Mkdir(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := homeRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, DirName)
	if got != want {
		t.Fatalf("homeRoot() = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(want, "config.json")); err != nil {
		t.Fatalf("migrated config missing: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy dir still present: %v", err)
	}
}

func TestDBPathMigratesLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	legacy := filepath.Join(home, LegacyDBFile)
	if err := os.WriteFile(legacy, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := DBPathFor("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, DBFile)
	if got != want {
		t.Fatalf("DBPathFor = %q, want %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("migrated db missing: %v", err)
	}
	if _, err := os.Stat(want + "-wal"); err != nil {
		t.Fatalf("migrated wal missing: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy db still present")
	}
}

func TestLoadForMissingAndPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)

	// Missing file: empty config with dir set, no error.
	c, err := LoadFor("aaa")
	if err != nil {
		t.Fatalf("LoadFor missing: %v", err)
	}
	if c.Site != "" || c.Token != "" {
		t.Fatalf("expected empty config, got %+v", c)
	}
	wantDir := filepath.Join(home, "profiles", "aaa")
	if c.dir != wantDir {
		t.Fatalf("dir %q, want %q", c.dir, wantDir)
	}

	// Write via Save (follows dir).
	c.Site = "https://aaa.example.invalid"
	c.Projects = []string{"AAA"}
	c.Email = "user@example.invalid"
	c.Token = "tok-aaa-secret"
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path := filepath.Join(wantDir, "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written to profile dir: %v", err)
	}

	// Reload
	c2, err := LoadFor("aaa")
	if err != nil {
		t.Fatalf("LoadFor: %v", err)
	}
	if c2.Site != "https://aaa.example.invalid" || c2.Token != "tok-aaa-secret" {
		t.Fatalf("reload: %+v", c2)
	}
	if c2.dir != wantDir {
		t.Fatalf("reload dir %q", c2.dir)
	}

	// dir must not appear in JSON on disk.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	if _, ok := probe["dir"]; ok {
		t.Fatalf("dir leaked into JSON: %s", raw)
	}
}

func TestSaveFollowsDirNotGlobalProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })

	// Global profile stays default; LoadFor("bbb") sets dir under profiles/bbb.
	SetProfile("")
	c, err := LoadFor("bbb")
	if err != nil {
		t.Fatal(err)
	}
	c.Site = "https://bbb.example.invalid"
	c.Projects = []string{"BBB"}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// Must not have written the default profile's config.json.
	if _, err := os.Stat(filepath.Join(home, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("default config.json should not exist, err=%v", err)
	}
	bbbPath := filepath.Join(home, "profiles", "bbb", "config.json")
	if _, err := os.Stat(bbbPath); err != nil {
		t.Fatalf("bbb config missing: %v", err)
	}

	// Load() without profile still sees empty default.
	def, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if def.Site != "" {
		t.Fatalf("default Load polluted: %+v", def)
	}
}

func TestLoadAndSaveDefaultUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c.Site = "https://root.example.invalid"
	c.Token = "root-tok"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	// After Load, dir is set — Save must still hit the root config.json.
	if _, err := os.Stat(filepath.Join(home, "config.json")); err != nil {
		t.Fatalf("root config: %v", err)
	}
	c2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c2.Site != "https://root.example.invalid" || c2.Token != "root-tok" {
		t.Fatalf("roundtrip: %+v", c2)
	}
}

func TestSaveCreatesProfileDir0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)

	c, err := LoadFor("sec")
	if err != nil {
		t.Fatal(err)
	}
	c.Site = "https://sec.example.invalid"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "profiles", "sec")
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Errorf("profile dir mode = %04o, want 0700", got)
	}
}

func TestSaveTightensExistingProfileDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)

	dir := filepath.Join(home, "profiles", "loose")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Also leave the GADAK_HOME root loose — Save must tighten the profile dir.
	if err := os.Chmod(home, 0o755); err != nil {
		t.Fatal(err)
	}

	c, err := LoadFor("loose")
	if err != nil {
		t.Fatal(err)
	}
	c.Site = "https://loose.example.invalid"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Errorf("after Save: profile dir mode = %04o, want 0700", got)
	}
}

func TestSaveLeavesOwnerLockedProfileDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)

	dir := filepath.Join(home, "profiles", "locked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	probe := filepath.Join(dir, ".write-probe")
	if f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
		_ = f.Close()
		_ = os.Remove(probe)
		t.Skip("filesystem still allows write as owner; cannot assert owner-locked dir")
	}

	c, err := LoadFor("locked")
	if err != nil {
		t.Fatal(err)
	}
	c.Site = "https://locked.example.invalid"
	if err := c.Save(); err == nil {
		t.Fatal("Save succeeded in owner-locked profile dir")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o555 {
		t.Errorf("after Save: profile dir mode = %04o, want 0555 (owner-locked dir must stay locked)", got)
	}
}

func TestRequireExistingProfileMissingListsAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })

	for _, name := range []string{"demo", "work"} {
		if err := os.MkdirAll(filepath.Join(home, "profiles", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	SetProfile("")
	if err := RequireExistingProfile(); err != nil {
		t.Fatalf("default profile must be allowed: %v", err)
	}

	SetProfile("demo")
	if err := RequireExistingProfile(); err != nil {
		t.Fatalf("existing named profile must be allowed: %v", err)
	}

	SetProfile("nosuch")
	err := RequireExistingProfile()
	if err == nil {
		t.Fatal("missing named profile must error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `workspace "nosuch" not found`) {
		t.Errorf("error %q, want workspace not found", msg)
	}
	// The hint must be pasteable: --workspace is a global flag, so it goes
	// before init (GDK-451 — the old `init --profile` form exits 2).
	if !strings.Contains(msg, `gadak --workspace "nosuch" init`) {
		t.Errorf("error %q, want pasteable init hint", msg)
	}
	if !strings.Contains(msg, "available: demo, work") {
		t.Errorf("error %q, want available: demo, work", msg)
	}
	if _, statErr := os.Stat(filepath.Join(home, "profiles", "nosuch")); !os.IsNotExist(statErr) {
		t.Fatalf("RequireExistingProfile must not create the dir; stat=%v", statErr)
	}
}

func TestApplyVerifiedIdentity(t *testing.T) {
	var c Config
	c.ApplyVerifiedIdentity("acc-1", "Ada", "2026-08-14T00:00:00.000Z")
	if c.AccountID != "acc-1" || c.TokenOwner != "Ada" || c.TokenVerifiedAt != "2026-08-14T00:00:00.000Z" {
		t.Fatalf("got id=%q owner=%q at=%q", c.AccountID, c.TokenOwner, c.TokenVerifiedAt)
	}
	var n *Config
	n.ApplyVerifiedIdentity("x", "y", "z") // nil receiver must not panic
}

func TestSaveRemovesTmpOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config.json")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Config{dir: dir, Site: "https://example.atlassian.net"}
	if err := c.Save(); err == nil {
		t.Fatal("Save: expected rename failure when dest is a directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("leftover config.json.tmp: %v", err)
	}
}

func TestWorkspaceKindDefaultConnected(t *testing.T) {
	if (&Config{}).WorkspaceKind() != KindConnected {
		t.Fatal("empty Kind must be connected")
	}
	if (&Config{Kind: "anything-else"}).WorkspaceKind() != KindConnected {
		t.Fatal("unknown Kind must be connected (no local, no migration)")
	}
	if (&Config{Kind: KindStandalone}).WorkspaceKind() != KindStandalone {
		t.Fatal("standalone Kind")
	}
	if (&Config{}).IsStandalone() {
		t.Fatal("empty must not be standalone")
	}
}

func TestErrNotConfiguredNamesThreeInitPaths(t *testing.T) {
	// GDK-454: the unconfigured sentence is one value; if a verb needs an
	// extra clause it wraps this, it does not invent a sibling.
	msg := ErrNotConfigured.Error()
	for _, want := range []string{
		"not configured",
		"gadak init (Jira)",
		"gadak init --standalone (local)",
		"gadak init --pairing-code (another machine's serve)",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrNotConfigured missing %q: %s", want, msg)
		}
	}
	wrapped := NotConfiguredWith("use `gadak views open KEY`")
	if !errors.Is(wrapped, ErrNotConfigured) {
		t.Fatalf("NotConfiguredWith must wrap ErrNotConfigured, got %v", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "gadak views open KEY") {
		t.Fatalf("addendum dropped: %v", wrapped)
	}
}

func TestSyncFrozenDoesNotChangeHasCredential(t *testing.T) {
	if (*Config)(nil).SyncFrozen() {
		t.Fatal("nil config")
	}
	if (&Config{}).SyncFrozen() {
		t.Fatal("zero config must not be frozen")
	}
	if !(&Config{Frozen: true}).SyncFrozen() {
		t.Fatal("Frozen: true")
	}
	c := &Config{Frozen: true, Site: "https://x.atlassian.net", Email: "a@b.c", Token: "t"}
	if !c.HasCredential() {
		t.Fatal("Frozen must not change HasCredential")
	}
	if !c.SyncFrozen() {
		t.Fatal("connected+frozen")
	}
	if !(&Config{Kind: KindStandalone, Frozen: true}).HasCredential() {
		t.Fatal("standalone Frozen must still report writes possible")
	}
}

func TestFrozenJSONRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	if err := (&Config{Frozen: true}).Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.SyncFrozen() {
		t.Fatal("Load lost frozen")
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["frozen"] != true {
		t.Fatalf("disk frozen=%v, want true; %s", doc["frozen"], raw)
	}

	if err := (&Config{Site: "https://x.example"}).Save(); err != nil {
		t.Fatal(err)
	}
	off, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(off), "frozen") {
		t.Fatalf("omitempty dropped: %s", off)
	}
}

func TestHasCredentialStandaloneVsConnected(t *testing.T) {
	if (*Config)(nil).HasCredential() {
		t.Fatal("nil config")
	}
	// Connected without creds: still blocked.
	if (&Config{}).HasCredential() {
		t.Fatal("empty connected must not have credential")
	}
	if (&Config{Site: "https://x.atlassian.net", Email: "a@b.c"}).HasCredential() {
		t.Fatal("connected missing token must not have credential")
	}
	if !(&Config{Site: "https://x.atlassian.net", Email: "a@b.c", Token: "t"}).HasCredential() {
		t.Fatal("connected with site/email/token")
	}
	// Standalone has no site/email/token and must still allow writes.
	if !(&Config{Kind: KindStandalone}).HasCredential() {
		t.Fatal("standalone must report writes possible")
	}
}

func TestHasCredentialLinear(t *testing.T) {
	if (*Config)(nil).HasCredential() {
		t.Fatal("nil config")
	}
	if (&Config{Linear: &LinearConfig{}}).HasCredential() {
		t.Fatal("linear block without apiKey is not a credential")
	}
	if !(&Config{Linear: &LinearConfig{APIKey: "linear-test-key-not-a-real-secret"}}).HasCredential() {
		t.Fatal("linear apiKey must count as a credential — writes go to Linear")
	}
	c := &Config{Linear: &LinearConfig{APIKey: "k"}}
	if !c.HasLinearCredential() {
		t.Fatal("HasLinearCredential")
	}
	if c.HasAtlassianCredential() {
		t.Fatal("a Linear key must not count as an Atlassian credential")
	}
	if !(&Config{Kind: KindStandalone}).HasAtlassianCredential() {
		t.Fatal("standalone is an Atlassian-family origin")
	}
}

func TestKindRoundTripNoMigrationOfExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	// Existing connected config.json has no kind field.
	legacy := []byte(`{"site":"https://old.example","email":"a@b.c","token":"tok"}`)
	if err := os.WriteFile(filepath.Join(home, "config.json"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kind != "" || cfg.IsStandalone() || cfg.WorkspaceKind() != KindConnected {
		t.Fatalf("legacy config mutated: kind=%q", cfg.Kind)
	}
	if cfg.Site != "https://old.example" {
		t.Fatalf("site %q", cfg.Site)
	}

	cfg.Kind = KindStandalone
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !again.IsStandalone() {
		t.Fatal("standalone did not persist")
	}
	if again.Directory() != home {
		t.Fatalf("Directory %q", again.Directory())
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = old
	_ = w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestWarnUnknownGADAKNamesDBOnStderrNotStdout(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Setenv("GADAK_DB", "/tmp/wrong.db")
	unknownEnvWarnOnce = sync.Once{}

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut := os.Stdout
	os.Stdout = wOut
	stderr := captureStderr(t, func() {
		if _, err := DBPath(); err != nil {
			t.Fatal(err)
		}
	})
	os.Stdout = oldOut
	_ = wOut.Close()
	stdout, _ := io.ReadAll(rOut)

	if !strings.Contains(stderr, "GADAK_DB") {
		t.Fatalf("stderr must name GADAK_DB, got %q", stderr)
	}
	if !strings.Contains(stderr, "unrecognised") {
		t.Fatalf("stderr must say unrecognised, got %q", stderr)
	}
	if !strings.Contains(stderr, "GADAK_HOME") || !strings.Contains(stderr, "GADAK_PROFILE") {
		t.Fatalf("stderr must name the real overrides, got %q", stderr)
	}
	if len(stdout) != 0 {
		t.Fatalf("stdout must stay empty (gadak sql contract), got %q", stdout)
	}
	if strings.Count(stderr, "\n") != 1 || !strings.HasSuffix(stderr, "\n") {
		t.Fatalf("warning must be one line, got %q", stderr)
	}
}

func TestWarnUnknownGADAKSilentWhenOnlyKnown(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Setenv("GADAK_PROFILE", "")
	t.Setenv("GADAK_TOKEN", "x")
	t.Setenv("GADAK_NO_OPEN", "1")
	t.Setenv("GADAK_DESKTOP_CLI", "/bin/true")
	for _, extra := range []string{"GADAK_DB", "GADAK_MEDIA", "GADAK_FRESHEN", "GADAK_PERF"} {
		t.Setenv(extra, "")
	}
	unknownEnvWarnOnce = sync.Once{}
	stderr := captureStderr(t, func() {
		if _, err := DBPath(); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(stderr, "unrecognised") {
		t.Fatalf("known env must not warn, got %q", stderr)
	}
}

func TestWarnUnknownGADAKEmptyIsUnset(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Setenv("GADAK_DB", "")
	for _, extra := range []string{"GADAK_MEDIA", "GADAK_FRESHEN", "GADAK_PERF"} {
		t.Setenv(extra, "")
	}
	unknownEnvWarnOnce = sync.Once{}
	stderr := captureStderr(t, func() {
		if _, err := DBPath(); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(stderr, "GADAK_DB") {
		t.Fatalf("empty GADAK_DB is unset, must not warn, got %q", stderr)
	}
}

func TestWorkspaceSourceFlagEnvDefault(t *testing.T) {
	t.Cleanup(func() { SetProfile("") })

	t.Setenv("GADAK_WORKSPACE", "")
	t.Setenv("GADAK_PROFILE", "")
	t.Setenv("SCRY_PROFILE", "")
	ReloadWorkspaceFromEnv()
	if Profile() != "" {
		t.Fatalf("default Profile() = %q", Profile())
	}
	kind, envName := WorkspaceSource()
	if kind != SourceDefault || envName != "" {
		t.Fatalf("default source = %q %q", kind, envName)
	}

	t.Setenv("GADAK_PROFILE", "pf")
	ReloadWorkspaceFromEnv()
	if Profile() != "pf" {
		t.Fatalf("GADAK_PROFILE Profile() = %q", Profile())
	}
	kind, envName = WorkspaceSource()
	if kind != SourceEnv || envName != "GADAK_PROFILE" {
		t.Fatalf("GADAK_PROFILE source = %q %q", kind, envName)
	}

	t.Setenv("GADAK_WORKSPACE", "ws")
	ReloadWorkspaceFromEnv()
	if Profile() != "ws" {
		t.Fatalf("GADAK_WORKSPACE should win, Profile() = %q", Profile())
	}
	kind, envName = WorkspaceSource()
	if kind != SourceEnv || envName != "GADAK_WORKSPACE" {
		t.Fatalf("GADAK_WORKSPACE source = %q %q", kind, envName)
	}

	SetProfile("flagged")
	if Profile() != "flagged" {
		t.Fatalf("SetProfile Profile() = %q", Profile())
	}
	kind, envName = WorkspaceSource()
	if kind != SourceFlag || envName != "" {
		t.Fatalf("SetProfile source = %q %q", kind, envName)
	}

	SetProfile("")
	if Profile() != "" {
		t.Fatalf("SetProfile empty Profile() = %q", Profile())
	}
	kind, envName = WorkspaceSource()
	if kind != SourceFlag {
		t.Fatalf("SetProfile empty is still flag, got %q", kind)
	}
}

func TestNoPackageInit(t *testing.T) {
	b, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "func init()") {
		t.Fatal("config init() was removed so importing this package does not select a workspace; mains must call ReloadWorkspaceFromEnv")
	}
}

func TestGADAKWorkspaceIsRecognised(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Setenv("GADAK_WORKSPACE", "oss")
	t.Setenv("GADAK_PROFILE", "")
	t.Setenv("GADAK_TOKEN", "")
	t.Setenv("GADAK_DB", "")
	for _, extra := range []string{"GADAK_MEDIA", "GADAK_FRESHEN", "GADAK_PERF"} {
		t.Setenv(extra, "")
	}
	unknownEnvWarnOnce = sync.Once{}
	stderr := captureStderr(t, func() {
		if _, err := DBPath(); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(stderr, "GADAK_WORKSPACE") {
		t.Fatalf("GADAK_WORKSPACE must not be unrecognised, got %q", stderr)
	}
	if strings.Contains(stderr, "unrecognised") {
		t.Fatalf("known env must not warn, got %q", stderr)
	}
}
