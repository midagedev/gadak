package clitool

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallMethodForWindowsIsCopy(t *testing.T) {
	if got := installMethodFor("windows"); got != MethodCopy {
		t.Fatalf("installMethodFor(windows) = %q, want %q", got, MethodCopy)
	}
	if got := destBaseFor("windows"); got != "gadak.exe" {
		t.Fatalf("destBaseFor(windows) = %q, want %q", got, "gadak.exe")
	}
	if got := installMethodFor("darwin"); got != MethodSymlink {
		t.Fatalf("installMethodFor(darwin) = %q, want %q", got, MethodSymlink)
	}
	if got := destBaseFor("linux"); got != "gadak" {
		t.Fatalf("destBaseFor(linux) = %q, want %q", got, "gadak")
	}
}

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

func TestDefaultDirForWindows(t *testing.T) {
	t.Run("localappdata_set", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("LOCALAPPDATA", base)
		got, err := DefaultDirFor("/usr/bin:"+filepath.Join("C:", "Windows"), "windows")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(base, "Programs", "gadak")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
		if strings.Contains(got, ".local") {
			t.Fatalf("windows default leaked a unix path: %q", got)
		}
	})
	t.Run("localappdata_empty", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("LOCALAPPDATA", "")
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		got, err := DefaultDirFor("", "windows")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, "AppData", "Local", "Programs", "gadak")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
		if strings.Contains(got, ".local") {
			t.Fatalf("windows default leaked a unix path: %q", got)
		}
	})
}

func TestPermissionHintForWindowsOmitsSudo(t *testing.T) {
	err := permissionHintFor(os.ErrPermission, `C:\Windows\System32`, "windows")
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	s := err.Error()
	if strings.Contains(strings.ToLower(s), "sudo") {
		t.Fatalf("windows hint leaked sudo: %q", s)
	}
	if !strings.Contains(s, "--dir") {
		t.Fatalf("windows hint should mention --dir: %q", s)
	}
}

func TestPermissionHintForUnixKeepsSudo(t *testing.T) {
	err := permissionHintFor(os.ErrPermission, "/usr/local/bin", "darwin")
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("unix hint lost sudo: %q", err)
	}
}

func TestInstallRecordsDesktopExePath(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(bundle, "gadak.exe")
	desktop := filepath.Join(bundle, "gadak-desktop.exe")
	if err := os.WriteFile(source, []byte("cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktop, []byte("desk"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(root, "desktop-exe-path")
	prev := DesktopExePathFile
	DesktopExePathFile = rec
	t.Cleanup(func() { DesktopExePathFile = prev })

	dir := filepath.Join(root, "bin")
	p, err := ResolveFor(source, dir, dir, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(p, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(rec)
	if err != nil {
		t.Fatalf("record file missing: %v", err)
	}
	got := strings.TrimSpace(string(raw))
	abs, err := filepath.Abs(desktop)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("recorded %q want %q", got, abs)
	}
	if RecordedDesktopExe() != abs {
		t.Fatalf("RecordedDesktopExe = %q want %q", RecordedDesktopExe(), abs)
	}

	if err := os.Remove(desktop); err != nil {
		t.Fatal(err)
	}
	if RecordedDesktopExe() != "" {
		t.Fatalf("missing target must be empty, got %q", RecordedDesktopExe())
	}
}

func TestInstallRecordFailureDoesNotFailInstall(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(bundle, "gadak.exe")
	desktop := filepath.Join(bundle, "gadak-desktop.exe")
	if err := os.WriteFile(source, []byte("cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktop, []byte("desk"), 0o755); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := DesktopExePathFile
	DesktopExePathFile = filepath.Join(blocker, "desktop-exe-path")
	t.Cleanup(func() { DesktopExePathFile = prev })

	dir := filepath.Join(root, "bin")
	p, err := ResolveFor(source, dir, dir, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(p, false); err != nil {
		t.Fatalf("record write failure must not fail install: %v", err)
	}
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

func TestResolveForWindowsPlansCopy(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "gadak-bin")
	if err := os.WriteFile(source, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	p, err := ResolveFor(source, dir, dir, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != MethodCopy {
		t.Fatalf("Method = %q, want %q", p.Method, MethodCopy)
	}
	if filepath.Base(p.Dest) != "gadak.exe" {
		t.Fatalf("Dest base = %q, want gadak.exe (Dest=%q)", filepath.Base(p.Dest), p.Dest)
	}
	if p.Status != StatusMissing {
		t.Fatalf("status = %q, want missing", p.Status)
	}
	if p.PrintStatusLine() != "status:  missing (would copy)" {
		t.Fatalf("print = %q", p.PrintStatusLine())
	}
}

func TestInstallForWindowsCopies(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "gadak-bin")
	body := []byte("windows-cli-bytes")
	if err := os.WriteFile(source, body, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	p, err := ResolveFor(source, dir, dir, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(p, false); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(p.Dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("windows install created a symlink")
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("dest mode = %v, want regular file", fi.Mode())
	}
	got, err := os.ReadFile(p.Dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("copied bytes = %q, want %q", got, body)
	}

	p2, err := ResolveFor(source, dir, dir, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if p2.Status != StatusLinked {
		t.Fatalf("re-resolve status = %q, want linked", p2.Status)
	}
	if err := Install(p2, false); err != nil {
		t.Fatalf("re-install no-op: %v", err)
	}
	if p2.AlreadyInstalledLine() != fmt.Sprintf("already installed: %s (copy of %s)", TildeHome(p2.Dest), TildeHome(p2.Source)) {
		t.Fatalf("already line = %q", p2.AlreadyInstalledLine())
	}
}

func TestInstallForWindowsConflictRequiresForce(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "new")
	if err := os.WriteFile(source, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "gadak.exe")
	if err := os.WriteFile(dest, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := ResolveFor(source, dir, dir, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusConflict {
		t.Fatalf("status = %q, want conflict", p.Status)
	}
	if err := Install(p, false); err == nil {
		t.Fatal("expected conflict error")
	}
	raw, _ := os.ReadFile(dest)
	if string(raw) != "old binary" {
		t.Fatalf("dest mutated without --force: %q", raw)
	}
	if err := Install(p, true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("after force: %q", got)
	}
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("force install created a symlink")
	}
}

func TestPathExportAdviseForWindowsNamesDirOnly(t *testing.T) {
	line, rc := PathExportAdviseFor(`/Users/x/.local/bin`, "", "windows")
	if rc != "" {
		t.Fatalf("rc = %q, want empty (no verified Windows rc)", rc)
	}
	if !strings.Contains(line, "user PATH") {
		t.Fatalf("line = %q", line)
	}
	if strings.Contains(line, "export PATH") || strings.Contains(line, ".zshrc") {
		t.Fatalf("windows advise leaked a unix one-liner: %q", line)
	}
}

func TestResolveAndInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-cli unsupported on windows")
	}
	root := t.TempDir()
	source := filepath.Join(root, "gadak-bin")
	if err := os.WriteFile(source, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "bin")
	pathEnv := dir

	p, err := ResolveFor(source, dir, pathEnv, runtime.GOOS)
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

	p2, err := ResolveFor(source, dir, pathEnv, runtime.GOOS)
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
	dest := filepath.Join(dir, "gadak")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := ResolveFor(source, dir, dir, runtime.GOOS)
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
	line, _ := PathExportAdvise("/opt/gadak/bin", "/bin/zsh")
	if !strings.Contains(line, ".zshrc") || !strings.Contains(line, `export PATH="/opt/gadak/bin:$PATH"`) {
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
