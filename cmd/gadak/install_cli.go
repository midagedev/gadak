package main

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/midagedev/gadak/internal/clitool"
)

// cmdInstallCLI puts the running binary on PATH (symlink on Unix, copy on
// Windows). Default directory prefers an existing PATH entry (see
// clitool.DefaultDir). Used after a desktop install so agents and scripts
// can invoke `gadak` without typing the app-bundle path.
func cmdInstallCLI(args []string) error {
	fs := newFlagSet("install-cli")
	dirFlag := fs.String("dir", "", "install directory (default: prefer a PATH entry, else ~/.local/bin; %LOCALAPPDATA%\\Programs\\gadak on Windows)")
	force := fs.Bool("force", false, "replace an existing file or symlink at the destination")
	printOnly := fs.Bool("print", false, "print the plan without creating anything")
	if err := fs.Parse(args); err != nil {
		return err
	}

	source, err := executablePath()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	dir, err := resolveInstallCLIDir(*dirFlag)
	if err != nil {
		return err
	}

	return installCLI(os.Stdout, source, dir, *force, *printOnly, os.Getenv("PATH"), os.Getenv("SHELL"), runtime.GOOS)
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

// installCLI creates (or plans) the install. goos selects dest name and
// symlink-vs-copy inside clitool — this function does not branch on it.
// pathEnv and shell are used only for the post-install PATH advisory.
// dir is already absolute.
func installCLI(w io.Writer, source, dir string, force, printOnly bool, pathEnv, shell, goos string) error {
	// Pass dir as dirFlag so Resolve skips the heuristic (already resolved).
	p, err := clitool.ResolveFor(source, dir, pathEnv, goos)
	if err != nil {
		return err
	}

	if printOnly {
		fmt.Fprintf(w, "source:  %s\n", clitool.TildeHome(p.Source))
		fmt.Fprintf(w, "dest:    %s\n", clitool.TildeHome(p.Dest))
		fmt.Fprintln(w, p.PrintStatusLine())
		return nil
	}

	if p.Status == clitool.StatusLinked {
		fmt.Fprintln(w, p.AlreadyInstalledLine())
		advisePATH(w, p.Dir, pathEnv, shell, goos)
		printInstallCLISkillFollowup(w, autoInstallSkill(os.Stderr))
		return nil
	}

	if err := clitool.Install(p, force); err != nil {
		return err
	}

	fmt.Fprintln(w, p.InstalledLine())
	advisePATH(w, p.Dir, pathEnv, shell, goos)
	printInstallCLISkillFollowup(w, autoInstallSkill(os.Stderr))
	return nil
}

// installCLISkillNext is the line printed when auto-install did not run
// because ~/.claude is absent (or failed). Verified: installCLI success
// paths used this exact string before GDK-93.
const installCLISkillNext = "next: gadak skill install   (Claude Code; for shell-less hosts like Claude Desktop use: gadak mcp install claude)\n"

func printInstallCLISkillFollowup(w io.Writer, skill string) {
	switch skill {
	case "installed":
		dest, err := resolveSkillDest(false, "")
		if err != nil {
			fmt.Fprintf(w, "skill: installed\n")
			return
		}
		fmt.Fprintf(w, "skill: installed %s\n", clitool.TildeHome(dest))
	case "skipped":
		if claudeDirExists() {
			// Conflict: --force already went to stderr. Do not suggest the
			// unforced one-liner, which would fail the same way.
			return
		}
		fmt.Fprint(w, installCLISkillNext)
	default:
		fmt.Fprint(w, installCLISkillNext)
	}
}

// pathContainsDir reports whether dir appears as a PATH entry.
// Kept for tests that call it by name.
func pathContainsDir(pathEnv, dir string) bool {
	return clitool.PathContains(pathEnv, dir)
}

// advisePATH prints whether dir is already on PATH, or a shell-specific one-liner
// to add it. goos is forwarded to clitool so Windows is not told to edit ~/.zshrc.
func advisePATH(w io.Writer, dir, pathEnv, shell, goos string) {
	if clitool.PathContains(pathEnv, dir) {
		fmt.Fprintf(w, "PATH: %s is already on your PATH\n", clitool.TildeHome(dir))
		return
	}
	fmt.Fprintf(w, "warning: %s is not on your PATH\n", clitool.TildeHome(dir))
	line, rc := clitool.PathExportAdviseFor(dir, shell, goos)
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
