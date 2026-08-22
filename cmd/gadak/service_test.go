package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func TestServeArgsIncludesProfile(t *testing.T) {
	// Without profile: serve --no-open only.
	got := serveArgsFor(config.Profile())
	if len(got) < 2 || got[len(got)-2] != "serve" || got[len(got)-1] != "--no-open" {
		t.Fatalf("serveArgsFor = %v", got)
	}
}

func TestServiceNamesDefaultUnchanged(t *testing.T) {
	label, plist, unit := serviceNames("")
	if label != serviceLabel || plist != serviceLabel+".plist" || unit != "gadak.service" {
		t.Fatalf("default names = %q %q %q", label, plist, unit)
	}
	label2, plist2, unit2 := serviceNames("default")
	if label2 != label || plist2 != plist || unit2 != unit {
		t.Fatalf("default alias names = %q %q %q", label2, plist2, unit2)
	}
}

func TestServiceNamesNamedDistinct(t *testing.T) {
	label, plist, unit := serviceNames("work")
	if label == serviceLabel || plist == serviceLabel+".plist" || unit == "gadak.service" {
		t.Fatalf("named profile must not reuse the default unit: %q %q %q", label, plist, unit)
	}
	if !strings.Contains(label, "work") || !strings.Contains(plist, "work") || !strings.Contains(unit, "work") {
		t.Fatalf("named unit must include the profile: %q %q %q", label, plist, unit)
	}
	_, _, other := serviceNames("demo")
	if other == unit {
		t.Fatalf("two named profiles share a unit: %q", unit)
	}
}

func mockServiceCmds(t *testing.T) {
	t.Helper()
	saved := runServiceCmd
	t.Cleanup(func() { runServiceCmd = saved })
	runServiceCmd = func(name string, arg ...string) error { return nil }
}

func TestTwoProfilesWriteTwoSystemdUnits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	mockServiceCmds(t)

	config.SetProfile("")
	if err := installSystemd(); err != nil {
		t.Fatalf("default install: %v", err)
	}
	config.SetProfile("work")
	if err := installSystemd(); err != nil {
		t.Fatalf("work install: %v", err)
	}

	def := filepath.Join(home, ".config", "systemd", "user", "gadak.service")
	work := filepath.Join(home, ".config", "systemd", "user", "gadak-work.service")
	if _, err := os.Stat(def); err != nil {
		t.Fatalf("default unit missing: %v", err)
	}
	if _, err := os.Stat(work); err != nil {
		t.Fatalf("work unit missing: %v", err)
	}
	raw, err := os.ReadFile(work)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "--profile") || !strings.Contains(string(raw), "work") {
		t.Fatalf("work unit missing --profile work: %s", raw)
	}
	defRaw, err := os.ReadFile(def)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(defRaw), "--profile") {
		t.Fatalf("default unit should not carry --profile: %s", defRaw)
	}
}

func TestTwoProfilesWriteTwoLaunchdPlists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	mockServiceCmds(t)

	config.SetProfile("")
	if err := installLaunchd(); err != nil {
		t.Fatalf("default install: %v", err)
	}
	config.SetProfile("work")
	if err := installLaunchd(); err != nil {
		t.Fatalf("work install: %v", err)
	}

	def := filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
	work := filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".work.plist")
	if _, err := os.Stat(def); err != nil {
		t.Fatalf("default plist missing: %v", err)
	}
	if _, err := os.Stat(work); err != nil {
		t.Fatalf("work plist missing: %v", err)
	}
}

func TestSystemdEnableFailurePropagates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	saved := runServiceCmd
	t.Cleanup(func() { runServiceCmd = saved })
	runServiceCmd = func(name string, arg ...string) error {
		for i, a := range arg {
			if a == "enable" {
				return fmt.Errorf("Failed to connect to bus")
			}
			_ = i
		}
		return nil
	}

	err := installSystemd()
	if err == nil {
		t.Fatal("systemctl enable --now failure must be a non-zero exit")
	}
	if !strings.Contains(err.Error(), "enable") && !strings.Contains(err.Error(), "bus") {
		t.Fatalf("error should mention enable failure: %v", err)
	}
}

func TestLaunchdLoadFailurePropagates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	saved := runServiceCmd
	t.Cleanup(func() { runServiceCmd = saved })
	runServiceCmd = func(name string, arg ...string) error {
		if len(arg) > 0 && (arg[0] == "load" || arg[0] == "bootstrap") {
			return errors.New("load refused")
		}
		return nil
	}

	err := installLaunchd()
	if err == nil {
		t.Fatal("launchctl load/bootstrap failure must be a non-zero exit")
	}
}

func TestMigrateLegacyDefaultUnitForNamedProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	mockServiceCmds(t)

	// Pre-D4 leftover: the default-named unit actually runs --profile work.
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "gadak.service")
	if err := os.WriteFile(legacy, []byte("[Service]\nExecStart=/bin/gadak --profile work serve --no-open\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	config.SetProfile("work")
	if err := installSystemd(); err != nil {
		t.Fatalf("install work: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gadak-work.service")); err != nil {
		t.Fatalf("new work unit missing: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy default unit owned by work should be removed; stat=%v", err)
	}
}

func TestMigrateLeavesRealDefaultUnit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	mockServiceCmds(t)

	config.SetProfile("")
	if err := installSystemd(); err != nil {
		t.Fatal(err)
	}
	config.SetProfile("work")
	if err := installSystemd(); err != nil {
		t.Fatal(err)
	}
	def := filepath.Join(home, ".config", "systemd", "user", "gadak.service")
	if _, err := os.Stat(def); err != nil {
		t.Fatalf("real default unit must survive a named install: %v", err)
	}
}

func TestUninstallNamedLeavesDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	mockServiceCmds(t)

	config.SetProfile("")
	if err := installSystemd(); err != nil {
		t.Fatal(err)
	}
	config.SetProfile("work")
	if err := installSystemd(); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSystemd(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "gadak-work.service")); !os.IsNotExist(err) {
		t.Fatalf("named unit should be gone; stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "gadak.service")); err != nil {
		t.Fatalf("default unit must remain: %v", err)
	}
}

func TestLegacyDefaultUnitOwnsNamedProfile(t *testing.T) {
	if !legacyUnitOwnsProfile("[Service]\nExecStart=/bin/gadak --profile work serve --no-open\n", "work") {
		t.Fatal("systemd ExecStart with --profile work should match")
	}
	plist := "<string>--profile</string>\n    <string>work</string>"
	if !legacyUnitOwnsProfile(plist, "work") {
		t.Fatal("launchd --profile/work pair should match")
	}
	if legacyUnitOwnsProfile("[Service]\nExecStart=/bin/gadak serve --no-open\n", "work") {
		t.Fatal("default unit must not match a named profile")
	}
	if legacyUnitOwnsProfile("ExecStart=/bin/gadak --profile demo serve --no-open", "work") {
		t.Fatal("demo unit must not match work")
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
	args := serveArgsFor(config.Profile())
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
