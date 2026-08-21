package main

// gadak mcp install <client> — register gadak as an MCP server with the current
// profile baked into the command so hosts that do not inherit shell env cannot
// silently attach to the default mirror.
//
// claude: exec `claude mcp add` (PATH lookup; dry-run prints only).
// cursor / codex / json: print paste-ready config (no exec).
// raycast: print form values — Raycast has no config file to paste into.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

// execLookPath is exec.LookPath; tests inject a failure for the missing-binary path.
var execLookPath = exec.LookPath

// mcpServerArgs is the argv tail for the gadak process that hosts MCP:
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
// mcp add gadak -- <exe> [--profile p] mcp
func claudeMCPAddArgv(exe, profile string) []string {
	return append([]string{"mcp", "add", "gadak", "--", exe}, mcpServerArgs(profile)...)
}

// formatClaudeMCPAddCommand is the shell-ready one-liner for dry-run and for
// the manual fallback when claude is not on PATH.
func formatClaudeMCPAddCommand(exe, profile string) string {
	parts := []string{"claude", "mcp", "add", "gadak", "--", shellQuote(exe)}
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
			"gadak": {
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
		return fmt.Sprintf("gadak: encode mcpServers: %v\n", err)
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
	b.WriteString("[mcp_servers.gadak]\n")
	fmt.Fprintf(&b, "command = %q\n", exe)
	fmt.Fprintf(&b, "args = [%s]\n", strings.Join(quoted, ", "))
	return b.String()
}

// formatMCPInstallRaycast prints the values to fill into Raycast's
// "Install New Server" form. It deliberately writes no file and prints no
// JSON snippet: Raycast's manual (manual.raycast.com/ai/model-context-protocol,
// checked 2026-08-16) documents no MCP config path or schema, and its settings
// tree holds no MCP file on disk — the form is the only registration path, so
// the most a CLI can do is hand the user the exact field values.
func formatMCPInstallRaycast(exe, profile string) string {
	args := mcpServerArgs(profile)
	// The form takes one Arguments field; quote tokens with spaces so a
	// human can retype them losslessly (shellQuote leaves bare tokens alone).
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	var b strings.Builder
	b.WriteString("Raycast → Manage MCP Servers → Install New Server — fill the form with:\n\n")
	fmt.Fprintf(&b, "  Name:       gadak\n")
	fmt.Fprintf(&b, "  Transport:  Standard Input/Output (stdio)\n")
	fmt.Fprintf(&b, "  Command:    %s\n", exe)
	fmt.Fprintf(&b, "  Arguments:  %s\n", strings.Join(quoted, " "))
	b.WriteString("\nRaycast registers MCP servers through this form only — there is no\nconfig file to paste into.\n")
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
	fmt.Print(`gadak mcp install — register gadak as an MCP server (current profile pinned)

Usage:
  gadak [--workspace <name>] mcp install <client> [--dry-run]

Clients:
  claude   run ` + "`claude mcp add`" + ` with this binary and profile baked in
  cursor   print Cursor MCP config to paste (.cursor/mcp.json)
  codex    print Codex MCP config to paste (~/.codex/config.toml)
  raycast  print the values to fill into Raycast's Install New Server form
  json     print mcpServers JSON snippet only

Options:
  --dry-run   print the command (claude) or config without registering

Examples:
  gadak mcp install claude
  gadak mcp install claude --dry-run
  gadak --workspace demo mcp install claude
  gadak --workspace demo mcp install json
  gadak mcp install cursor
  gadak mcp install codex
  gadak mcp install raycast

See also: gadak mcp, gadak profiles, docs/MCP.md, docs/AGENT_SETUP.md
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
			return fmt.Errorf("unknown flag %s\nrun \"gadak mcp install --help\" for examples", a)
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
		return usageError("mcp install", "usage: gadak mcp install <client> [--dry-run]")
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
	case "raycast":
		fmt.Print(formatMCPInstallRaycast(exe, profile))
		return nil
	case "json":
		fmt.Print(formatMCPInstallJSON(exe, profile))
		return nil
	default:
		return fmt.Errorf("unknown client %q — supported: claude, cursor, codex, raycast, json\nrun \"gadak mcp install --help\" for examples", client)
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
			fmt.Fprintln(os.Stderr, "gadak: already registered — nothing to do")
			return nil
		}
		if len(out) == 0 {
			return fmt.Errorf("claude mcp add failed: %w\nmanual: %s", err, line)
		}
		return fmt.Errorf("claude mcp add failed: %w", err)
	}
	return nil
}
