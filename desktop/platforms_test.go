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
	{GOOS: "linux", PackScript: "build-linux.sh", EventOnURLArg: false, ArgvOnURLArg: true},
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
