package main

import (
	"os"
	"strings"
	"testing"
)

// desktopPlatform is one row of the platform contract: what this module
// packs, and how a first-launch gadak:// URL reaches the window.
//
// Two owners already exist and this table must not become a third:
//
//   - pack scripts on disk are the ship-shape owner
//     (build-app.sh / build-linux.sh / build-windows.ps1)
//   - coldStartDecisionFor in main.go is the deep-link delivery owner (GDK-293)
//
// The table exists so a reader (and this file) can answer both questions in
// one place, and so comments can point at it / at those owners instead of
// restating either. EventOnURLArg / ArgvOnURLArg are the one-argument
// gadak:// shape the CLI launcher uses (len(args)==2, args[1] contains "://").
type desktopPlatform struct {
	GOOS          string
	PackScript    string
	EventOnURLArg bool
	ArgvOnURLArg  bool
}

var desktopPlatforms = []desktopPlatform{
	{GOOS: "darwin", PackScript: "build-app.sh", EventOnURLArg: true, ArgvOnURLArg: false},
	{GOOS: "windows", PackScript: "build-windows.ps1", EventOnURLArg: true, ArgvOnURLArg: false},
	// linux EventOnURLArg was false under v3.0.0-beta.9 (GTK4 did not emit).
	// wailsapp/wails#6000 landed in beta.10; this module pins v3.0.0-beta.12.
	{GOOS: "linux", PackScript: "build-linux.sh", EventOnURLArg: true, ArgvOnURLArg: false},
}

func TestDesktopPackScriptsExist(t *testing.T) {
	for _, p := range desktopPlatforms {
		if _, err := os.Stat(p.PackScript); err != nil {
			t.Errorf("pack script for GOOS %s missing: %s: %v", p.GOOS, p.PackScript, err)
		}
	}
}

func TestDesktopPlatformsMatchColdStartOwner(t *testing.T) {
	urlArgs := []string{"gadak-desktop", "gadak://view?issue=NMB-1"}
	for _, p := range desktopPlatforms {
		got := coldStartDecisionFor(p.GOOS, urlArgs)
		if got.DeferToEvent != p.EventOnURLArg {
			t.Errorf("coldStartDecisionFor(%s, urlArg).DeferToEvent = %v, table EventOnURLArg = %v",
				p.GOOS, got.DeferToEvent, p.EventOnURLArg)
		}
		if got.ApplyArgv != p.ArgvOnURLArg {
			t.Errorf("coldStartDecisionFor(%s, urlArg).ApplyArgv = %v, table ArgvOnURLArg = %v",
				p.GOOS, got.ApplyArgv, p.ArgvOnURLArg)
		}
	}
}

// pinnedWailsVersion is the require line in this module's go.mod. A bump
// that forgets to re-check the comments that name the pin fails
// TestPinnedWailsVersionIsNamed, which is the point: those comments are
// beliefs about upstream, and they rot the way GDK-296 already closed.
func pinnedWailsVersion(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "github.com/wailsapp/wails/v3 "
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			return fields[1]
		}
	}
	t.Fatal("go.mod has no github.com/wailsapp/wails/v3 require")
	return ""
}

func TestPinnedWailsVersionIsNamed(t *testing.T) {
	ver := pinnedWailsVersion(t)
	// Every tracked file in desktop/ that states a belief about upstream
	// behaviour. A bump that left build-windows.ps1 naming an older pin
	// (beta.6 after the beta.9 bump) is the rot GDK-296 closed, so it is
	// in the list. Current pin is v3.0.0-beta.12 (#6000 inboard since beta.10).
	for _, path := range []string{"main.go", "README.md", "build-windows.ps1"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), ver) {
			t.Errorf("%s does not name the pinned wails version %q — re-read the pin's launch-event and lock behaviour before landing a bump", path, ver)
		}
	}
}

// replacedWailsVersion is the version go.mod redirects the wails module to,
// or "" when there is no replace. Distinct from pinnedWailsVersion: the
// require line names the upstream base even while a fork carries the build.
func replacedWailsVersion(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "replace github.com/wailsapp/wails/v3 "
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			return fields[4]
		}
		t.Fatalf("go.mod wails replace is not a module@version form: %q", line)
	}
	return ""
}

func TestWailsModuleVersionMatchesGoMod(t *testing.T) {
	want := pinnedWailsVersion(t)
	// 2026-08-30 (GDK-1024 fork policy, docs/runbooks/upstream-pr.md): a
	// `replace` onto a fork makes the built binary's version legitimately
	// differ from the require line, and wailsModuleVersion() prefers
	// Replace.Version by design — it reports what actually linked. So the
	// assertion narrows rather than relaxes: the fork must be a patch ON
	// the pin (prefix), and the binary must be the fork, not the base.
	// FAIL-first: this test went red on the commit that added the replace,
	// "wailsModuleVersion() = \"v3.0.0-beta.12-gadak.1\", go.mod has
	// \"v3.0.0-beta.12\"" — the run is on PR #78, job "Desktop tests".
	if fork := replacedWailsVersion(t); fork != "" {
		if !strings.HasPrefix(fork, want) {
			t.Fatalf("go.mod replaces wails with %q, which is not a patch on the pinned %q — a fork cut from a different upstream base is a silent version bump", fork, want)
		}
		want = fork
	}
	if got := wailsModuleVersion(); got != want {
		t.Fatalf("wailsModuleVersion() = %q, go.mod has %q", got, want)
	}
}

func TestDesktopREADMENamesPlatformOwners(t *testing.T) {
	body, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	needles := []string{
		"coldStartDecisionFor",
		"ApplicationLaunchedWithUrl",
	}
	for _, p := range desktopPlatforms {
		needles = append(needles, p.PackScript)
	}
	for _, n := range needles {
		if !strings.Contains(text, n) {
			t.Errorf("desktop/README.md does not mention %q (platform table / owners)", n)
		}
	}
}

// Phrases that were true before 0.16 and are false now. A comment that
// restates any of these reintroduces a bug that was already fixed (GDK-293
// was caused by a comment asserting the wrong platform split).
func TestDesktopCommentsDoNotRestateStalePlatformFacts(t *testing.T) {
	cases := []struct {
		path    string
		phrases []string
	}{
		{"deeplink.go", []string{"on other platforms the events", "simply never fire"}},
		{"embed_other.go", []string{"The desktop app ships for macOS only today"}},
		{"fatal_windows.go", []string{"wails has no dialog helper"}},
		{"README.md", []string{"does not set an `ErrorHandler`", "No workspace switcher"}},
		{"build-windows.ps1", []string{"sets no ErrorHandler"}},
		{"../docs/DESKTOP.md", []string{"Intel Macs and Linux use the"}},
		{"../cmd/gadak/views.go", []string{"which is macOS-only today"}},
	}
	for _, tc := range cases {
		body, err := os.ReadFile(tc.path)
		if err != nil {
			t.Errorf("%s: %v", tc.path, err)
			continue
		}
		text := string(body)
		for _, p := range tc.phrases {
			if strings.Contains(text, p) {
				t.Errorf("%s still contains stale platform claim %q", tc.path, p)
			}
		}
	}
}
