package main

// scry mcp install <client> — register scry as an MCP server with the current
// profile baked into the command so hosts that do not inherit shell env cannot
// silently attach to the default mirror.
//
// claude: exec `claude mcp add` (PATH lookup; dry-run prints only).
// cursor / codex / json: print paste-ready config (no exec).

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/midagedev/scry/internal/config"
)

// execLookPath is exec.LookPath; tests inject a failure for the missing-binary path.
var execLookPath = exec.LookPath

// mcpServerArgs is the argv tail for the scry process that hosts MCP:
// optional --profile <name>, then "mcp". Empty profile means default (omit flag).
func mcpServerArgs(profile string) []string {
	var args []string
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	args = append(args, "mcp")
	return args
}

// claudeMCPAddArgv is the argv for `claude` (without the binary name):
// mcp add scry -- <exe> [--profile p] mcp
func claudeMCPAddArgv(exe, profile string) []string {
	return append([]string{"mcp", "add", "scry", "--", exe}, mcpServerArgs(profile)...)
}

// formatClaudeMCPAddCommand is the shell-ready one-liner for dry-run and for
// the manual fallback when claude is not on PATH.
func formatClaudeMCPAddCommand(exe, profile string) string {
	parts := []string{"claude", "mcp", "add", "scry", "--", shellQuote(exe)}
	for _, a := range mcpServerArgs(profile) {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// mcpServersDoc is the shape hosts expect under mcpServers.
type mcpServersDoc struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func mcpServersPayload(exe, profile string) mcpServersDoc {
	return mcpServersDoc{
		MCPServers: map[string]mcpServerEntry{
			"scry": {
				Command: exe,
				Args:    mcpServerArgs(profile),
			},
		},
	}
}

// formatMCPInstallJSON is the mcpServers snippet (trailing newline).
func formatMCPInstallJSON(exe, profile string) string {
	raw, err := json.MarshalIndent(mcpServersPayload(exe, profile), "", "  ")
	if err != nil {
		// Only fails on non-encodable values; strings always encode.
		return fmt.Sprintf("scry: encode mcpServers: %v\n", err)
	}
	return string(raw) + "\n"
}

// formatMCPInstallCursor is a paste block for Cursor's MCP config.
func formatMCPInstallCursor(exe, profile string) string {
	var b strings.Builder
	b.WriteString("# Paste into .cursor/mcp.json (project) or Cursor Settings → MCP:\n\n")
	b.WriteString(formatMCPInstallJSON(exe, profile))
	return b.String()
}

// formatMCPInstallCodex is a paste block for Codex config.toml.
func formatMCPInstallCodex(exe, profile string) string {
	args := mcpServerArgs(profile)
	// TOML array of strings.
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	var b strings.Builder
	b.WriteString("# Paste into ~/.codex/config.toml (or the project Codex config):\n\n")
	b.WriteString("[mcp_servers.scry]\n")
	fmt.Fprintf(&b, "command = %q\n", exe)
	fmt.Fprintf(&b, "args = [%s]\n", strings.Join(quoted, ", "))
	return b.String()
}

func errClaudeNotFound(exe, profile string) error {
	return fmt.Errorf("claude not found on PATH\n\nInstall Claude Code, or run manually:\n  %s",
		formatClaudeMCPAddCommand(exe, profile))
}

// looksAlreadyRegistered is a best-effort match on claude mcp add stderr/stdout
// when the server name is already registered.
func looksAlreadyRegistered(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "already")
}

func printMCPInstallHelp() {
	fmt.Print(`scry mcp install — register scry as an MCP server (current profile pinned)

Usage:
  scry [--profile <name>] mcp install <client> [--dry-run]

Clients:
  claude   run ` + "`claude mcp add`" + ` with this binary and profile baked in
  cursor   print Cursor MCP config to paste (.cursor/mcp.json)
  codex    print Codex MCP config to paste (~/.codex/config.toml)
  json     print mcpServers JSON snippet only

Options:
  --dry-run   print the command (claude) or config without registering

Examples:
  scry mcp install claude
  scry mcp install claude --dry-run
  scry --profile demo mcp install claude
  scry --profile demo mcp install json
  scry mcp install cursor
  scry mcp install codex

See also: scry mcp, scry profiles, docs/MCP.md, docs/AGENT_SETUP.md
`)
}

func cmdMCPInstall(args []string) error {
	dryRun := false
	var positionals []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			printMCPInstallHelp()
			return nil
		case a == "--dry-run" || a == "-dry-run":
			dryRun = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s\nrun \"scry mcp install --help\" for examples", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) == 0 {
		// No client: list supported clients (and usage).
		printMCPInstallHelp()
		return nil
	}
	if len(positionals) != 1 {
		return usageError("mcp install", "usage: scry mcp install <client> [--dry-run]")
	}

	client := strings.ToLower(positionals[0])
	exe, err := executablePath()
	if err != nil {
		return err
	}
	profile := config.Profile()

	switch client {
	case "claude":
		return mcpInstallClaude(exe, profile, dryRun)
	case "cursor":
		fmt.Print(formatMCPInstallCursor(exe, profile))
		return nil
	case "codex":
		fmt.Print(formatMCPInstallCodex(exe, profile))
		return nil
	case "json":
		fmt.Print(formatMCPInstallJSON(exe, profile))
		return nil
	default:
		return fmt.Errorf("unknown client %q — supported: claude, cursor, codex, json\nrun \"scry mcp install --help\" for examples", client)
	}
}

func mcpInstallClaude(exe, profile string, dryRun bool) error {
	line := formatClaudeMCPAddCommand(exe, profile)
	if dryRun {
		fmt.Println(line)
		return nil
	}
	claudePath, err := execLookPath("claude")
	if err != nil {
		return errClaudeNotFound(exe, profile)
	}
	argv := claudeMCPAddArgv(exe, profile)
	cmd := exec.Command(claudePath, argv...)
	// Claude's UX is interactive-ish messaging on both streams; capture both so
	// "already exists" detection and the user see the same text.
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		// Preserve claude's wording; do not reformat.
		fmt.Print(string(out))
		if !strings.HasSuffix(string(out), "\n") {
			fmt.Println()
		}
	}
	if err != nil {
		if looksAlreadyRegistered(string(out)) {
			fmt.Fprintln(os.Stderr, "scry: already registered — nothing to do")
			return nil
		}
		if len(out) == 0 {
			return fmt.Errorf("claude mcp add failed: %w\nmanual: %s", err, line)
		}
		return fmt.Errorf("claude mcp add failed: %w", err)
	}
	return nil
}
