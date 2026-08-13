// Package clitool installs the gadak binary onto PATH via a symlink.
// Shared by `gadak install-cli` and the desktop "Install Command Line Tool…"
// menu so both surfaces use the same resolve / install / PATH-advice logic.
package clitool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Destination status after Resolve.
const (
	StatusMissing  = "missing"  // nothing at Dest
	StatusLinked   = "linked"   // Dest is already a symlink to Source
	StatusConflict = "conflict" // Dest exists but is not the desired link
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
}

// Resolve picks the install directory and inspects Dest.
//
// dirFlag empty → DefaultDir(pathEnv) heuristic. Non-empty → expand/absolutize
// that path and skip the heuristic. pathEnv is the PATH string (usually
// os.Getenv("PATH")); inject a fake value in tests.
func Resolve(source, dirFlag, pathEnv string) (Plan, error) {
	source = filepath.Clean(source)
	dir, err := ResolveDir(dirFlag, pathEnv)
	if err != nil {
		return Plan{}, err
	}
	dest := filepath.Join(dir, "gadak")

	p := Plan{
		Source: source,
		Dir:    dir,
		Dest:   dest,
		OnPath: PathContains(pathEnv, dir),
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

// DefaultDir chooses an install directory when --dir is omitted.
//
// Order (see comment block in the body):
//  1. ~/.local/bin if it exists and is already on PATH
//  2. /usr/local/bin if it is on PATH and writable without sudo
//  3. ~/.local/bin otherwise (create on Install; caller may warn about PATH)
func DefaultDir(pathEnv string) (string, error) {
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

// Install creates Dir if needed and symlinks Dest → Source.
// When Status is linked, Install is a no-op.
// When Status is conflict and force is false, Install returns an error without
// touching the filesystem. force replaces an existing Dest.
func Install(p Plan, force bool) error {
	if p.Status == StatusLinked {
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

	if err := os.Symlink(p.Source, p.Dest); err != nil {
		return permissionHint(fmt.Errorf("symlink %s → %s: %w", TildeHome(p.Dest), TildeHome(p.Source), err), p.Dest)
	}
	return nil
}

// PathExportLine returns a one-liner the user can paste to add dir to PATH
// (clipboard / advisory). shell is typically os.Getenv("SHELL").
func PathExportLine(dir, shell string) string {
	line, _ := PathExportAdvise(dir, shell)
	return line
}

// PathExportAdvise returns a pasteable one-liner and a suggested shell rc path
// for persistence (rc may be empty).
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
	if err == nil {
		return nil
	}
	if !isPermissionError(err) {
		return err
	}
	return fmt.Errorf("%w\npermission denied writing %s — re-run with sudo, or use --dir ~/.local/bin (no sudo)",
		err, TildeHome(path))
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
