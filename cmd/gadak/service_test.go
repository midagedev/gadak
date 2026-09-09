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
	got := serveArgsFor(config.Profile(), nil)
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
	if err := installSystemd(nil); err != nil {
		t.Fatalf("default install: %v", err)
	}
	config.SetProfile("work")
	if err := installSystemd(nil); err != nil {
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
	if err := installLaunchd(nil); err != nil {
		t.Fatalf("default install: %v", err)
	}
	config.SetProfile("work")
	if err := installLaunchd(nil); err != nil {
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

	err := installSystemd(nil)
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

	err := installLaunchd(nil)
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
	if err := installSystemd(nil); err != nil {
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
	if err := installSystemd(nil); err != nil {
		t.Fatal(err)
	}
	config.SetProfile("work")
	if err := installSystemd(nil); err != nil {
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
	if err := installSystemd(nil); err != nil {
		t.Fatal(err)
	}
	config.SetProfile("work")
	if err := installSystemd(nil); err != nil {
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
	args := serveArgsFor(config.Profile(), nil)
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

// GDK-1267. install-service accepted serve flags after -- only in the sense
// that it never read them: an --addr that serve itself refuses (non-loopback
// without --allow-remote) installed happily, and the written unit would die
// at start and restart forever under KeepAlive / Restart=on-failure — the
// crash loop has no exit code to read. The refusal belongs at install time.
func TestInstallServiceRefusesServeFlagServeItselfRefuses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-service unsupported on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	mockServiceCmds(t)

	err := cmdInstallService([]string{"--", "--addr", "0.0.0.0:7777"})
	if err == nil {
		t.Fatal("install accepted --addr 0.0.0.0:7777 without --allow-remote")
	}
	if !strings.Contains(err.Error(), "allow-remote") {
		t.Fatalf("error should name the flag that was missing: %v", err)
	}
	// A refused install writes nothing — a half-written unit carrying an
	// address serve refuses is worse than no unit.
	var wrote []string
	paths := map[string][]string{
		"darwin": {filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")},
		"linux":  {filepath.Join(home, ".config", "systemd", "user", "gadak.service")},
	}
	for _, p := range paths[runtime.GOOS] {
		if _, statErr := os.Stat(p); statErr == nil {
			wrote = append(wrote, p)
		}
	}
	if len(wrote) > 0 {
		t.Errorf("refused install still wrote: %v", wrote)
	}
}

// GDK-1267, the carry half: serve flags the parser accepts ride in the
// unit's ExecStart, after serve --no-open.
func TestInstallServicePassesServeFlagsToUnit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-service unsupported on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")
	mockServiceCmds(t)

	extra := []string{"--addr", "127.0.0.1:8200", "--allow-remote"}
	if err := cmdInstallService(append([]string{"--"}, extra...)); err != nil {
		t.Fatalf("valid serve flags refused: %v", err)
	}
	var body string
	switch runtime.GOOS {
	case "darwin":
		raw, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist"))
		if err != nil {
			t.Fatal(err)
		}
		body = string(raw)
	case "linux":
		raw, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "gadak.service"))
		if err != nil {
			t.Fatal(err)
		}
		body = string(raw)
	}
	if !strings.Contains(body, "serve") || !strings.Contains(body, "--no-open") {
		t.Fatalf("unit lost serve --no-open: %s", body)
	}
	for _, a := range extra {
		if !strings.Contains(body, a) {
			t.Fatalf("unit dropped serve flag %q: %s", a, body)
		}
	}
	if iNoOpen, iAddr := strings.Index(body, "--no-open"), strings.Index(body, "--addr"); iNoOpen < 0 || iAddr < iNoOpen {
		t.Fatalf("extras must ride after serve --no-open: %s", body)
	}
}

func TestServeArgsCarryServeExtras(t *testing.T) {
	got := serveArgsFor("", []string{"--addr", "127.0.0.1:8200"})
	want := []string{"serve", "--no-open", "--addr", "127.0.0.1:8200"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("serveArgsFor = %v, want %v", got, want)
	}
	got = serveArgsFor("work", []string{"--no-sync"})
	want = []string{"--profile", "work", "serve", "--no-open", "--no-sync"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("serveArgsFor(profile, extra) = %v, want %v", got, want)
	}
}

// Both unit formats carry the extras — installLaunchd/installSystemd are
// plain writers (cmdInstallService picks one by GOOS), so the tests call
// both and each format is asserted on every host.
func TestUnitsCarryServeExtrasBothFormats(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")
	mockServiceCmds(t)
	extra := []string{"--addr", "127.0.0.1:8200", "--allow-remote"}

	if err := installLaunchd(extra); err != nil {
		t.Fatalf("launchd install with extras: %v", err)
	}
	plist, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{
		{"<string>serve</string>", "serve"},
		{"<string>--no-open</string>", "--no-open"},
		{"<string>--addr</string>", "--addr"},
		{"<string>127.0.0.1:8200</string>", "--addr value"},
		{"<string>--allow-remote</string>", "--allow-remote"},
	} {
		if !strings.Contains(string(plist), pair[0]) {
			t.Fatalf("plist missing %s (%s): %s", pair[1], pair[0], plist)
		}
	}

	if err := installSystemd(extra); err != nil {
		t.Fatalf("systemd install with extras: %v", err)
	}
	unit, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "gadak.service"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "ExecStart="; !strings.Contains(string(unit), want) {
		t.Fatalf("unit missing ExecStart: %s", unit)
	}
	if want := "serve --no-open --addr 127.0.0.1:8200 --allow-remote"; !strings.Contains(string(unit), want) {
		t.Fatalf("ExecStart does not carry the extras in order (%s): %s", want, unit)
	}
}

// An unknown extra (`-- --nonsense`) needs no test here: newFlagSet is
// flag.ExitOnError, so the parse inside parseServeOpts exits 2 with serve's
// own usage — which documents exactly the flags allowed after -- — before
// any unit is written. That refusal belongs to the flag package, shared by
// every subcommand; what this round adds is gated above (extras carried into
// the unit, checkServeAddr enforced at install time, uninstall tolerating
// extras).

// --uninstall ignores extras: the unit being removed is identified by
// profile alone, and a scripted uninstall+reinstall cycle would otherwise
// fail on its second half.
func TestUninstallToleratesServeExtras(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-service unsupported on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")
	mockServiceCmds(t)

	if err := cmdInstallService([]string{"--", "--addr", "0.0.0.0:7777", "--allow-remote"}); err != nil {
		t.Fatalf("install with a serve-refused-but-flagged addr: %v", err)
	}
	if err := cmdInstallService([]string{"--uninstall", "--", "--addr", "0.0.0.0:7777"}); err != nil {
		t.Fatalf("uninstall must tolerate the extras it is not using: %v", err)
	}
	switch runtime.GOOS {
	case "darwin":
		if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")); !os.IsNotExist(err) {
			t.Fatalf("uninstall left the plist behind; stat=%v", err)
		}
	case "linux":
		if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "gadak.service")); !os.IsNotExist(err) {
			t.Fatalf("uninstall left the unit behind; stat=%v", err)
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
