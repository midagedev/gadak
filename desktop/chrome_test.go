package main

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowChromeNonDarwinIsNative(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "freebsd", "js"} {
		if got := windowChromeFor(goos); got != windowChromeNative {
			t.Errorf("windowChromeFor(%q) = %q, want %s", goos, got, windowChromeNative)
		}
	}
}

func TestWindowChromeDarwinIsTrafficLightsInset(t *testing.T) {
	if got := windowChromeFor("darwin"); got != windowChromeTrafficLightsInset {
		t.Fatalf("windowChromeFor(darwin) = %q, want %s", got, windowChromeTrafficLightsInset)
	}
}

func TestWindowOptionsFollowChrome(t *testing.T) {
	var native application.WebviewWindowOptions
	applyWindowChrome(&native, windowChromeNative)
	if native.Mac.TitleBar == application.MacTitleBarHiddenInset {
		t.Fatal("native chrome still requested HiddenInset")
	}

	var inset application.WebviewWindowOptions
	applyWindowChrome(&inset, windowChromeTrafficLightsInset)
	if inset.Mac.TitleBar != application.MacTitleBarHiddenInset {
		t.Fatalf("inset chrome TitleBar = %+v, want HiddenInset", inset.Mac.TitleBar)
	}
}

func TestMainWindowOptionsFollowsOwner(t *testing.T) {
	opts := mainWindowOptions()
	wantInset := windowChrome() == windowChromeTrafficLightsInset
	gotInset := opts.Mac.TitleBar == application.MacTitleBarHiddenInset
	if wantInset != gotInset {
		t.Fatalf("owner %s vs TitleBar HiddenInset=%v (GOOS=%s)", windowChrome(), gotInset, runtime.GOOS)
	}
}

func TestWithDesktopFlagCarriesOwnerChrome(t *testing.T) {
	doc, err := withDesktopFlag([]byte(`{"apiBase":"/x/"}`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatal(err)
	}
	if m["desktop"] != true {
		t.Fatalf("desktop = %v", m["desktop"])
	}
	if m["windowChrome"] != windowChrome() {
		t.Fatalf("windowChrome %v != owner %s", m["windowChrome"], windowChrome())
	}
	if _, ok := m["apiBase"]; !ok {
		t.Fatalf("apiBase lost: %s", doc)
	}
}

func TestPrintWindowChromeFlag(t *testing.T) {
	if printWindowChromeIfRequested(nil) {
		t.Fatal("empty args should not print")
	}
	if printWindowChromeIfRequested([]string{"gadak://view"}) {
		t.Fatal("deeplink is not the chrome query")
	}
	if !printWindowChromeIfRequested([]string{"--print-window-chrome"}) {
		t.Fatal("flag not recognized")
	}
}
