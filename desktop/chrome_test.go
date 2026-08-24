package main

import (
	"encoding/json"
	"errors"
	"runtime"
	"strings"
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

// GDK-786/791: the stamp must round-trip the ui block and configVersion the
// registry's WebConfigBase emits — the /w/<name>/config.json path goes
// through this remarshal, and dropping the block here would make workspace
// tabs the only surface without color overrides. FAIL-first: withDesktopFlag
// predates the ui field, so this pins the structural-preservation property.
func TestWithDesktopFlagCarriesUIBlock(t *testing.T) {
	in := []byte(`{"apiBase":"/w/work/api/v1/","configVersion":"123.456",` +
		`"ui":{"vars":{"light":{"--color-accent":"#7a4bd0"}},` +
		`"dataColors":{"label":{"urgent":"#c03030"}}}}`)
	doc, err := withDesktopFlag(in)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Desktop       bool   `json:"desktop"`
		ConfigVersion string `json:"configVersion"`
		UI            struct {
			Vars       map[string]map[string]string `json:"vars"`
			DataColors map[string]map[string]string `json:"dataColors"`
		} `json:"ui"`
	}
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatal(err)
	}
	if !m.Desktop {
		t.Fatal("desktop flag lost")
	}
	if m.ConfigVersion != "123.456" {
		t.Fatalf("configVersion lost: %s", doc)
	}
	if m.UI.Vars["light"]["--color-accent"] != "#7a4bd0" || m.UI.DataColors["label"]["urgent"] != "#c03030" {
		t.Fatalf("ui block lost: %s", doc)
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

func TestPrintIntegrationsFlag(t *testing.T) {
	if printIntegrationsIfRequested(nil) {
		t.Fatal("empty args should not print")
	}
	if printIntegrationsIfRequested([]string{"--print-window-chrome"}) {
		t.Fatal("chrome flag is not the integrations query")
	}
	if printIntegrationsIfRequested([]string{"gadak://view"}) {
		t.Fatal("deeplink is not the integrations query")
	}
	if !printIntegrationsIfRequested([]string{"--print-integrations"}) {
		t.Fatal("flag not recognized")
	}
}

func TestWebView2MissingMessageNamesRuntimeAndInstaller(t *testing.T) {
	msg := webview2UserMessage(errors.New("no webview2 found"))
	if !strings.Contains(msg, "WebView2") {
		t.Fatalf("message must name WebView2:\n%s", msg)
	}
	if !strings.Contains(msg, webview2EvergreenURL) {
		t.Fatalf("message must point at the Evergreen installer:\n%s", msg)
	}
}
