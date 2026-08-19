package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	raycastext "github.com/midagedev/gadak/contrib/raycast"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
)

func TestRaycastEmbedPackageJSON(t *testing.T) {
	raw, err := raycastext.FS.ReadFile("package.json")
	if err != nil {
		t.Fatalf("embed package.json: %v", err)
	}
	var pkg struct {
		Name     string `json:"name"`
		Commands []struct {
			Title string `json:"title"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("package.json: %v", err)
	}
	if pkg.Name != "gadak" {
		t.Errorf("package name %q, want gadak", pkg.Name)
	}
	if len(pkg.Commands) == 0 || pkg.Commands[0].Title != "Search Jira & Confluence" {
		t.Errorf("command title = %+v, want Search Jira & Confluence", pkg.Commands)
	}
}

func TestDeployRaycastExtWritesSampleBytes(t *testing.T) {
	dst := t.TempDir()
	// Leftover managed files are replaced; node_modules is kept.
	if err := os.WriteFile(filepath.Join(dst, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(dst, "node_modules", "kept")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "x"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := deployRaycastExt(dst, raycastext.FS); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	want, err := raycastext.FS.ReadFile("package.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("package.json bytes differ from embed (%d vs %d)", len(got), len(want))
	}
	if _, err := os.Stat(filepath.Join(dst, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale.txt should have been removed; stat=%v", err)
	}
	kept, err := os.ReadFile(filepath.Join(mod, "x"))
	if err != nil {
		t.Fatalf("node_modules should be preserved: %v", err)
	}
	if string(kept) != "keep" {
		t.Errorf("node_modules/kept/x = %q", kept)
	}
}

func TestResolveNPMLookupOrder(t *testing.T) {
	var order []string
	look := func(name string) (string, error) {
		order = append(order, "path:"+name)
		return "", errors.New("not on PATH")
	}
	present := func(p string) bool {
		order = append(order, p)
		return false
	}
	if got, ok := clitool.ResolveNPM(look, present); ok {
		t.Fatalf("expected miss, got %q", got)
	}
	want := append([]string{"path:npm"}, clitool.NPMFallbackPaths...)
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, want %v", order, want)
	}
}

func TestResolveNPMPathWins(t *testing.T) {
	look := func(name string) (string, error) {
		if name != "npm" {
			t.Errorf("lookPath(%q)", name)
		}
		return "/custom/bin/npm", nil
	}
	present := func(p string) bool {
		t.Errorf("fallback should not run after PATH hit; present(%s)", p)
		return false
	}
	got, ok := clitool.ResolveNPM(look, present)
	if !ok || got != "/custom/bin/npm" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveNPMSecondFallback(t *testing.T) {
	look := func(string) (string, error) { return "", errors.New("no") }
	present := func(p string) bool { return p == "/usr/local/bin/npm" }
	got, ok := clitool.ResolveNPM(look, present)
	if !ok || got != "/usr/local/bin/npm" {
		t.Fatalf("got %q ok=%v, want /usr/local/bin/npm", got, ok)
	}
}

func TestWatchDevelopLinesFindsMarker(t *testing.T) {
	r := strings.NewReader("preparing workspace\nready  - built extension successfully\nwatching\n")
	got := watchDevelopLines(r, time.Second)
	if !got.Found {
		t.Fatalf("want found, got %+v", got)
	}
	if got.TimedOut {
		t.Fatal("should not time out when the marker is present")
	}
	if !strings.Contains(got.Output, developSuccessMarker) {
		t.Errorf("output missing marker:\n%s", got.Output)
	}
}

func TestWatchDevelopLinesTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
	})
	got := watchDevelopLines(pr, 20*time.Millisecond)
	if !got.TimedOut {
		t.Fatalf("want timeout branch, got %+v", got)
	}
	if got.Found {
		t.Fatal("timeout stream must not count as success")
	}
}

func TestRaycastExtDirHonorsGADAKHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("work")

	got, err := clitool.RaycastExtDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, clitool.RaycastExtDirName)
	if got != want {
		t.Errorf("RaycastExtDir() = %q, want %q (profile must not nest it)", got, want)
	}
}

func TestRaycastIsProfileIndependent(t *testing.T) {
	if !profileIndependent["raycast"] {
		t.Fatal("raycast must be profileIndependent")
	}
	if profileCreateOK["raycast"] {
		t.Fatal("raycast must not mint a profile directory")
	}
}

func TestCmdRaycastUsage(t *testing.T) {
	if err := cmdRaycast([]string{"uninstall"}); err == nil || !strings.Contains(err.Error(), "usage: gadak raycast install") {
		t.Fatalf("unsupported subcommand: %v", err)
	}
	if err := cmdRaycastInstall([]string{"extra"}); err == nil || !strings.Contains(err.Error(), "usage: gadak raycast install") {
		t.Fatalf("extra args: %v", err)
	}
}

func TestNPMMissingMessageListsTried(t *testing.T) {
	msg := npmMissingMessage()
	detail := clitool.NPMNotFoundDetail()
	if !strings.Contains(msg, detail) {
		t.Errorf("npm-missing text must include %q; got:\n%s", detail, msg)
	}
	for _, p := range clitool.NPMFallbackPaths {
		if !strings.Contains(msg, p) {
			t.Errorf("npm-missing text must name tried path %s; got:\n%s", p, msg)
		}
	}
}
