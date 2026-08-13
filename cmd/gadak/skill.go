package main

// gadak skill install [client] — install the embedded Claude Code skill so
// agents that prefer skills over MCP get schema/query knowledge without a
// server process. Source is go:embed'd (gadak.SkillMarkdown); brew installs
// work without a repo checkout.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/clitool"
)

func printSkillHelp() {
	fmt.Print(`gadak skill — install the Claude Code skill (schema + query patterns)

Usage:
  gadak skill install [client] [--project] [--dir <path>] [--print] [--force]

Clients:
  claude   Claude Code skill (default). Installs SKILL.md under skills/gadak/

Other agents (cursor, codex, …) are not supported by this command yet —
use gadak mcp install <client>, or copy skills/gadak/SKILL.md from the repo
yourself.

Options:
  --project   install into ./.claude/skills/gadak/ (current working directory)
  --dir PATH  install into PATH/gadak/SKILL.md (overrides default and --project)
  --print     print the install plan without writing
  --force     overwrite when the existing file differs from the embedded skill

Install locations (when --dir is omitted):
  default     ~/.claude/skills/gadak/SKILL.md
  --project   .claude/skills/gadak/SKILL.md under the current directory

Examples:
  gadak skill install
  gadak skill install claude
  gadak skill install --print
  gadak skill install --project
  gadak skill install --dir /tmp/skills-preview --print
  gadak skill install --force

See also: gadak mcp install, docs/AGENT_SETUP.md, docs/MCP.md
`)
}

func cmdSkill(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		printSkillHelp()
		return nil
	}
	if args[0] == "install" {
		return cmdSkillInstall(args[1:])
	}
	return usageError("skill", "usage: gadak skill install [client] [--project] [--dir <path>] [--print] [--force]")
}

func cmdSkillInstall(args []string) error {
	project := false
	printOnly := false
	force := false
	dirFlag := ""
	var positionals []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printSkillHelp()
			return nil
		case a == "--project":
			project = true
		case a == "--print":
			printOnly = true
		case a == "--force":
			force = true
		case a == "--dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--dir requires a path\nrun \"gadak skill install --help\" for examples")
			}
			i++
			dirFlag = args[i]
		case strings.HasPrefix(a, "--dir="):
			dirFlag = strings.TrimPrefix(a, "--dir=")
			if dirFlag == "" {
				return fmt.Errorf("--dir requires a path\nrun \"gadak skill install --help\" for examples")
			}
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s\nrun \"gadak skill install --help\" for examples", a)
		default:
			positionals = append(positionals, a)
		}
	}

	client := "claude"
	if len(positionals) > 1 {
		return usageError("skill install", "usage: gadak skill install [client] [--project] [--dir <path>] [--print] [--force]")
	}
	if len(positionals) == 1 {
		client = strings.ToLower(positionals[0])
	}
	if client != "claude" {
		return fmt.Errorf("unknown client %q — only Claude Code skills are supported (client: claude)\n"+
			"for other agents use `gadak mcp install <client>`, or copy skills/gadak/SKILL.md yourself\n"+
			"run \"gadak skill install --help\" for examples", client)
	}

	dest, err := resolveSkillDest(project, dirFlag)
	if err != nil {
		return err
	}
	return installSkill(os.Stdout, gadak.SkillMarkdown(), dest, force, printOnly)
}

// resolveSkillDest picks the SKILL.md path.
// --dir wins (dir/gadak/SKILL.md); else --project → cwd/.claude/skills/gadak/;
// else ~/.claude/skills/gadak/SKILL.md.
func resolveSkillDest(project bool, dirFlag string) (string, error) {
	if dirFlag != "" {
		expanded, err := clitool.ExpandHome(dirFlag)
		if err != nil {
			return "", err
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", err
		}
		return filepath.Join(abs, "gadak", "SKILL.md"), nil
	}
	if project {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("current directory: %w", err)
		}
		return filepath.Join(cwd, ".claude", "skills", "gadak", "SKILL.md"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md"), nil
}

// installSkill writes content to dest (or plans with printOnly). Byte-identical
// existing file → already installed (exit 0). Differing content without force → error.
func installSkill(w io.Writer, content []byte, dest string, force, printOnly bool) error {
	if len(content) == 0 {
		return fmt.Errorf("embedded skill is empty — this binary was built without skills/gadak/SKILL.md")
	}

	status, existing, err := skillDestStatus(dest, content)
	if err != nil {
		return err
	}

	if printOnly {
		fmt.Fprintf(w, "source:  embedded skills/gadak/SKILL.md\n")
		fmt.Fprintf(w, "dest:    %s\n", clitool.TildeHome(dest))
		switch status {
		case "missing":
			fmt.Fprintf(w, "status:  missing (would install)\n")
		case "identical":
			fmt.Fprintf(w, "status:  already installed (identical)\n")
		case "differs":
			fmt.Fprintf(w, "status:  differs (use --force to overwrite)\n")
		}
		return nil
	}

	switch status {
	case "identical":
		fmt.Fprintf(w, "already installed: %s\n", clitool.TildeHome(dest))
		fmt.Fprintf(w, "next: restart the agent or open a new session so it picks up the skill\n")
		return nil
	case "differs":
		if !force {
			return fmt.Errorf("%s exists and differs from the embedded skill — re-run with --force to overwrite (your edits will be lost)",
				clitool.TildeHome(dest))
		}
	case "missing":
		// install below
	default:
		return fmt.Errorf("internal: unknown skill dest status %q", status)
	}
	_ = existing // only used for status classification

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", clitool.TildeHome(dir), err)
	}
	// Write via temp + rename so a partial write never leaves a corrupt skill.
	tmp, err := os.CreateTemp(dir, "SKILL.md.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", clitool.TildeHome(dir), err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we fail before rename.
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", clitool.TildeHome(dest), err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod %s: %w", clitool.TildeHome(tmpName), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install %s: %w", clitool.TildeHome(dest), err)
	}

	fmt.Fprintf(w, "installed: %s\n", clitool.TildeHome(dest))
	fmt.Fprintf(w, "next: restart the agent or open a new session so it picks up the skill\n")
	return nil
}

// skillDestStatus classifies dest relative to content.
// Returns status "missing" | "identical" | "differs", and existing bytes when present.
func skillDestStatus(dest string, content []byte) (status string, existing []byte, err error) {
	fi, err := os.Lstat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", nil, nil
		}
		return "", nil, fmt.Errorf("inspect %s: %w", clitool.TildeHome(dest), err)
	}
	if fi.IsDir() {
		return "", nil, fmt.Errorf("%s is a directory — remove it or choose another --dir", clitool.TildeHome(dest))
	}
	existing, err = os.ReadFile(dest)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", clitool.TildeHome(dest), err)
	}
	if bytes.Equal(existing, content) {
		return "identical", existing, nil
	}
	return "differs", existing, nil
}
