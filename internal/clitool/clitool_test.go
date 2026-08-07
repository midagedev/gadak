package clitool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultDirThreeCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix paths")
	}

	// Case A: ~/.local/bin exists and is on PATH → pick it (even if a writable
	// system candidate is also on PATH).
	t.Run("local_bin_on_path", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		localBin := filepath.Join(home, ".local", "bin")
		if err := os.MkdirAll(localBin, 0o755); err != nil {
			t.Fatal(err)
		}
		sysBin := t.TempDir()
		prev := systemBinCandidate
		systemBinCandidate = sysBin
		t.Cleanup(func() { systemBinCandidate = prev })

		pathEnv := localBin + string(os.PathListSeparator) + sysBin
		got, err := DefaultDir(pathEnv)
		if err != nil {
			t.Fatal(err)
		}
		if got != localBin {
			t.Fatalf("got %q want %q", got, localBin)
		}
	})

	// Case B: ~/.local/bin absent; system candidate on PATH and writable → pick it.
	t.Run("system_bin_writable_on_path", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		// Do not create ~/.local/bin.
		sysBin := t.TempDir()
		prev := systemBinCandidate
		systemBinCandidate = sysBin
		t.Cleanup(func() { systemBinCandidate = prev })

		pathEnv := sysBin + string(os.PathListSeparator) + "/usr/bin"
		got, err := DefaultDir(pathEnv)
		if err != nil {
			t.Fatal(err)
		}
		if got != sysBin {
			t.Fatalf("got %q want %q", got, sysBin)
		}
	})

	// Case C: neither rule 1 nor 2 → ~/.local/bin fallback.
	t.Run("fallback_local_bin", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		// System candidate exists but is NOT on PATH → skip rule 2.
		sysBin := t.TempDir()
		prev := systemBinCandidate
		systemBinCandidate = sysBin
		t.Cleanup(func() { systemBinCandidate = prev })

		got, err := DefaultDir("/usr/bin:/bin")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".local", "bin")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
}

func TestDefaultDirSkipsUnwritableSystemBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Read-only stand-in for root-owned /usr/local/bin.
	parent := t.TempDir()
	sysBin := filepath.Join(parent, "bin")
	if err := os.Mkdir(sysBin, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sysBin, 0o755) })
	if isWritableDir(sysBin) {
		t.Skip("filesystem still allows write as owner; cannot assert skip")
	}
	prev := systemBinCandidate
	systemBinCandidate = sysBin
	t.Cleanup(func() { systemBinCandidate = prev })

	got, err := DefaultDir(sysBin + string(os.PathListSeparator) + "/usr/bin")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "bin")
	if got != want {
		t.Fatalf("got %q want fallback %q", got, want)
	}
}

func TestResolveDirExplicitSkipsHeuristic(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "custom-bin")
	got, err := ResolveDir(dir, "/usr/bin")
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("explicit dir = %q, want %q", got, dir)
	}
}

func TestResolveAndInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "scry-bin")
	if err := os.WriteFile(source, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	pathEnv := dir

	p, err := Resolve(source, dir, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusMissing {
		t.Fatalf("status = %q, want missing", p.Status)
	}
	if !p.OnPath {
		t.Error("expected OnPath")
	}
	if err := Install(p, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(p.Dest)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(source) {
		t.Errorf("link = %q, want %q", got, source)
	}

	p2, err := Resolve(source, dir, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Status != StatusLinked {
		t.Errorf("status = %q, want linked", p2.Status)
	}
	if err := Install(p2, false); err != nil {
		t.Errorf("re-install no-op: %v", err)
	}
}

func TestInstallConflictRequiresForce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "new")
	if err := os.WriteFile(source, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "scry")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := Resolve(source, dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusConflict {
		t.Fatalf("status = %q, want conflict", p.Status)
	}
	if err := Install(p, false); err == nil {
		t.Fatal("expected conflict error")
	} else if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error: %v", err)
	}
	if err := Install(p, true); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(source) {
		t.Errorf("after force: %q", got)
	}
}

func TestPathContainsAndExport(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	if !PathContains("/usr/bin:"+dir+":/bin", dir) {
		t.Error("should find dir")
	}
	if PathContains("/usr/bin:/bin", dir) {
		t.Error("should not find dir")
	}
	line := PathExportLine("/opt/scry/bin", "/bin/zsh")
	if !strings.Contains(line, ".zshrc") || !strings.Contains(line, `export PATH="/opt/scry/bin:$PATH"`) {
		t.Errorf("line = %q", line)
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := ExpandHome("~/.local/bin")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "bin")
	if got != want {
		t.Errorf("expand = %q, want %q", got, want)
	}
}
