package config

// GDK-1233: both saves in this package (Config.Save and the stored-default
// write) used to stage through one fixed "<dest>.tmp" path. Two savers
// sharing a profile — serve's settings PUT and a CLI verb, often separate
// processes — therefore shared one staging file, and os.WriteFile truncates
// before it writes, so the loser's half-written bytes could be renamed onto
// the credential document. These tests pin the replacement contract: every
// save stages through a unique temp name in the destination directory, the
// final file stays 0600 and complete JSON, and a failed save leaves no
// staging file behind.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestSaveIgnoresSiblingAtFixedTmpName is the deterministic form of the
// collision: a sibling holding the old fixed name is exactly what a
// concurrent in-flight save used to leave behind, so a save that still
// insisted on one staging path would fail against it.
func TestSaveIgnoresSiblingAtFixedTmpName(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "config.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := &Config{dir: dir, Site: "https://example.atlassian.net", Token: "tok"}
	if err := c.Save(); err != nil {
		t.Fatalf("Save must not share a staging path with a sibling: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back Config
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("final file not complete JSON: %v\n%s", err, raw)
	}
	if back.Site != c.Site || back.Token != c.Token {
		t.Fatalf("roundtrip mismatch: got %+v", back)
	}
}

// TestStoredWorkspaceSaveIgnoresSiblingAtFixedTmpName: the stored default
// goes through writeStoredWorkspace, the second fixed-name staging path.
func TestStoredWorkspaceSaveIgnoresSiblingAtFixedTmpName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")
	if err := os.MkdirAll(filepath.Join(home, "profiles", "oss"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "default-workspace.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SetStoredWorkspace("oss"); err != nil {
		t.Fatalf("SetStoredWorkspace must not share a staging path with a sibling: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(home, "default-workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "oss\n" {
		t.Fatalf("stored default = %q, want %q", b, "oss\n")
	}
}

// TestSaveDoesNotInheritSiblingTmpMode: os.WriteFile never chmods an
// existing file, so the old fixed-name path wrote through a leftover staging
// file and renamed its loose mode onto the credential document. A fresh
// staging file per save is created 0600 and must be the only thing renamed.
func TestSaveDoesNotInheritSiblingTmpMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json.tmp"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Config{dir: dir, Token: "tok"}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("config.json mode = %04o, want 0600 — staging must not widen the credential file", got)
	}
}

// TestSaveFileMode0600 is the plain no-widening guard for both saved files.
func TestSaveFileMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	if err := (&Config{dir: dir, Token: "tok"}).Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("config.json mode = %04o, want 0600", got)
	}

	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")
	if err := os.MkdirAll(filepath.Join(home, "profiles", "oss"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SetStoredWorkspace("oss"); err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(filepath.Join(home, "default-workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sfi.Mode().Perm(); got != 0o600 {
		t.Errorf("default-workspace mode = %04o, want 0600", got)
	}
}

// TestSaveConcurrentUniqueStaging races many saves onto one profile dir.
// With a shared staging name a writer can rename the file another writer
// truncated, surfacing as a failed rename or a torn final document; with
// per-save staging every save must succeed and leave exactly config.json.
func TestSaveConcurrentUniqueStaging(t *testing.T) {
	dir := t.TempDir()
	const writers = 8
	const saves = 25
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			c := &Config{
				dir:            dir,
				Site:           "https://example.atlassian.net",
				Token:          "tok",
				DefaultProject: fmt.Sprintf("W%d", w),
			}
			for i := 0; i < saves; i++ {
				if err := c.Save(); err != nil {
					errs <- fmt.Errorf("writer %d: %w", w, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back Config
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("final file not complete JSON: %v\n%s", err, raw)
	}
	if !strings.HasPrefix(back.DefaultProject, "W") || len(back.DefaultProject) < 2 {
		t.Fatalf("final defaultProject %q not from any writer", back.DefaultProject)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Errorf("leftover staging file after concurrent saves: %s", e.Name())
		}
	}
}
