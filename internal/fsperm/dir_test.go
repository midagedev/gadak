package fsperm

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func skipWindowsPerm(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestEnsurePrivateDirCreates0700(t *testing.T) {
	skipWindowsPerm(t)
	dir := filepath.Join(t.TempDir(), "new")
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	if got := fileMode(t, dir); got != 0o700 {
		t.Errorf("new dir mode = %04o, want 0700", got)
	}
}

func TestEnsurePrivateDirTightens0755(t *testing.T) {
	skipWindowsPerm(t)
	dir := filepath.Join(t.TempDir(), "loose")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := fileMode(t, dir); got != 0o755 {
		t.Fatalf("setup: dir mode %04o, want 0755", got)
	}
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	if got := fileMode(t, dir); got != 0o700 {
		t.Errorf("after EnsurePrivateDir: mode = %04o, want 0700", got)
	}
}

func TestEnsurePrivateDirLeaves0555(t *testing.T) {
	skipWindowsPerm(t)
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if got := fileMode(t, dir); got != 0o555 {
		t.Fatalf("setup: dir mode %04o, want 0555", got)
	}
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	if got := fileMode(t, dir); got != 0o555 {
		t.Errorf("owner-locked dir mode = %04o, want 0555", got)
	}
}

func TestEnsurePrivateDirFileIsFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := EnsurePrivateDir(path)
	if err == nil {
		t.Fatal("want error when path is a file")
	}
	if errors.Is(err, ErrChmod) {
		t.Fatalf("file path must not be a chmod warning: %v", err)
	}
}
