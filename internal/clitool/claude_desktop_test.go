package clitool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The three GOOS branches of ClaudeDesktopConfigPathFor, pinned on any host
// (darwin CI would otherwise never run the windows and linux shapes).
func TestClaudeDesktopConfigPathForGOOSBranches(t *testing.T) {
	got, err := ClaudeDesktopConfigPathFor("darwin", "/Users/alice", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/Users/alice", "Library", "Application Support", "Claude", "claude_desktop_config.json"); got != want {
		t.Fatalf("darwin: got %q want %q", got, want)
	}

	got, err = ClaudeDesktopConfigPathFor("windows", `C:\Users\alice`, `C:\Users\alice\AppData\Roaming`, "")
	if err != nil {
		t.Fatal(err)
	}
	// Separator comes from filepath.Join, so assert on the joined shape rather
	// than a hardcoded backslash (this test also runs on unix hosts).
	if want := filepath.Join(`C:\Users\alice\AppData\Roaming`, "Claude", "claude_desktop_config.json"); got != want {
		t.Fatalf("windows: got %q want %q", got, want)
	}
	if !strings.HasSuffix(got, filepath.Join("Claude", "claude_desktop_config.json")) {
		t.Fatalf("windows: unexpected tail %q", got)
	}

	got, err = ClaudeDesktopConfigPathFor("linux", "/home/alice", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/home/alice/.config/Claude/claude_desktop_config.json"; got != want {
		t.Fatalf("linux without XDG: got %q want %q", got, want)
	}
	got, err = ClaudeDesktopConfigPathFor("linux", "/home/alice", "", "/home/alice/.config")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/home/alice/.config/Claude/claude_desktop_config.json"; got != want {
		t.Fatalf("linux with XDG: got %q want %q", got, want)
	}
	got, err = ClaudeDesktopConfigPathFor("linux", "/home/alice", "", "/home/alice/xdg-conf")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/home/alice/xdg-conf/Claude/claude_desktop_config.json"; got != want {
		t.Fatalf("XDG wins over home: got %q want %q", got, want)
	}
}

// Each branch that needs a lookup refuses (with the variable named) when the
// lookup is empty — an empty string silently joining into "/Claude/…" would
// point the writer at the filesystem root.
func TestClaudeDesktopConfigPathForRefusesEmptyLookups(t *testing.T) {
	if _, err := ClaudeDesktopConfigPathFor("darwin", "", "", ""); err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("darwin without home: err=%v", err)
	}
	if _, err := ClaudeDesktopConfigPathFor("windows", "h", "", ""); err == nil || !strings.Contains(err.Error(), "APPDATA") {
		t.Fatalf("windows without APPDATA: err=%v", err)
	}
	if _, err := ClaudeDesktopConfigPathFor("linux", "", "", ""); err == nil || !strings.Contains(err.Error(), "XDG_CONFIG_HOME") {
		t.Fatalf("linux without home or XDG: err=%v", err)
	}
}

// The zero-arg wrapper must agree with the For function fed the same
// environment — the wrapper is what production calls.
func TestClaudeDesktopConfigPathMatchesFor(t *testing.T) {
	got, err := ClaudeDesktopConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want, err := ClaudeDesktopConfigPathFor(runtime.GOOS, home, os.Getenv("APPDATA"), os.Getenv("XDG_CONFIG_HOME"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("wrapper=%q For=%q", got, want)
	}
}
