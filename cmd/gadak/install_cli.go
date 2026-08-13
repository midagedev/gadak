package main

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/midagedev/gadak/internal/clitool"
)

// cmdInstallCLI puts the running binary on PATH via a symlink.
// Default directory prefers an existing PATH entry (see clitool.DefaultDir).
// Used after a desktop install so agents and scripts can invoke `gadak`
// without typing the app-bundle path.
func cmdInstallCLI(args []string) error {
	fs := newFlagSet("install-cli")
	dirFlag := fs.String("dir", "", "install directory (default: prefer a PATH entry, else ~/.local/bin)")
	force := fs.Bool("force", false, "replace an existing file or symlink at the destination")
	printOnly := fs.Bool("print", false, "print the plan without creating anything")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		return fmt.Errorf("install-cli is not supported on Windows — add the directory containing gadak.exe to your PATH, or copy the binary into a directory already on PATH")
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
// clitool.DefaultDir(PATH). Leading ~ is expanded with os.UserHomeDir.
func resolveInstallCLIDir(flag string) (string, error) {
	return clitool.ResolveDir(flag, os.Getenv("PATH"))
}

// expandHomePath expands a leading ~ or ~/… using os.UserHomeDir.
// Kept for tests that call it by name; implementation lives in clitool.
func expandHomePath(p string) (string, error) {
	return clitool.ExpandHome(p)
}

// installCLI creates (or plans) a symlink dir/gadak → source. pathEnv and shell
// are used only for the post-install PATH advisory. dir is already absolute.
func installCLI(w io.Writer, source, dir string, force, printOnly bool, pathEnv, shell string) error {
	// Pass dir as dirFlag so Resolve skips the heuristic (already resolved).
	p, err := clitool.Resolve(source, dir, pathEnv)
	if err != nil {
		return err
	}

	if printOnly {
		fmt.Fprintf(w, "source:  %s\n", clitool.TildeHome(p.Source))
		fmt.Fprintf(w, "dest:    %s\n", clitool.TildeHome(p.Dest))
		switch p.Status {
		case clitool.StatusMissing:
			fmt.Fprintf(w, "status:  missing (would create symlink)\n")
		case clitool.StatusLinked:
			fmt.Fprintf(w, "status:  already installed (same target)\n")
		case clitool.StatusConflict:
			if p.ExistingKind == "symlink" {
				fmt.Fprintf(w, "status:  symlink → %s (use --force to replace)\n", clitool.TildeHome(p.ExistingTarget))
			} else {
				fmt.Fprintf(w, "status:  %s exists (use --force to replace)\n", p.ExistingKind)
			}
		}
		return nil
	}

	if p.Status == clitool.StatusLinked {
		fmt.Fprintf(w, "already installed: %s → %s\n", clitool.TildeHome(p.Dest), clitool.TildeHome(p.Source))
		advisePATH(w, p.Dir, pathEnv, shell)
		fmt.Fprintf(w, "next: gadak mcp install claude\n")
		return nil
	}

	if err := clitool.Install(p, force); err != nil {
		return err
	}

	fmt.Fprintf(w, "installed: %s → %s\n", clitool.TildeHome(p.Dest), clitool.TildeHome(p.Source))
	advisePATH(w, p.Dir, pathEnv, shell)
	fmt.Fprintf(w, "next: gadak mcp install claude\n")
	return nil
}

// pathContainsDir reports whether dir appears as a PATH entry.
// Kept for tests that call it by name.
func pathContainsDir(pathEnv, dir string) bool {
	return clitool.PathContains(pathEnv, dir)
}

// advisePATH prints whether dir is already on PATH, or a shell-specific one-liner
// to add it.
func advisePATH(w io.Writer, dir, pathEnv, shell string) {
	if clitool.PathContains(pathEnv, dir) {
		fmt.Fprintf(w, "PATH: %s is already on your PATH\n", clitool.TildeHome(dir))
		return
	}
	fmt.Fprintf(w, "warning: %s is not on your PATH\n", clitool.TildeHome(dir))
	line, rc := clitool.PathExportAdvise(dir, shell)
	fmt.Fprintf(w, "  add it with:\n    %s\n", line)
	if rc != "" {
		fmt.Fprintf(w, "  (or append that export to %s and open a new shell)\n", rc)
	}
}

// pathExportLine returns a one-liner to prepend dir to PATH, and a suggested
// shell rc path for persistence (may be empty). Kept for tests.
func pathExportLine(dir, shell string) (line, rc string) {
	return clitool.PathExportAdvise(dir, shell)
}
