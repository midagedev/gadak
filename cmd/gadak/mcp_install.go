package main

// gadak mcp install <client> — register gadak as an MCP server with the current
// profile baked into the command so hosts that do not inherit shell env cannot
// silently attach to the default mirror.
//
// claude: exec `claude mcp add` (PATH lookup; dry-run prints only) — Claude Code.
// claude-desktop: merge the gadak entry into Claude Desktop's config file.
// cursor / codex / json: print paste-ready config (no exec).
// raycast: print form values — Raycast has no config file to paste into.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
)

// execLookPath is exec.LookPath; tests inject a failure for the missing-binary path.
var execLookPath = exec.LookPath

// claudeDesktopConfigPath resolves Claude Desktop's config file. clitool owns
// the path (the desktop Integrations row reads the same one); a package var so
// tests can point it at a throwaway home.
var claudeDesktopConfigPath = clitool.ClaudeDesktopConfigPath

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
  claude         run ` + "`claude mcp add`" + ` for Claude Code with this binary and profile baked in
  claude-desktop merge the gadak entry into Claude Desktop's claude_desktop_config.json
  cursor         print Cursor MCP config to paste (.cursor/mcp.json)
  codex          print Codex MCP config to paste (~/.codex/config.toml)
  raycast        print the values to fill into Raycast's Install New Server form
  json           print mcpServers JSON snippet only

Options:
  --dry-run   print the command (claude), the merged config (claude-desktop),
              or the config to paste without registering

For Claude Code, prefer ` + "`gadak skill install`" + ` — the skill carries the schema
and query patterns MCP tools cannot. claude is Claude Code's MCP registration;
claude-desktop is the path for Claude Desktop, which has no shell to run the
CLI from; cursor, codex and raycast print config to paste.

Examples:
  gadak mcp install claude
  gadak mcp install claude --dry-run
  gadak mcp install claude-desktop
  gadak --workspace demo mcp install claude-desktop --dry-run
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
	case "claude-desktop":
		return mcpInstallClaudeDesktop(exe, profile, dryRun)
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
		return fmt.Errorf("unknown client %q — supported: claude, claude-desktop, cursor, codex, raycast, json\nrun \"gadak mcp install --help\" for examples", client)
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
	cmd := execCommand(claudePath, argv...)
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

// mcpInstallClaudeDesktop registers gadak with Claude Desktop by merging a
// gadak entry into claude_desktop_config.json. The file belongs to Claude
// Desktop: every other top-level key and every other server entry is carried
// over as raw JSON untouched, a file that is not a JSON object is refused
// without a write, and the write itself is atomic.
func mcpInstallClaudeDesktop(exe, profile string, dryRun bool) error {
	path, err := claudeDesktopConfigPath()
	if err != nil {
		return err
	}
	entry := mcpServerEntry{Command: exe, Args: mcpServerArgs(profile)}
	doc, existed, same, err := claudeDesktopMergeDoc(path, entry)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Println(path)
		fmt.Print(string(doc))
		return nil
	}
	if same {
		fmt.Fprintf(os.Stderr, "gadak: already registered in %s — nothing to do\n", path)
		return nil
	}
	if err := writeClaudeDesktopConfig(path, doc); err != nil {
		return err
	}
	verb := "registered"
	if existed {
		verb = "updated"
	}
	fmt.Printf("%s gadak in %s — restart Claude Desktop to load it\n", verb, path)
	return nil
}

// claudeDesktopMergeDoc reads path (a missing file starts from an empty
// object) and returns the document a write would place there, whether a
// gadak entry was already present with different values (existed), and
// whether one with the same command and args is there (same — nothing to do).
func claudeDesktopMergeDoc(path string, entry mcpServerEntry) (doc []byte, existed, same bool, err error) {
	top := map[string]json.RawMessage{}
	raw, readErr := os.ReadFile(path)
	switch {
	case readErr == nil:
		top, err = claudeDesktopParseTop(raw, path)
		if err != nil {
			return nil, false, false, err
		}
	case os.IsNotExist(readErr):
		// No file yet: start from {}.
	default:
		return nil, false, false, fmt.Errorf("read %s: %w", path, readErr)
	}
	servers := map[string]json.RawMessage{}
	if prev, ok := top["mcpServers"]; ok {
		servers, err = claudeDesktopParseServers(prev, path)
		if err != nil {
			return nil, false, false, err
		}
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return nil, false, false, fmt.Errorf("encode gadak entry: %w", err)
	}
	if prev, ok := servers["gadak"]; ok {
		existed = true
		var prevEntry mcpServerEntry
		if json.Unmarshal(prev, &prevEntry) == nil &&
			prevEntry.Command == entry.Command && slices.Equal(prevEntry.Args, entry.Args) {
			same = true
		}
	}
	servers["gadak"] = entryRaw
	serversRaw, err := json.Marshal(servers)
	if err != nil {
		return nil, false, false, fmt.Errorf("encode mcpServers: %w", err)
	}
	top["mcpServers"] = serversRaw
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// No HTML escaping: this is the user's file, and a less-than sign inside
	// some other server's argument must not be rewritten as an escape sequence.
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(top); err != nil {
		return nil, false, false, fmt.Errorf("encode %s: %w", path, err)
	}
	return buf.Bytes(), existed, same, nil
}

// claudeDesktopParseTop parses the top level of claude_desktop_config.json as
// a JSON object. Anything else — a parse error, an array, a scalar, null —
// refuses; the error names the path and says the file was left untouched.
func claudeDesktopParseTop(raw []byte, path string) (map[string]json.RawMessage, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("%s is not a JSON object (%v) — the file was left untouched; fix or remove it by hand and re-run", path, err)
	}
	if top == nil {
		// Literal null unmarshals into a nil map without an error.
		return nil, fmt.Errorf("%s is not a JSON object (null) — the file was left untouched; fix or remove it by hand and re-run", path)
	}
	return top, nil
}

// claudeDesktopParseServers is claudeDesktopParseTop for the mcpServers
// value: present but not an object (an array, a scalar, null) refuses the
// same way.
func claudeDesktopParseServers(raw json.RawMessage, path string) (map[string]json.RawMessage, error) {
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, fmt.Errorf("%s: mcpServers is not an object (%v) — the file was left untouched; fix or remove it by hand and re-run", path, err)
	}
	if servers == nil {
		return nil, fmt.Errorf("%s: mcpServers is not an object (null) — the file was left untouched; fix or remove it by hand and re-run", path)
	}
	return servers, nil
}

// writeClaudeDesktopConfig writes doc to path atomically — a temp file in the
// same directory, 0o600, then rename over the target — so a partial write can
// never leave Claude Desktop without its config. The parent directory is
// created 0o700 when missing.
func writeClaudeDesktopConfig(path string, doc []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".claude-desktop-config-*.tmp")
	if err != nil {
		return fmt.Errorf("stage write to %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if _, err := tmp.Write(doc); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
