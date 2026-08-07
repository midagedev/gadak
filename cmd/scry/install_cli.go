package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// cmdInstallCLI puts the running binary on PATH via a symlink (default
// ~/.local/bin/scry). Used after a desktop install so agents and scripts can
// invoke `scry` without typing the app-bundle path.
func cmdInstallCLI(args []string) error {
	fs := newFlagSet("install-cli")
	dirFlag := fs.String("dir", "", "install directory (default: ~/.local/bin)")
	force := fs.Bool("force", false, "replace an existing file or symlink at the destination")
	printOnly := fs.Bool("print", false, "print the plan without creating anything")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		return fmt.Errorf("install-cli is not supported on Windows — add the directory containing scry.exe to your PATH, or copy the binary into a directory already on PATH")
	}

	source, err := executablePath()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	dir, err := resolveInstallCLIDir(*dirFlag)
	if err != nil {
		return err
	}

	return installCLI(os.Stdout, source, dir, *force, *printOnly, os.Getenv("PATH"), os.Getenv("SHELL"))
}

// resolveInstallCLIDir returns the absolute install directory. Empty flag →
// ~/.local/bin. Leading ~ is expanded with os.UserHomeDir.
func resolveInstallCLIDir(flag string) (string, error) {
	if flag == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home directory: %w", err)
		}
		return filepath.Join(home, ".local", "bin"), nil
	}
	expanded, err := expandHomePath(flag)
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

// expandHomePath expands a leading ~ or ~/… using os.UserHomeDir. Other paths
// are returned cleaned as-is.
func expandHomePath(p string) (string, error) {
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

// installCLI creates (or plans) a symlink dir/scry → source. pathEnv and shell
// are used only for the post-install PATH advisory.
func installCLI(w io.Writer, source, dir string, force, printOnly bool, pathEnv, shell string) error {
	source = filepath.Clean(source)
	dir = filepath.Clean(dir)
	dest := filepath.Join(dir, "scry")

	// Inspect what already sits at dest (if anything).
	existingKind, existingTarget, existErr := inspectInstallDest(dest)
	exists := existErr == nil || !os.IsNotExist(existErr)
	if existErr != nil && !os.IsNotExist(existErr) {
		// e.g. permission denied on Lstat
		return fmt.Errorf("inspect %s: %w", tildeHome(dest), existErr)
	}

	sameTarget := existingKind == "symlink" && existingTarget == source

	if printOnly {
		fmt.Fprintf(w, "source:  %s\n", tildeHome(source))
		fmt.Fprintf(w, "dest:    %s\n", tildeHome(dest))
		switch {
		case !exists:
			fmt.Fprintf(w, "status:  missing (would create symlink)\n")
		case sameTarget:
			fmt.Fprintf(w, "status:  already installed (same target)\n")
		case existingKind == "symlink":
			fmt.Fprintf(w, "status:  symlink → %s (use --force to replace)\n", tildeHome(existingTarget))
		default:
			fmt.Fprintf(w, "status:  %s exists (use --force to replace)\n", existingKind)
		}
		return nil
	}

	if sameTarget {
		fmt.Fprintf(w, "already installed: %s → %s\n", tildeHome(dest), tildeHome(source))
		advisePATH(w, dir, pathEnv, shell)
		fmt.Fprintf(w, "next: scry mcp install claude\n")
		return nil
	}

	if exists && !force {
		switch existingKind {
		case "symlink":
			return fmt.Errorf("%s already exists and points to %s (not %s); re-run with --force to replace",
				tildeHome(dest), tildeHome(existingTarget), tildeHome(source))
		default:
			return fmt.Errorf("%s already exists (%s); re-run with --force to replace",
				tildeHome(dest), existingKind)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return permissionHint(fmt.Errorf("create directory %s: %w", tildeHome(dir), err), dir)
	}

	if exists {
		if err := os.Remove(dest); err != nil {
			return permissionHint(fmt.Errorf("remove %s: %w", tildeHome(dest), err), dest)
		}
	}

	if err := os.Symlink(source, dest); err != nil {
		return permissionHint(fmt.Errorf("symlink %s → %s: %w", tildeHome(dest), tildeHome(source), err), dest)
	}

	fmt.Fprintf(w, "installed: %s → %s\n", tildeHome(dest), tildeHome(source))
	advisePATH(w, dir, pathEnv, shell)
	fmt.Fprintf(w, "next: scry mcp install claude\n")
	return nil
}

// inspectInstallDest classifies dest: "symlink" (with resolved link target as
// stored — not EvalSymlinks of the ultimate file), "file", "directory", or
// "other". On not-exist, returns ("", "", err) with os.IsNotExist.
func inspectInstallDest(dest string) (kind, linkTarget string, err error) {
	fi, err := os.Lstat(dest)
	if err != nil {
		return "", "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, rerr := os.Readlink(dest)
		if rerr != nil {
			return "symlink", "", rerr
		}
		// Normalize for comparison: absolute if stored absolute, else join with dest dir.
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

// permissionHint appends sudo / --dir guidance when the error looks like EACCES.
func permissionHint(err error, path string) error {
	if err == nil {
		return nil
	}
	if !isPermissionError(err) {
		return err
	}
	return fmt.Errorf("%w\npermission denied writing %s — re-run with sudo, or use --dir ~/.local/bin (no sudo)",
		err, tildeHome(path))
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	// Symlink/MkdirAll sometimes wrap the errno; string match is a last resort.
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "permission denied") || strings.Contains(s, "operation not permitted")
}

// advisePATH prints whether dir is already on PATH, or a shell-specific one-liner
// to add it.
func advisePATH(w io.Writer, dir, pathEnv, shell string) {
	if pathContainsDir(pathEnv, dir) {
		fmt.Fprintf(w, "PATH: %s is already on your PATH\n", tildeHome(dir))
		return
	}
	fmt.Fprintf(w, "warning: %s is not on your PATH\n", tildeHome(dir))
	line, rc := pathExportLine(dir, shell)
	fmt.Fprintf(w, "  add it with:\n    %s\n", line)
	if rc != "" {
		fmt.Fprintf(w, "  (or append that export to %s and open a new shell)\n", rc)
	}
}

// pathContainsDir reports whether dir appears as a PATH entry (exact, after
// Clean). Empty entries are ignored.
func pathContainsDir(pathEnv, dir string) bool {
	dir = filepath.Clean(dir)
	for _, p := range filepath.SplitList(pathEnv) {
		if p == "" {
			continue
		}
		// Expand ~ in PATH entries rarely, but Clean is enough for absolute paths.
		if filepath.Clean(p) == dir {
			return true
		}
		// Also accept $HOME/.local/bin style when dir is under home: compare
		// expanded forms if p starts with ~.
		if expanded, err := expandHomePath(p); err == nil && filepath.Clean(expanded) == dir {
			return true
		}
	}
	return false
}

// pathExportLine returns a one-liner to prepend dir to PATH, and a suggested
// shell rc path for persistence (may be empty).
func pathExportLine(dir, shell string) (line, rc string) {
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
		fishDir := display
		return fmt.Sprintf(`fish_add_path %s`, fishDir), "~/.config/fish/config.fish"
	default:
		// zsh is the macOS default and common on developer Linux; use it when unknown.
		return fmt.Sprintf(`echo '%s' >> ~/.zshrc`, export), "~/.zshrc"
	}
}
