package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func integrationsMux() http.Handler {
	return fallbackHandler(http.NotFoundHandler(), nil, nil, nil, newBrowseTabs())
}

func TestIntegrationsGETOrderAndDetect(t *testing.T) {
	home := t.TempDir()
	gadakHome := filepath.Join(home, ".gadak")
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", gadakHome)

	h := integrationsMux()
	get := func() []map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/desktop/integrations", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("content-type %q", ct)
		}
		var doc struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("json: %v\n%s", err, rec.Body.String())
		}
		wantN := 4
		if runtime.GOOS == "windows" {
			wantN = 3
		}
		if len(doc.Items) != wantN {
			t.Fatalf("len(items)=%d want %d: %s", len(doc.Items), wantN, rec.Body.String())
		}
		return doc.Items
	}

	items := get()
	wantIDs := []string{"command-line-tool", "raycast", "skill", "mcp-claude"}
	wantCmd := []string{"gadak install-cli", "gadak raycast install", "gadak skill install claude", "gadak mcp install claude"}
	if runtime.GOOS == "windows" {
		wantIDs = []string{"command-line-tool", "skill", "mcp-claude"}
		wantCmd = []string{"gadak install-cli", "gadak skill install claude", "gadak mcp install claude"}
	}
	for i, id := range wantIDs {
		if items[i]["id"] != id {
			t.Fatalf("items[%d].id=%v want %s", i, items[i]["id"], id)
		}
		if items[i]["command"] != wantCmd[i] {
			t.Fatalf("items[%d].command=%v want %s", i, items[i]["command"], wantCmd[i])
		}
	}
	byID := map[string]map[string]any{}
	for _, it := range items {
		id, _ := it["id"].(string)
		byID[id] = it
	}
	if runtime.GOOS != "windows" {
		if byID["raycast"]["installed"] != false {
			t.Fatalf("raycast installed=%v want false", byID["raycast"]["installed"])
		}
		if byID["raycast"]["detail"] != "~/.gadak/raycast-extension" {
			t.Fatalf("raycast detail=%v", byID["raycast"]["detail"])
		}
	} else if _, ok := byID["raycast"]; ok {
		t.Fatal("windows GET must not include raycast")
	}
	if byID["skill"]["installed"] != false {
		t.Fatalf("skill installed=%v want false", byID["skill"]["installed"])
	}
	if byID["skill"]["prerequisite"] != nil {
		t.Fatalf("skill prerequisite=%v want null", byID["skill"]["prerequisite"])
	}

	if err := os.MkdirAll(filepath.Join(gadakHome, "raycast-extension"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gadakHome, "raycast-extension", "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("# gadak\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	items = get()
	byID = map[string]map[string]any{}
	for _, it := range items {
		id, _ := it["id"].(string)
		byID[id] = it
	}
	if runtime.GOOS != "windows" {
		if byID["raycast"]["installed"] != true {
			t.Fatalf("raycast after touch: installed=%v want true", byID["raycast"]["installed"])
		}
	}
	if byID["skill"]["installed"] != true {
		t.Fatalf("skill after touch: installed=%v want true", byID["skill"]["installed"])
	}
}

func TestDesktopCLICandidatesWindowsSibling(t *testing.T) {
	got := desktopCLICandidates(`C:\bundle`, "windows")
	want := []string{
		filepath.Join(`C:\bundle`, "..", "Resources", "bin", "gadak"),
		filepath.Join(`C:\bundle`, "gadak.exe"),
		filepath.Join(`C:\bundle`, "gadak"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestDesktopCLIOKForWindowsIgnoresUnixMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gadak.exe")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !desktopCLIOKFor(p, "windows") {
		t.Fatal("windows must accept a regular file without a POSIX execute bit")
	}
	if desktopCLIOKFor(p, "darwin") {
		t.Fatal("darwin must still require the execute bit")
	}
}

func TestIntegrationsPOSTStreamsOutput(t *testing.T) {
	script := filepath.Join(t.TempDir(), "gadak")
	body := "#!/bin/sh\necho \"args: $*\"\necho stderr-line >&2\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GADAK_DESKTOP_CLI", script)

	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("content-type %q", ct)
	}
	got := rec.Body.String()
	if !strings.Contains(got, "args: skill install claude") {
		t.Fatalf("missing argv line:\n%s", got)
	}
	if !strings.Contains(got, "stderr-line") {
		t.Fatalf("stderr not streamed:\n%s", got)
	}
	if lastLine(got) != "exit=0" {
		t.Fatalf("last line %q want exit=0\n%s", lastLine(got), got)
	}
}

func TestIntegrationsPOSTUnknownID(t *testing.T) {
	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/nope/install", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d want 404 body %s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationsPOSTRaycastOnWindowsIsUnknown(t *testing.T) {
	if runtime.GOOS != "windows" {
		// The GOOS seam lives in InstallArgsFor; pin it there. This test
		// is the handler-level check for a Windows process.
		t.Skip("handler uses runtime.GOOS; InstallArgsFor is pinned in integrations_test")
	}
	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/raycast/install", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("windows raycast POST: %d want 404 body %s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationsPOSTConflict(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	script := filepath.Join(dir, "gadak")
	body := fmt.Sprintf("#!/bin/sh\ntouch %q\nwhile [ ! -f %q ]; do sleep 0.05; done\necho done\nexit 0\n", started, release)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GADAK_DESKTOP_CLI", script)

	h := integrationsMux()
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
		first <- rec
	}()

	select {
	case rec := <-first:
		t.Fatalf("first install returned too soon: %d %s", rec.Code, rec.Body.String())
	case <-time.After(50 * time.Millisecond):
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first install never created the started file")
		}
		time.Sleep(20 * time.Millisecond)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("concurrent: got %d want 409 body %s", rec2.Code, rec2.Body.String())
	}

	if err := os.WriteFile(release, []byte("go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec1 := <-first
	if rec1.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rec1.Code, rec1.Body.String())
	}
	if lastLine(rec1.Body.String()) != "exit=0" {
		t.Fatalf("first last line %q\n%s", lastLine(rec1.Body.String()), rec1.Body.String())
	}
}

func TestIntegrationsPOSTMissingCLI(t *testing.T) {
	t.Setenv("GADAK_DESKTOP_CLI", filepath.Join(t.TempDir(), "no-such-gadak"))
	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body.String())
	}
	if lastLine(rec.Body.String()) != "exit=127" {
		t.Fatalf("last line %q want exit=127\n%s", lastLine(rec.Body.String()), rec.Body.String())
	}
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}
