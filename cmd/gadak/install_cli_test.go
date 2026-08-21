package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	gadak "github.com/midagedev/gadak"
)

func TestInstallCLINewSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "bin", "gadak")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("#!/bin/sh\necho gadak\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "local-bin")
	var buf bytes.Buffer
	// Put dir on PATH so the success path does not emit the warning.
	if err := installCLI(&buf, source, dir, false, false, dir, "zsh", "linux"); err != nil {
		t.Fatalf("installCLI: %v", err)
	}
	dest := filepath.Join(dir, "gadak")
	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("Readlink: %v\nout:\n%s", err, buf.String())
	}
	if filepath.Clean(got) != filepath.Clean(source) {
		t.Errorf("link target = %q, want %q", got, source)
	}
	out := buf.String()
	if !strings.Contains(out, "installed:") {
		t.Errorf("expected installed: line, got:\n%s", out)
	}
	if !strings.Contains(out, "next: gadak skill install   (Claude Code; for shell-less hosts like Claude Desktop use: gadak mcp install claude)") {
		t.Errorf("expected next-step line, got:\n%s", out)
	}
}

func TestInstallCLIAlreadySameTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "gadak-bin")
	if err := os.WriteFile(source, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "gadak")
	if err := os.Symlink(source, dest); err != nil {
		t.Fatal(err)
	}
	// Capture mtime so we can assert no-op.
	before, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, false, dir, "zsh", "linux"); err != nil {
		t.Fatalf("re-install: %v", err)
	}
	after, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("expected no-op (same mtime); before=%v after=%v", before.ModTime(), after.ModTime())
	}
	out := buf.String()
	if !strings.Contains(out, "already installed") {
		t.Errorf("expected already installed, got:\n%s", out)
	}
	if !strings.Contains(out, "next: gadak skill install   (Claude Code; for shell-less hosts like Claude Desktop use: gadak mcp install claude)") {
		t.Errorf("expected next-step line, got:\n%s", out)
	}
	// Still points at source.
	got, _ := os.Readlink(dest)
	if filepath.Clean(got) != filepath.Clean(source) {
		t.Errorf("link target drifted: %q", got)
	}
}

func TestInstallCLIConflictAndForce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "new-gadak")
	if err := os.WriteFile(source, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "gadak")
	// Pre-existing regular file (not a symlink to source).
	if err := os.WriteFile(dest, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := installCLI(&buf, source, dir, false, false, dir, "zsh", "linux")
	if err == nil {
		t.Fatal("expected error when dest is a different file")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force: %v", err)
	}
	// File still the old content.
	raw, _ := os.ReadFile(dest)
	if string(raw) != "old binary" {
		t.Errorf("dest mutated without --force: %q", raw)
	}

	buf.Reset()
	if err := installCLI(&buf, source, dir, true, false, dir, "zsh", "linux"); err != nil {
		t.Fatalf("--force: %v", err)
	}
	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("after force, expected symlink: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(source) {
		t.Errorf("force link target = %q, want %q", got, source)
	}
	if !strings.Contains(buf.String(), "installed:") {
		t.Errorf("force output:\n%s", buf.String())
	}
}

func TestInstallCLIPrintNoOp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "gadak")
	if err := os.WriteFile(source, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "empty-bin")
	// Do not create dir — --print must not create it either.
	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, true, "", "zsh", "linux"); err != nil {
		t.Fatalf("print: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("--print created dir: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "gadak")); !os.IsNotExist(err) {
		t.Errorf("--print created dest")
	}
	out := buf.String()
	if !strings.Contains(out, "source:") || !strings.Contains(out, "dest:") || !strings.Contains(out, "status:") {
		t.Errorf("print plan missing fields:\n%s", out)
	}
	if !strings.Contains(out, "missing") {
		t.Errorf("print should say missing:\n%s", out)
	}
	if !strings.Contains(out, "would create symlink") {
		t.Errorf("unix print should still say would create symlink:\n%s", out)
	}
	// With force=true still no create under --print.
	buf.Reset()
	if err := installCLI(&buf, source, dir, true, true, "", "zsh", "linux"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("--print --force must not create")
	}
}

func TestInstallCLIPATHWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "gadak")
	if err := os.WriteFile(source, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "off-path-bin")
	var buf bytes.Buffer
	// PATH deliberately excludes dir.
	if err := installCLI(&buf, source, dir, false, false, "/usr/bin:/bin", "zsh", "linux"); err != nil {
		t.Fatalf("install: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "warning:") || !strings.Contains(strings.ToLower(out), "path") {
		t.Errorf("expected PATH warning, got:\n%s", out)
	}
	if !strings.Contains(out, "export PATH=") && !strings.Contains(out, ".zshrc") {
		t.Errorf("expected shell one-liner, got:\n%s", out)
	}
}

func TestInstallCLIConflictOtherSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "new")
	other := filepath.Join(root, "other")
	if err := os.WriteFile(source, []byte("a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("b"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "gadak")
	if err := os.Symlink(other, dest); err != nil {
		t.Fatal(err)
	}
	err := installCLI(io.Discard, source, dir, false, false, dir, "zsh", "linux")
	if err == nil {
		t.Fatal("expected conflict error for different symlink target")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error: %v", err)
	}
}

func TestPathContainsDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	if pathContainsDir("/usr/bin:"+dir+":/bin", dir) != true {
		t.Error("should find dir in PATH")
	}
	if pathContainsDir("/usr/bin:/bin", dir) {
		t.Error("should not find dir")
	}
}

func TestPathExportLineZsh(t *testing.T) {
	line, rc := pathExportLine("/opt/gadak/bin", "/bin/zsh")
	if !strings.Contains(line, ".zshrc") {
		t.Errorf("line = %q", line)
	}
	if rc != "~/.zshrc" {
		t.Errorf("rc = %q", rc)
	}
	if !strings.Contains(line, `export PATH="/opt/gadak/bin:$PATH"`) {
		t.Errorf("export missing: %q", line)
	}
}

func TestResolveInstallCLIDirDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Pin PATH to a directory that is neither ~/.local/bin nor /usr/local/bin.
	// The default-dir policy prefers /usr/local/bin when it is on PATH and
	// writable (clitool.DefaultDir rule 2, added 2026-08-07), so asserting on
	// the host's real PATH made this test answer "is /usr/local/bin writable
	// here" — green on this Mac, red on GitHub runners for four straight CI
	// runs (e.g. run 31195133090). With no suitable candidate on PATH the
	// default must fall back to ~/.local/bin; per-rule coverage is hermetic in
	// internal/clitool/clitool_test.go.
	t.Setenv("PATH", t.TempDir())
	got, err := resolveInstallCLIDir("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "bin")
	if got != want {
		t.Errorf("default dir = %q, want %q", got, want)
	}
}

func TestInstallCLIWindowsCopyPrintAndInstall(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "gadak-src")
	if err := os.WriteFile(source, []byte("cli-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "local-bin")

	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, true, dir, "", "windows"); err != nil {
		t.Fatalf("print: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "symlink") {
		t.Fatalf("windows --print must not say symlink:\n%s", out)
	}
	if !strings.Contains(out, "would copy") {
		t.Fatalf("windows --print should say would copy:\n%s", out)
	}
	if !strings.Contains(out, "gadak.exe") {
		t.Fatalf("windows dest should be gadak.exe:\n%s", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("--print created dir: %v", err)
	}

	buf.Reset()
	if err := installCLI(&buf, source, dir, false, false, dir, "", "windows"); err != nil {
		t.Fatalf("install: %v", err)
	}
	out = buf.String()
	if strings.Contains(out, "symlink") || strings.Contains(out, " → ") {
		t.Fatalf("windows install output must not claim a symlink:\n%s", out)
	}
	if !strings.Contains(out, "copy of") {
		t.Fatalf("windows install should say copy of:\n%s", out)
	}
	dest := filepath.Join(dir, "gadak.exe")
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("windows install created a symlink")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cli-bytes" {
		t.Fatalf("copied = %q", got)
	}

	buf.Reset()
	if err := installCLI(&buf, source, dir, false, false, dir, "", "windows"); err != nil {
		t.Fatalf("re-install: %v", err)
	}
	if !strings.Contains(buf.String(), "already installed") || !strings.Contains(buf.String(), "copy of") {
		t.Fatalf("re-install:\n%s", buf.String())
	}
}

func TestInstallCLIWindowsPATHWarningDoesNotSuggestZshrc(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "gadak")
	if err := os.WriteFile(source, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "off-path-bin")
	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, false, `C:\Windows\System32`, "", "windows"); err != nil {
		t.Fatalf("install: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "warning:") {
		t.Fatalf("expected PATH warning:\n%s", out)
	}
	if strings.Contains(out, ".zshrc") || strings.Contains(out, "export PATH") {
		t.Fatalf("windows PATH hint leaked a unix one-liner:\n%s", out)
	}
	if !strings.Contains(out, "user PATH") {
		t.Fatalf("expected user PATH hint:\n%s", out)
	}
}

func TestExpandHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := expandHomePath("~/.local/bin")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "bin")
	if got != want {
		t.Errorf("expand = %q, want %q", got, want)
	}
}

func seedInstallCLI(t *testing.T) (source, dir string) {
	t.Helper()
	root := t.TempDir()
	source = filepath.Join(root, "bin", "gadak")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("#!/bin/sh\necho gadak\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(root, "local-bin")
	return source, dir
}

// TestInstallCLIAutoInstallsSkillWhenClaudeDirExists — GDK-93.
// FAIL-first (2026-08-21, pre-fix): install-cli succeeded, ~/.claude existed,
// SKILL.md was not created, stdout still said "next: gadak skill install".
func TestInstallCLIAutoInstallsSkillWhenClaudeDirExists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	home := isolateHomeWithClaude(t)
	source, dir := seedInstallCLI(t)
	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, false, dir, "zsh", "linux"); err != nil {
		t.Fatalf("installCLI: %v", err)
	}
	got, err := os.ReadFile(skillDestUnder(home))
	if err != nil {
		t.Fatalf("SKILL.md missing after install-cli: %v\nout:\n%s", err, buf.String())
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatalf("installed bytes differ from embed (%d vs %d)", len(got), len(gadak.SkillMarkdown()))
	}
	out := buf.String()
	if strings.Contains(out, "next: gadak skill install") {
		t.Fatalf("installed skill still printed the next-step line:\n%s", out)
	}
	if !strings.Contains(out, "skill: installed") {
		t.Fatalf("expected skill: installed, got:\n%s", out)
	}
}

func TestInstallCLIAlreadyInstalledAutoInstallsSkill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	home := isolateHomeWithClaude(t)
	source, dir := seedInstallCLI(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(dir, "gadak")); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, false, dir, "zsh", "linux"); err != nil {
		t.Fatalf("re-install: %v", err)
	}
	if _, err := os.Stat(skillDestUnder(home)); err != nil {
		t.Fatalf("SKILL.md missing on already-installed path: %v\nout:\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "already installed") {
		t.Fatalf("expected already installed, got:\n%s", out)
	}
	if strings.Contains(out, "next: gadak skill install") {
		t.Fatalf("already-installed path still printed next-step:\n%s", out)
	}
}

func TestInstallCLISkillSkippedWithoutClaudeDirKeepsNextStep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	home := isolateHome(t)
	source, dir := seedInstallCLI(t)
	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, false, dir, "zsh", "linux"); err != nil {
		t.Fatalf("installCLI: %v", err)
	}
	if _, err := os.Stat(skillDestUnder(home)); !os.IsNotExist(err) {
		t.Fatalf("must not create SKILL.md when ~/.claude is absent: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "next: gadak skill install   (Claude Code; for shell-less hosts like Claude Desktop use: gadak mcp install claude)") {
		t.Fatalf("skipped (no ~/.claude) must keep the next-step line, got:\n%s", out)
	}
}

func TestInstallCLISkillConflictPreservesFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	home := isolateHomeWithClaude(t)
	dest := skillDestUnder(home)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("user-authored skill body\n")
	if err := os.WriteFile(dest, old, 0o644); err != nil {
		t.Fatal(err)
	}
	source, dir := seedInstallCLI(t)
	var buf bytes.Buffer
	_, stderr, err := captureErr(t, func() error {
		return installCLI(&buf, source, dir, false, false, dir, "zsh", "linux")
	})
	if err != nil {
		t.Fatalf("conflict must not fail install-cli: %v\nstdout=%s\nstderr=%s", err, buf.String(), stderr)
	}
	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, old) {
		t.Fatalf("conflict overwrote the user file")
	}
	if !strings.Contains(stderr, "gadak skill install --force") {
		t.Fatalf("stderr must name gadak skill install --force, got:\n%s", stderr)
	}
	if strings.Contains(buf.String(), "next: gadak skill install") {
		t.Fatalf("conflict skip must not print the unforced next-step:\n%s", buf.String())
	}
}

func TestInstallCLIPrintDoesNotAutoInstallSkill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	home := isolateHomeWithClaude(t)
	source, dir := seedInstallCLI(t)
	var buf bytes.Buffer
	if err := installCLI(&buf, source, dir, false, true, dir, "zsh", "linux"); err != nil {
		t.Fatalf("print: %v", err)
	}
	if _, err := os.Stat(skillDestUnder(home)); !os.IsNotExist(err) {
		t.Fatalf("--print must not write SKILL.md: %v", err)
	}
}
