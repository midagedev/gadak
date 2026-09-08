package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/clitool"
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

func TestFormatMCPInstallRaycast(t *testing.T) {
	const exe = "/usr/local/bin/gadak"
	got := formatMCPInstallRaycast(exe, "")
	// Where to open: Raycast registers MCP servers through a form, not a file.
	if !strings.Contains(got, "Manage MCP Servers") || !strings.Contains(got, "Install New Server") {
		t.Fatalf("missing form path:\n%s", got)
	}
	// Transport must be named: picking the wrong transport breaks everything.
	if !strings.Contains(got, "Standard Input/Output") {
		t.Fatalf("missing stdio transport:\n%s", got)
	}
	if !strings.Contains(got, exe) {
		t.Fatalf("missing command:\n%s", got)
	}
	if !strings.Contains(got, "mcp") {
		t.Fatalf("missing mcp argument:\n%s", got)
	}
	// Default profile must stay out of the form values.
	if strings.Contains(got, "--profile") {
		t.Fatalf("default profile leaked:\n%s", got)
	}
	// Raycast has no config file to paste into — never a JSON snippet.
	if strings.Contains(got, "mcpServers") {
		t.Fatalf("must not print an mcpServers snippet:\n%s", got)
	}

	got = formatMCPInstallRaycast(exe, "demo")
	if !strings.Contains(got, "--profile demo") {
		t.Fatalf("missing profile args:\n%s", got)
	}
	if !strings.Contains(got, exe) {
		t.Fatalf("missing command with profile:\n%s", got)
	}
	// A profile with a space must be quoted so it survives the form field.
	got = formatMCPInstallRaycast(exe, "my work")
	if !strings.Contains(got, `"my work"`) {
		t.Fatalf("spaced profile not quoted:\n%s", got)
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

// Contract↔assertion table for `gadak mcp install claude-desktop` (spec
// clauses A.1–A.7). Every clause has a happy-path and a violation/boundary
// assertion:
//
//	A.1 path per GOOS (3 branches) → TestClaudeDesktopPathPerGOOS (all three
//	   branches end-to-end); shapes pinned in
//	   internal/clitool TestClaudeDesktopConfigPathForGOOSBranches
//	A.1 entry shape == json client → TestClaudeDesktopEntryMatchesJSONClient
//	A.2 missing file → {} + dir    → TestClaudeDesktopInstallCreatesFileAndDir
//	A.3 refuse non-object top      → TestClaudeDesktopRefusesNonObjectFile
//	A.4 mcpServers not object      → TestClaudeDesktopRefusesBadMCPServers
//	A.4 other keys/servers kept    → TestClaudeDesktopPreservesOtherKeysAndServers
//	A.5 same command+args → no-op  → TestClaudeDesktopAlreadyRegistered
//	A.6 write: verb line, update,  → TestClaudeDesktopInstallUpdatesEntry
//	   0o600, indent, newline        and TestClaudeDesktopInstallCreatesFileAndDir
//	A.7 dry-run: path + JSON only  → TestClaudeDesktopDryRun

// pointClaudeDesktopAt injects a config path for the given GOOS branch under a
// throwaway home and returns it. darwin CI would otherwise never run the
// windows and linux shapes.
func pointClaudeDesktopAt(t *testing.T, goos string) string {
	t.Helper()
	home := t.TempDir()
	path, err := clitool.ClaudeDesktopConfigPathFor(goos, home,
		filepath.Join(home, "AppData", "Roaming"), filepath.Join(home, ".config"))
	if err != nil {
		t.Fatal(err)
	}
	old := claudeDesktopConfigPath
	claudeDesktopConfigPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { claudeDesktopConfigPath = old })
	return path
}

// A.1: the three GOOS branches each resolve (and write) through the injected
// path — the darwin host running this test must not be the only shape that works.
func TestClaudeDesktopPathPerGOOS(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		path := pointClaudeDesktopAt(t, goos)
		if filepath.Base(path) != "claude_desktop_config.json" {
			t.Fatalf("%s: base=%q", goos, filepath.Base(path))
		}
		stdout, _, err := captureBoth(t, func() error {
			return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "", false)
		})
		if err != nil {
			t.Fatalf("%s: install: %v", goos, err)
		}
		if !strings.Contains(stdout, "registered gadak in "+path) {
			t.Fatalf("%s: stdout=%q", goos, stdout)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s: file not written: %v", goos, err)
		}
	}
}

// A.1: a fresh install writes exactly the entry `mcp install json` prints.
func TestClaudeDesktopEntryMatchesJSONClient(t *testing.T) {
	path := pointClaudeDesktopAt(t, "darwin")
	if _, _, err := captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "demo", false)
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got mcpServersDoc
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("written file: %v\n%s", err, raw)
	}
	var want mcpServersDoc
	if err := json.Unmarshal([]byte(formatMCPInstallJSON("/usr/local/bin/gadak", "demo")), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entry:\n got %v\nwant %v", got, want)
	}
}

// A.2: a missing file starts from {} and the parent directory is created.
func TestClaudeDesktopInstallCreatesFileAndDir(t *testing.T) {
	path := pointClaudeDesktopAt(t, "darwin")
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s already exists", dir)
	}
	stdout, _, err := captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "", false)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "registered gadak in "+path+" — restart Claude Desktop to load it") {
		t.Fatalf("stdout=%q", stdout)
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("parent dir: %v", err)
	}
	// Two-space indent and a trailing newline: the file is read by humans too.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "{\n  \"") {
		t.Fatalf("indent:\n%s", raw)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("no trailing newline:\n%q", raw)
	}
}

// A.3: a file that is not a JSON object is refused, named, and left untouched.
func TestClaudeDesktopRefusesNonObjectFile(t *testing.T) {
	for name, content := range map[string]string{
		"truncated": `{"mcpServers": {`,
		"array":     `[]`,
		"scalar":    `"nope"`,
		"null":      `null`,
	} {
		path := pointClaudeDesktopAt(t, "darwin")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := captureBoth(t, func() error {
			return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "", false)
		})
		if err == nil {
			t.Fatalf("%s: expected refusal", name)
		}
		if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "left untouched") {
			t.Fatalf("%s: error must name the path and say untouched: %v", name, err)
		}
		after, rerr := os.ReadFile(path)
		if rerr != nil || string(after) != content {
			t.Fatalf("%s: file changed: %q", name, after)
		}
	}
}

// A.4: mcpServers present but not an object refuses the same way.
func TestClaudeDesktopRefusesBadMCPServers(t *testing.T) {
	for name, content := range map[string]string{
		"array":  "{\"mcpServers\": [{\"gadak\": {}}]}",
		"null":   "{\"mcpServers\": null}",
		"scalar": "{\"mcpServers\": 3}",
	} {
		path := pointClaudeDesktopAt(t, "darwin")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := captureBoth(t, func() error {
			return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "", false)
		})
		if err == nil || !strings.Contains(err.Error(), "mcpServers is not an object") {
			t.Fatalf("%s: error=%v", name, err)
		}
		after, rerr := os.ReadFile(path)
		if rerr != nil || string(after) != content {
			t.Fatalf("%s: file changed: %q", name, after)
		}
	}
}

// A.4: every other top-level key and every other server entry survives a
// merge with its value intact.
func TestClaudeDesktopPreservesOtherKeysAndServers(t *testing.T) {
	path := pointClaudeDesktopAt(t, "darwin")
	seed := `{
  "globalShortcut": "CmdOrCtrl+Shift+G",
  "nested": {"a": [1, 2, {"b": true}]},
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    }
  }
}`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "demo", false)
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		GlobalShortcut string                     `json:"globalShortcut"`
		Nested         map[string]json.RawMessage `json:"nested"`
		MCPServers     map[string]mcpServerEntry  `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("merged file: %v\n%s", err, raw)
	}
	if doc.GlobalShortcut != "CmdOrCtrl+Shift+G" {
		t.Fatalf("globalShortcut=%q", doc.GlobalShortcut)
	}
	if string(doc.Nested["a"]) == "" || !strings.Contains(string(doc.Nested["a"]), "true") {
		t.Fatalf("nested key lost: %v", doc.Nested)
	}
	fs, ok := doc.MCPServers["filesystem"]
	if !ok || fs.Command != "npx" || len(fs.Args) != 3 || fs.Args[2] != "/tmp" {
		t.Fatalf("filesystem entry: %+v ok=%v", fs, ok)
	}
	if g := doc.MCPServers["gadak"]; g.Command != "/usr/local/bin/gadak" || len(g.Args) != 3 {
		t.Fatalf("gadak entry: %+v", g)
	}
}

// A.5: a gadak entry with the same command and args is a no-op — stderr line,
// exit 0, and the file's bytes unchanged.
func TestClaudeDesktopAlreadyRegistered(t *testing.T) {
	path := pointClaudeDesktopAt(t, "darwin")
	if _, _, err := captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "demo", false)
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "demo", false)
	})
	if err != nil {
		t.Fatalf("second install must exit 0: %v", err)
	}
	if !strings.Contains(stderr, "gadak: already registered in "+path+" — nothing to do") {
		t.Fatalf("stderr=%q", stderr)
	}
	if strings.Contains(stdout, "registered") || strings.Contains(stdout, "updated") {
		t.Fatalf("stdout must stay quiet: %q", stdout)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("file rewritten on a no-op")
	}
}

// A.6 + input defense (malicious): a gadak entry pointing at another binary
// is replaced, the verb is "updated", and the file lands 0o600.
func TestClaudeDesktopInstallUpdatesEntry(t *testing.T) {
	path := pointClaudeDesktopAt(t, "darwin")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"mcpServers": {"gadak": {"command": "/tmp/not-gadak", "args": ["mcp"]}}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "", false)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "updated gadak in "+path+" — restart Claude Desktop to load it") {
		t.Fatalf("stdout=%q", stdout)
	}
	if strings.Contains(stdout, "registered gadak") {
		t.Fatalf("verb must be updated: %q", stdout)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc mcpServersDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.MCPServers["gadak"].Command != "/usr/local/bin/gadak" {
		t.Fatalf("stale entry survived: %s", raw)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm=%o want 600", fi.Mode().Perm())
	}
}

// A.7: dry-run prints the path and the exact document, writes no file, and
// creates no directory.
func TestClaudeDesktopDryRun(t *testing.T) {
	path := pointClaudeDesktopAt(t, "darwin")
	stdout, _, err := captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "demo", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(stdout, "\n", 2)
	if lines[0] != path {
		t.Fatalf("first line=%q want %q", lines[0], path)
	}
	var doc mcpServersDoc
	if err := json.Unmarshal([]byte(lines[1]), &doc); err != nil {
		t.Fatalf("dry-run body: %v\n%s", err, lines[1])
	}
	if g := doc.MCPServers["gadak"]; g.Command != "/usr/local/bin/gadak" || len(g.Args) != 3 {
		t.Fatalf("dry-run entry: %+v", g)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote the file")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the directory")
	}

	// Boundary: dry-run over an existing file previews the merge without
	// touching the file.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"globalShortcut\": \"x\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = captureBoth(t, func() error {
		return mcpInstallClaudeDesktop("/usr/local/bin/gadak", "", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.SplitN(stdout, "\n", 2)[1]
	if !strings.Contains(body, "globalShortcut") || !strings.Contains(body, "\"gadak\"") {
		t.Fatalf("dry-run merge preview:\n%s", body)
	}
	after, _ := os.ReadFile(path)
	if string(after) != "{\"globalShortcut\": \"x\"}\n" {
		t.Fatalf("dry-run changed the file: %q", after)
	}
}
