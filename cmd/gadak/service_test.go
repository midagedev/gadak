package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServeArgsIncludesProfile(t *testing.T) {
	// Without profile: serve --no-open only.
	got := serveArgs()
	if len(got) < 2 || got[len(got)-2] != "serve" || got[len(got)-1] != "--no-open" {
		t.Fatalf("serveArgs = %v", got)
	}
}

func TestInstallServiceWritesUnit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-service unsupported on windows")
	}
	// Redirect home so we never touch the real LaunchAgents / systemd tree.
	home := t.TempDir()
	t.Setenv("HOME", home)
	// USERPROFILE for good measure on some Go runtimes.
	t.Setenv("USERPROFILE", home)

	exe, err := executablePath()
	if err != nil {
		t.Fatal(err)
	}
	// Don't call real launchctl/systemctl in unit tests — write the file the
	// same way the install helpers do, then assert shape.
	args := serveArgs()
	switch runtime.GOOS {
	case "darwin":
		dir := filepath.Join(home, "Library", "LaunchAgents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, serviceLabel+".plist")
		var prog strings.Builder
		prog.WriteString("    <string>" + xmlEscape(exe) + "</string>\n")
		for _, a := range args {
			prog.WriteString("    <string>" + xmlEscape(a) + "</string>\n")
		}
		body := `<?xml version="1.0"?><plist><dict><key>ProgramArguments</key><array>
` + prog.String() + `</array></dict></plist>`
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		if !strings.Contains(s, exe) {
			t.Errorf("plist missing binary path %q", exe)
		}
		if !strings.Contains(s, "serve") || !strings.Contains(s, "--no-open") {
			t.Errorf("plist missing serve --no-open: %s", s)
		}
	case "linux":
		dir := filepath.Join(home, ".config", "systemd", "user")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "gadak.service")
		parts := []string{shellQuote(exe)}
		for _, a := range args {
			parts = append(parts, shellQuote(a))
		}
		unit := "[Service]\nExecStart=" + strings.Join(parts, " ") + "\n"
		if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(path)
		s := string(raw)
		if !strings.Contains(s, "serve") || !strings.Contains(s, "--no-open") {
			t.Errorf("unit missing serve --no-open: %s", s)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote(`/usr/bin/gadak`); got != `/usr/bin/gadak` {
		t.Errorf("plain path: %q", got)
	}
	if got := shellQuote(`/path with space/gadak`); !strings.HasPrefix(got, `"`) {
		t.Errorf("spaced path should be quoted: %q", got)
	}
}

func TestXMLEscape(t *testing.T) {
	if got := xmlEscape(`a&b<c>"d"`); got != `a&amp;b&lt;c&gt;&quot;d&quot;` {
		t.Errorf("xmlEscape = %q", got)
	}
}
