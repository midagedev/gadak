package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirForDefaultAndNamed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
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
	if db != filepath.Join(want, "scry.db") {
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

func TestLoadForMissingAndPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)

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
	t.Setenv("SCRY_HOME", home)
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
	t.Setenv("SCRY_HOME", home)
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
	t.Setenv("SCRY_HOME", home)

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
	t.Setenv("SCRY_HOME", home)

	dir := filepath.Join(home, "profiles", "loose")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Also leave the SCRY_HOME root loose — Save must tighten the profile dir.
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
