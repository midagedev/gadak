package main

import (
	"errors"
	"strings"
	"testing"
)

func TestMCPServerArgs(t *testing.T) {
	if got := mcpServerArgs(""); len(got) != 1 || got[0] != "mcp" {
		t.Fatalf("default profile: got %v", got)
	}
	got := mcpServerArgs("demo")
	want := []string{"--profile", "demo", "mcp"}
	if len(got) != len(want) {
		t.Fatalf("demo: got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("demo: got %v want %v", got, want)
		}
	}
}

func TestClaudeMCPAddArgv(t *testing.T) {
	exe := "/usr/local/bin/gadak"
	got := claudeMCPAddArgv(exe, "")
	want := []string{"mcp", "add", "gadak", "--", exe, "mcp"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("default:\n got %v\nwant %v", got, want)
	}
	got = claudeMCPAddArgv(exe, "demo")
	want = []string{"mcp", "add", "gadak", "--", exe, "--profile", "demo", "mcp"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("profile:\n got %v\nwant %v", got, want)
	}
}

func TestFormatClaudeMCPAddCommand(t *testing.T) {
	// Absolute path + no profile: shell-ready one-liner for dry-run.
	got := formatClaudeMCPAddCommand("/opt/gadak", "")
	want := "claude mcp add gadak -- /opt/gadak mcp"
	if got != want {
		t.Fatalf("default:\n got %q\nwant %q", got, want)
	}
	got = formatClaudeMCPAddCommand("/opt/gadak", "work")
	want = "claude mcp add gadak -- /opt/gadak --profile work mcp"
	if got != want {
		t.Fatalf("profile:\n got %q\nwant %q", got, want)
	}
	// Spaces in path must be shell-quoted.
	got = formatClaudeMCPAddCommand("/path with space/gadak", "demo")
	if !strings.Contains(got, `"/path with space/gadak"`) {
		t.Fatalf("spaced path not quoted: %q", got)
	}
	if !strings.HasPrefix(got, "claude mcp add gadak -- ") {
		t.Fatalf("prefix: %q", got)
	}
}

func TestFormatMCPInstallJSON(t *testing.T) {
	const exe = "/usr/local/bin/gadak"
	got := formatMCPInstallJSON(exe, "")
	want := `{
  "mcpServers": {
    "gadak": {
      "command": "/usr/local/bin/gadak",
      "args": [
        "mcp"
      ]
    }
  }
}
`
	if got != want {
		t.Fatalf("default json:\n got %q\nwant %q", got, want)
	}
	got = formatMCPInstallJSON(exe, "demo")
	want = `{
  "mcpServers": {
    "gadak": {
      "command": "/usr/local/bin/gadak",
      "args": [
        "--profile",
        "demo",
        "mcp"
      ]
    }
  }
}
`
	if got != want {
		t.Fatalf("profile json:\n got %q\nwant %q", got, want)
	}
}

func TestFormatMCPInstallCursor(t *testing.T) {
	got := formatMCPInstallCursor("/usr/local/bin/gadak", "demo")
	// Header points at Cursor's MCP config location.
	if !strings.Contains(got, ".cursor/mcp.json") {
		t.Fatalf("missing cursor path hint:\n%s", got)
	}
	// Body is the same mcpServers snippet (absolute path + profile).
	if !strings.Contains(got, `"/usr/local/bin/gadak"`) {
		t.Fatalf("missing absolute command:\n%s", got)
	}
	if !strings.Contains(got, `"--profile"`) || !strings.Contains(got, `"demo"`) {
		t.Fatalf("missing profile args:\n%s", got)
	}
	if !strings.Contains(got, `"mcpServers"`) {
		t.Fatalf("missing mcpServers:\n%s", got)
	}
}

func TestFormatMCPInstallCodex(t *testing.T) {
	got := formatMCPInstallCodex("/usr/local/bin/gadak", "")
	if !strings.Contains(got, "config.toml") && !strings.Contains(got, "~/.codex") {
		t.Fatalf("missing codex path hint:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.gadak]") {
		t.Fatalf("missing toml section:\n%s", got)
	}
	if !strings.Contains(got, `command = "/usr/local/bin/gadak"`) {
		t.Fatalf("missing command:\n%s", got)
	}
	if !strings.Contains(got, `args = ["mcp"]`) {
		t.Fatalf("default args:\n%s", got)
	}

	got = formatMCPInstallCodex("/usr/local/bin/gadak", "demo")
	if !strings.Contains(got, `args = ["--profile", "demo", "mcp"]`) {
		t.Fatalf("profile args:\n%s", got)
	}
}

func TestClaudeNotFoundError(t *testing.T) {
	err := errClaudeNotFound("/opt/gadak", "demo")
	s := err.Error()
	if !strings.Contains(s, "claude not found") {
		t.Fatalf("missing not-found: %q", s)
	}
	if !strings.Contains(s, "claude mcp add gadak -- /opt/gadak --profile demo mcp") {
		t.Fatalf("missing manual command: %q", s)
	}
}

func TestLooksAlreadyRegistered(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"MCP server gadak already exists", true},
		{"already registered", true},
		{"Server already exists in config", true},
		{"added gadak successfully", false},
		{"", false},
	}
	for _, c := range cases {
		if got := looksAlreadyRegistered(c.in); got != c.want {
			t.Errorf("looksAlreadyRegistered(%q) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestMCPInstallClaudeMissingBinary(t *testing.T) {
	// Inject LookPath failure — never touches a real claude binary.
	old := execLookPath
	execLookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}
	t.Cleanup(func() { execLookPath = old })

	err := mcpInstallClaude("/opt/gadak", "demo", false)
	if err == nil {
		t.Fatal("expected error when claude missing")
	}
	if !strings.Contains(err.Error(), "claude not found") {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(err.Error(), "--profile demo") {
		t.Fatalf("manual command should pin profile: %v", err)
	}
}

func TestMCPInstallClaudeDryRun(t *testing.T) {
	// dry-run must not call LookPath or exec.
	old := execLookPath
	execLookPath = func(string) (string, error) {
		t.Fatal("dry-run must not look up claude")
		return "", nil
	}
	t.Cleanup(func() { execLookPath = old })

	if err := mcpInstallClaude("/tmp/gadak-bin", "work", true); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	cmd := formatClaudeMCPAddCommand("/tmp/gadak-bin", "work")
	if cmd != "claude mcp add gadak -- /tmp/gadak-bin --profile work mcp" {
		t.Fatalf("dry-run command: %q", cmd)
	}
}
