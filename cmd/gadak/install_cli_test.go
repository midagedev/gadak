package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if err := installCLI(&buf, source, dir, false, false, dir, "zsh"); err != nil {
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
	if err := installCLI(&buf, source, dir, false, false, dir, "zsh"); err != nil {
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
	err := installCLI(&buf, source, dir, false, false, dir, "zsh")
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
	if err := installCLI(&buf, source, dir, true, false, dir, "zsh"); err != nil {
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
	if err := installCLI(&buf, source, dir, false, true, "", "zsh"); err != nil {
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
	// With force=true still no create under --print.
	buf.Reset()
	if err := installCLI(&buf, source, dir, true, true, "", "zsh"); err != nil {
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
	if err := installCLI(&buf, source, dir, false, false, "/usr/bin:/bin", "zsh"); err != nil {
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
	err := installCLI(io.Discard, source, dir, false, false, dir, "zsh")
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
