// Package clitool installs the gadak binary onto PATH.
// Unix uses a symlink; Windows copies (symlink creation needs Developer
// Mode or elevation, so the default path fails). Shared by
// `gadak install-cli` and the desktop "Install Command Line Tool…" menu
// so both surfaces use the same resolve / install / PATH-advice logic.
//
// LookPathThen / ResolveNPM / RaycastExtDir live here too: the CLI install
// verb and the desktop catalog must try the same candidates.
package clitool

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Destination status after Resolve.
const (
	StatusMissing  = "missing"  // nothing at Dest
	StatusLinked   = "linked"   // Dest is already the desired install (symlink or copy)
	StatusConflict = "conflict" // Dest exists but is not the desired install
)

// How Install places Dest. Windows copies: creating a symlink needs Developer
// Mode or elevation, so the default path fails. Unix keeps a symlink.
const (
	methodSymlink = "symlink"
	methodCopy    = "copy"
)

// Plan is the result of Resolve: where to install, what is there, and whether
// the install directory is already on PATH.
type Plan struct {
	Source string
	Dir    string
	Dest   string
	// Status is one of StatusMissing, StatusLinked, StatusConflict.
	Status string
	OnPath bool

	// ExistingKind / ExistingTarget describe a conflict (or the current link).
	// Kind is "symlink", "file", "directory", or "other".
	ExistingKind   string
	ExistingTarget string // absolute path for symlinks; empty otherwise

	// Method is methodSymlink or methodCopy. Set by ResolveFor from goos.
	Method string
}

// installMethodFor chooses how Install places Dest. goos is a GOOS value so
// tests can exercise the Windows branch on any host (same shape as
// desktop windowChromeFor).
func installMethodFor(goos string) string {
	if goos == "windows" {
		return methodCopy
	}
	return methodSymlink
}

// destBaseFor is the filename under the install directory. Windows PATH
// lookup requires the .exe suffix.
func destBaseFor(goos string) string {
	if goos == "windows" {
		return "gadak.exe"
	}
	return "gadak"
}

// ResolveFor picks the install directory and inspects Dest. goos is an
// explicit GOOS value so the Windows dest name and copy/symlink choice
// can be tested on any host (callers pass runtime.GOOS in production).
//
// dirFlag empty → DefaultDir(pathEnv) heuristic. Non-empty → expand/absolutize
// that path and skip the heuristic. pathEnv is the PATH string (usually
// os.Getenv("PATH")); inject a fake value in tests.
func ResolveFor(source, dirFlag, pathEnv, goos string) (Plan, error) {
	source = filepath.Clean(source)
	dir, err := ResolveDir(dirFlag, pathEnv)
	if err != nil {
		return Plan{}, err
	}
	dest := filepath.Join(dir, destBaseFor(goos))

	p := Plan{
		Source: source,
		Dir:    dir,
		Dest:   dest,
		OnPath: PathContains(pathEnv, dir),
		Method: installMethodFor(goos),
	}

	kind, target, existErr := inspectDest(dest)
	if existErr != nil && !os.IsNotExist(existErr) {
		return Plan{}, fmt.Errorf("inspect %s: %w", TildeHome(dest), existErr)
	}
	if os.IsNotExist(existErr) {
		p.Status = StatusMissing
		return p, nil
	}
	p.ExistingKind = kind
	p.ExistingTarget = target
	if kind == "symlink" && target == source {
		p.Status = StatusLinked
		return p, nil
	}
	if p.Method == methodCopy && kind == "file" {
		same, sameErr := sameRegularFile(dest, source)
		if sameErr != nil {
			return Plan{}, fmt.Errorf("compare %s: %w", TildeHome(dest), sameErr)
		}
		if same {
			p.Status = StatusLinked
			return p, nil
		}
	}
	p.Status = StatusConflict
	return p, nil
}

// ResolveDir returns the absolute install directory.
// Empty flag → DefaultDir(pathEnv). Leading ~ is expanded.
func ResolveDir(dirFlag, pathEnv string) (string, error) {
	if dirFlag == "" {
		return DefaultDir(pathEnv)
	}
	expanded, err := ExpandHome(dirFlag)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// systemBinCandidate is the second-choice install directory (on PATH + writable).
// Production default is /usr/local/bin; tests override to a temp path so they
// never touch the real system directory.
var systemBinCandidate = "/usr/local/bin"

// DesktopExePathFile is the pointer file written after a successful
// install-cli when gadak-desktop.exe sat next to the source binary.
// Empty means <UserConfigDir>/gadak/desktop-exe-path. Tests override
// to a temp path so they never touch the real UserConfigDir.
// Not GADAK_HOME: that directory is store-owned.
var DesktopExePathFile string

// DefaultDir chooses an install directory when --dir is omitted.
func DefaultDir(pathEnv string) (string, error) {
	return defaultDirFor(pathEnv, runtime.GOOS)
}

// defaultDirFor is DefaultDir with an explicit GOOS so the Windows
// default can be tested on any host (same shape as installMethodFor).
//
// Unix order:
//  1. ~/.local/bin if it exists and is already on PATH
//  2. /usr/local/bin if it is on PATH and writable without sudo
//  3. ~/.local/bin otherwise (create on Install; caller may warn about PATH)
//
// Windows: %LOCALAPPDATA%\Programs\gadak (empty LOCALAPPDATA →
// UserHomeDir + AppData\Local). Unix candidates are not considered.
func defaultDirFor(pathEnv, goos string) (string, error) {
	if goos == "windows" {
		return defaultDirWindows()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	localBin := filepath.Join(home, ".local", "bin")

	// Prefer a directory the user's shell already searches, so `gadak` works
	// immediately after install without editing rc files.
	//
	// 1. ~/.local/bin — common XDG-style location; only when it already exists
	//    and appears on PATH (creating it when it is not on PATH still leaves
	//    the user with "command not found", so existence alone is not enough).
	if dirExists(localBin) && PathContains(pathEnv, localBin) {
		return localBin, nil
	}
	// 2. /usr/local/bin — on macOS this is usually on the default PATH. Use it
	//    only when writable (skip root-owned trees; never prompt for sudo).
	//    systemBinCandidate is /usr/local/bin in production; tests redirect it.
	if PathContains(pathEnv, systemBinCandidate) && isWritableDir(systemBinCandidate) {
		return systemBinCandidate, nil
	}
	// 3. Fall back to ~/.local/bin (create later). Caller should surface a PATH
	//    export hint when OnPath is false.
	return localBin, nil
}

func defaultDirWindows() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home directory: %w", err)
		}
		local = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(local, "Programs", "gadak"), nil
}

// Install creates Dir if needed and places Dest according to p.Method
// (symlink on Unix, copy on Windows). Empty Method is treated as symlink
// so a Plan built by older callers keeps the previous behaviour.
// When Status is linked, Install does not touch Dest (it may still record
// gadak-desktop.exe next to Source as a discovery fallback).
// When Status is conflict and force is false, Install returns an error without
// touching the filesystem. force replaces an existing Dest.
func Install(p Plan, force bool) error {
	if p.Status == StatusLinked {
		recordDesktopExeIfPresent(p.Source)
		return nil
	}
	if p.Status == StatusConflict && !force {
		switch p.ExistingKind {
		case "symlink":
			return fmt.Errorf("%s already exists and points to %s (not %s); re-run with --force to replace",
				TildeHome(p.Dest), TildeHome(p.ExistingTarget), TildeHome(p.Source))
		default:
			return fmt.Errorf("%s already exists (%s); re-run with --force to replace",
				TildeHome(p.Dest), p.ExistingKind)
		}
	}

	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return permissionHint(fmt.Errorf("create directory %s: %w", TildeHome(p.Dir), err), p.Dir)
	}

	// Remove a conflicting entry when force (or re-check existence for races).
	if _, err := os.Lstat(p.Dest); err == nil {
		if err := os.Remove(p.Dest); err != nil {
			return permissionHint(fmt.Errorf("remove %s: %w", TildeHome(p.Dest), err), p.Dest)
		}
	} else if !os.IsNotExist(err) {
		return permissionHint(fmt.Errorf("inspect %s: %w", TildeHome(p.Dest), err), p.Dest)
	}

	if p.Method == methodCopy {
		if err := installCopy(p.Source, p.Dest); err != nil {
			return permissionHint(fmt.Errorf("copy %s from %s: %w", TildeHome(p.Dest), TildeHome(p.Source), err), p.Dest)
		}
		recordDesktopExeIfPresent(p.Source)
		return nil
	}

	if err := os.Symlink(p.Source, p.Dest); err != nil {
		return permissionHint(fmt.Errorf("symlink %s → %s: %w", TildeHome(p.Dest), TildeHome(p.Source), err), p.Dest)
	}
	recordDesktopExeIfPresent(p.Source)
	return nil
}

// PrintStatusLine is the `gadak install-cli --print` status line. Ownership
// lives here so the CLI cannot drift into saying "symlink" for a copy.
func (p Plan) PrintStatusLine() string {
	switch p.Status {
	case StatusMissing:
		if p.Method == methodCopy {
			return "status:  missing (would copy)"
		}
		return "status:  missing (would create symlink)"
	case StatusLinked:
		if p.Method == methodCopy {
			return "status:  already installed (same copy)"
		}
		return "status:  already installed (same target)"
	case StatusConflict:
		if p.ExistingKind == "symlink" {
			return fmt.Sprintf("status:  symlink → %s (use --force to replace)", TildeHome(p.ExistingTarget))
		}
		return fmt.Sprintf("status:  %s exists (use --force to replace)", p.ExistingKind)
	default:
		return fmt.Sprintf("status:  %s", p.Status)
	}
}

// InstalledLine is printed after a successful Install.
func (p Plan) InstalledLine() string {
	if p.Method == methodCopy {
		return fmt.Sprintf("installed: %s (copy of %s)", TildeHome(p.Dest), TildeHome(p.Source))
	}
	return fmt.Sprintf("installed: %s → %s", TildeHome(p.Dest), TildeHome(p.Source))
}

// AlreadyInstalledLine is printed when Status is already linked/copied.
func (p Plan) AlreadyInstalledLine() string {
	if p.Method == methodCopy {
		return fmt.Sprintf("already installed: %s (copy of %s)", TildeHome(p.Dest), TildeHome(p.Source))
	}
	return fmt.Sprintf("already installed: %s → %s", TildeHome(p.Dest), TildeHome(p.Source))
}

// PathExportAdviseFor is PathExportAdvise with an explicit GOOS. Windows has
// no verified one-liner in this package (setx truncates PATH; this runner
// has not executed a Windows PATH edit), so the line only names the directory.
func PathExportAdviseFor(dir, shell, goos string) (line, rc string) {
	if goos == "windows" {
		return fmt.Sprintf("add %s to your user PATH", TildeHome(dir)), ""
	}
	return PathExportAdvise(dir, shell)
}

// PathExportAdvise returns a pasteable one-liner and a suggested shell rc path
// for persistence (rc may be empty). Unix shells only; Windows uses
// PathExportAdviseFor.
func PathExportAdvise(dir, shell string) (line, rc string) {
	// Prefer $HOME-relative form when dir is under home so the snippet is portable.
	display := dir
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		homeClean := filepath.Clean(home)
		if filepath.Clean(dir) == homeClean {
			display = "$HOME"
		} else if strings.HasPrefix(filepath.Clean(dir), homeClean+string(filepath.Separator)) {
			display = "$HOME" + filepath.Clean(dir)[len(homeClean):]
		}
	}
	export := fmt.Sprintf(`export PATH="%s:$PATH"`, display)

	base := filepath.Base(shell)
	switch base {
	case "zsh":
		return fmt.Sprintf(`echo '%s' >> ~/.zshrc`, export), "~/.zshrc"
	case "bash":
		// macOS bash often uses .bash_profile; Linux uses .bashrc. Suggest .bashrc
		// and mention .bash_profile for login shells.
		return fmt.Sprintf(`echo '%s' >> ~/.bashrc`, export), "~/.bashrc"
	case "fish":
		return fmt.Sprintf(`fish_add_path %s`, display), "~/.config/fish/config.fish"
	default:
		// zsh is the macOS default and common on developer Linux; use it when unknown.
		return fmt.Sprintf(`echo '%s' >> ~/.zshrc`, export), "~/.zshrc"
	}
}

// PathContains reports whether dir appears as a PATH entry (exact, after Clean).
// Empty entries are ignored. Leading ~ in pathEnv entries is expanded.
func PathContains(pathEnv, dir string) bool {
	dir = filepath.Clean(dir)
	for _, p := range filepath.SplitList(pathEnv) {
		if p == "" {
			continue
		}
		if filepath.Clean(p) == dir {
			return true
		}
		if expanded, err := ExpandHome(p); err == nil && filepath.Clean(expanded) == dir {
			return true
		}
	}
	return false
}

// ExpandHome expands a leading ~ or ~/… using os.UserHomeDir. Other paths
// are returned cleaned as-is.
func ExpandHome(p string) (string, error) {
	if p == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home directory: %w", err)
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// TildeHome shortens an absolute path under the user's home to ~/… for display.
func TildeHome(path string) string {
	if path == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean(path)
	}
	clean := filepath.Clean(path)
	homeClean := filepath.Clean(home)
	if clean == homeClean {
		return "~"
	}
	sep := string(filepath.Separator)
	if strings.HasPrefix(clean, homeClean+sep) {
		return "~" + clean[len(homeClean):]
	}
	return clean
}

// inspectDest classifies dest: "symlink" (with stored link target, not
// EvalSymlinks of the ultimate file), "file", "directory", or "other".
// On not-exist, returns ("", "", err) with os.IsNotExist.
func inspectDest(dest string) (kind, linkTarget string, err error) {
	fi, err := os.Lstat(dest)
	if err != nil {
		return "", "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, rerr := os.Readlink(dest)
		if rerr != nil {
			return "symlink", "", rerr
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(dest), target)
		}
		return "symlink", filepath.Clean(target), nil
	}
	if fi.IsDir() {
		return "directory", "", nil
	}
	if fi.Mode().IsRegular() {
		return "file", "", nil
	}
	return "other", "", nil
}

func permissionHint(err error, path string) error {
	return permissionHintFor(err, path, runtime.GOOS)
}

func permissionHintFor(err error, path, goos string) error {
	if err == nil {
		return nil
	}
	if !isPermissionError(err) {
		return err
	}
	if goos == "windows" {
		return fmt.Errorf("%w\npermission denied writing %s — check the directory is writable, or pass --dir with a path you can write",
			err, TildeHome(path))
	}
	return fmt.Errorf("%w\npermission denied writing %s — re-run with sudo, or use --dir ~/.local/bin (no sudo)",
		err, TildeHome(path))
}

func desktopExeRecordPath() string {
	if DesktopExePathFile != "" {
		return DesktopExePathFile
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return ""
	}
	return filepath.Join(base, "gadak", "desktop-exe-path")
}

// recordDesktopExeIfPresent writes the absolute path of gadak-desktop.exe
// sitting next to source. Write failure is ignored — the file is a fallback
// pointer, not the record of the install.
func recordDesktopExeIfPresent(source string) {
	if source == "" {
		return
	}
	sibling := filepath.Join(filepath.Dir(source), "gadak-desktop.exe")
	fi, err := os.Stat(sibling)
	if err != nil || fi.IsDir() {
		return
	}
	abs, err := filepath.Abs(sibling)
	if err != nil {
		return
	}
	dest := desktopExeRecordPath()
	if dest == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(dest, []byte(abs+"\n"), 0o644)
}

// RecordedDesktopExe returns the path stored in DesktopExePathFile when that
// path names an existing non-directory file. Missing or stale records are
// ignored (empty string).
func RecordedDesktopExe() string {
	rec := desktopExeRecordPath()
	if rec == "" {
		return ""
	}
	raw, err := os.ReadFile(rec)
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(raw))
	if p == "" {
		return ""
	}
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return ""
	}
	return p
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "permission denied") || strings.Contains(s, "operation not permitted")
}

func dirExists(dir string) bool {
	fi, err := os.Stat(dir)
	return err == nil && fi.IsDir()
}

// sameRegularFile reports whether a and b are regular files with identical
// contents. Used so a Windows re-install of the same bytes is StatusLinked
// instead of a conflict.
func sameRegularFile(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if !ia.Mode().IsRegular() || !ib.Mode().IsRegular() {
		return false, nil
	}
	if ia.Size() != ib.Size() {
		return false, nil
	}
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(da, db), nil
}

// installCopy writes dest as a copy of src via temp+rename so a partial
// write never leaves a corrupt dest. Snapshot's copyFile is a 0600
// fixture helper in another package — wrong mode and wrong dependency
// direction — so this stays here.
func installCopy(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), "gadak-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// isWritableDir reports whether dir exists and this process can create a file
// in it. Used to avoid picking root-owned /usr/local/bin (no sudo prompts).
func isWritableDir(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".gadak-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
