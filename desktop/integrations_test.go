package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
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
	// CODEX_HOME is a user preference the destination table honours, so pin it
	// inside the throwaway home: a developer who has it set must not turn
	// their own ~/.codex into a row in this test.
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))

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
		return doc.Items
	}
	idsOf := func(items []map[string]any) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i], _ = it["id"].(string)
		}
		return out
	}

	// An empty home has no skill host on it, so no host row is offered — the
	// Raycast rule (GDK-1513). ~/.agents is the exception: the shared root
	// every agentskills.io host reads is worth offering before any particular
	// host is installed.
	items := get()
	want := []string{"command-line-tool", "raycast", "skill-agents", "mcp-claude", "mcp-claude-desktop"}
	wantCmd := map[string]string{
		"command-line-tool":  "gadak install-cli",
		"raycast":            "gadak raycast install",
		"skill-agents":       "gadak skill install agents",
		"skill-claude":       "gadak skill install claude",
		"mcp-claude":         "gadak mcp install claude",
		"mcp-claude-desktop": "gadak mcp install claude-desktop",
	}
	if runtime.GOOS == "windows" {
		want = []string{"command-line-tool", "skill-agents", "mcp-claude", "mcp-claude-desktop"}
	}
	if got := idsOf(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("ids=%v want %v", got, want)
	}
	byID := func(items []map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, it := range items {
			id, _ := it["id"].(string)
			out[id] = it
		}
		return out
	}
	rows := byID(items)
	for _, id := range want {
		if rows[id]["command"] != wantCmd[id] {
			t.Fatalf("%s command=%v want %s", id, rows[id]["command"], wantCmd[id])
		}
	}
	if runtime.GOOS != "windows" {
		if rows["raycast"]["installed"] != false {
			t.Fatalf("raycast installed=%v want false", rows["raycast"]["installed"])
		}
		if rows["raycast"]["detail"] != "~/.gadak/raycast-extension" {
			t.Fatalf("raycast detail=%v", rows["raycast"]["detail"])
		}
		// Only skill rows carry a status word.
		if _, ok := rows["raycast"]["status"]; ok {
			t.Fatalf("raycast must carry no status: %v", rows["raycast"])
		}
	} else if _, ok := rows["raycast"]; ok {
		t.Fatal("windows GET must not include raycast")
	}
	if rows["skill-agents"]["installed"] != false {
		t.Fatalf("skill-agents installed=%v want false", rows["skill-agents"]["installed"])
	}
	if rows["skill-agents"]["status"] != "missing" {
		t.Fatalf("skill-agents status=%v want missing", rows["skill-agents"]["status"])
	}
	if rows["skill-agents"]["prerequisite"] != nil {
		t.Fatalf("skill prerequisite=%v want null", rows["skill-agents"]["prerequisite"])
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
	// Not gadak's bytes and no receipt beside them: this is the user's own
	// file, which the app must report as a conflict rather than as an install
	// it can quietly overwrite (GDK-1514).
	if err := os.WriteFile(skill, []byte("# gadak\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	items = get()
	rows = byID(items)
	wantAfter := []string{"command-line-tool", "raycast", "skill-claude", "skill-agents", "mcp-claude", "mcp-claude-desktop"}
	if runtime.GOOS == "windows" {
		wantAfter = []string{"command-line-tool", "skill-claude", "skill-agents", "mcp-claude", "mcp-claude-desktop"}
	}
	if got := idsOf(items); !reflect.DeepEqual(got, wantAfter) {
		t.Fatalf("after touch: ids=%v want %v", got, wantAfter)
	}
	if runtime.GOOS != "windows" {
		if rows["raycast"]["installed"] != true {
			t.Fatalf("raycast after touch: installed=%v want true", rows["raycast"]["installed"])
		}
	}
	if rows["skill-claude"]["installed"] != true {
		t.Fatalf("skill-claude after touch: installed=%v want true", rows["skill-claude"]["installed"])
	}
	if rows["skill-claude"]["status"] != "conflict" {
		t.Fatalf("skill-claude after touch: status=%v want conflict", rows["skill-claude"]["status"])
	}
	if rows["skill-claude"]["command"] != wantCmd["skill-claude"] {
		t.Fatalf("skill-claude command=%v", rows["skill-claude"]["command"])
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

// One route per host, and the pre-GDK-1513 id still answers. The argv is the
// single owner of what an install does — the app implements nothing itself.
func TestIntegrationsPOSTPerHostArgv(t *testing.T) {
	script := filepath.Join(t.TempDir(), "gadak")
	body := "#!/bin/sh\necho \"args: $*\"\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GADAK_DESKTOP_CLI", script)

	for id, want := range map[string]string{
		"skill":              "args: skill install claude", // legacy alias
		"skill-claude":       "args: skill install claude",
		"skill-codex":        "args: skill install codex",
		"skill-agents":       "args: skill install agents",
		"skill-cursor":       "args: skill install cursor",
		"skill-gemini":       "args: skill install gemini",
		"skill-opencode":     "args: skill install opencode",
		"skill-grok":         "args: skill install grok",
		"mcp-claude":         "args: mcp install claude",
		"mcp-claude-desktop": "args: mcp install claude-desktop",
	} {
		rec := httptest.NewRecorder()
		integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/"+id+"/install", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: POST %d %s", id, rec.Code, rec.Body.String())
		}
		got := rec.Body.String()
		if !strings.Contains(got, want) {
			t.Fatalf("%s: want %q in:\n%s", id, want, got)
		}
		// Never --project: the app has no working directory to mean.
		if strings.Contains(got, "--project") {
			t.Fatalf("%s: argv must not carry --project:\n%s", id, got)
		}
	}

	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill-nosuchhost/install", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown host: %d want 404 %s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationsPOSTUnknownID(t *testing.T) {
	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/nope/install", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d want 404 body %s", rec.Code, rec.Body.String())
	}
}

// TestIntegrationsPOSTForceQuery — GDK-1535. The second tap of the armed
// confirm posts ?force=1 and the skill row's argv grows exactly one --force;
// every other row's argv is untouched by the flag, so a stray force on the
// wire cannot turn another install into an overwrite.
func TestIntegrationsPOSTForceQuery(t *testing.T) {
	script := filepath.Join(t.TempDir(), "gadak")
	body := "#!/bin/sh\necho \"args: $*\"\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GADAK_DESKTOP_CLI", script)

	for id, want := range map[string]string{
		"skill-claude": "args: skill install claude --force",
		"skill":        "args: skill install claude --force", // legacy alias
		"skill-codex":  "args: skill install codex --force",
		// Non-skill rows stay flagless under ?force=1.
		"mcp-claude":         "args: mcp install claude",
		"command-line-tool":  "args: install-cli",
		"mcp-claude-desktop": "args: mcp install claude-desktop",
	} {
		rec := httptest.NewRecorder()
		integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
			"/desktop/integrations/"+id+"/install?force=1", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: POST %d %s", id, rec.Code, rec.Body.String())
		}
		got := rec.Body.String()
		if !strings.Contains(got, want) {
			t.Fatalf("%s: want %q in:\n%s", id, want, got)
		}
	}

	// And without the query the skill argv is the plain verb — the flag is
	// opt-in per request, never sticky.
	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill-claude/install", nil))
	if got := rec.Body.String(); !strings.Contains(got, "args: skill install claude\n") || strings.Contains(got, "--force") {
		t.Fatalf("plain POST must stay flagless:\n%s", got)
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
	returned := make(chan struct{})
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
		first <- rec
		close(returned)
	}()

	// GDK-1502: installGate is package-global, so whatever fails below, this
	// test must not leave its install wedged — a leaked in-flight "skill" is
	// invisible here and turns the next test's POST into a 409. Releasing the
	// fixture and draining the request is ownership of the shared gate, not
	// best-effort tidying: t.Cleanup runs on every exit, including the
	// timeouts that used to be the leak. `returned` (not `first`) is what
	// cleanup waits on: the body below may already have drained the value.
	t.Cleanup(func() {
		_ = os.WriteFile(release, []byte("go\n"), 0o644)
		select {
		case <-returned:
			// The handler returned, so its deferred endInstall ran too. If
			// the key is still held the gate itself leaks — say so here,
			// where the culprit is, not in a sibling's 409.
			if held := activeInstalls(); len(held) != 0 {
				t.Errorf("cleanup: install returned but the gate still holds %v", held)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("cleanup: install request never returned; gate holds %v", activeInstalls())
		}
	})

	select {
	case rec := <-first:
		t.Fatalf("first install returned too soon: %d %s", rec.Code, rec.Body.String())
	case <-time.After(50 * time.Millisecond):
	}

	// Poll to a generous upper bound (GDK-1502): this wait covers a process
	// spawn on a loaded machine. The old 3s was not an upper bound, it was a
	// flake budget — the very timeout meant to detect a stuck spawn instead
	// wedged the gate for good. Exceeding it now falls into cleanup.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first install never created %s; gate holds %v", started, activeInstalls())
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

// activeInstalls copies the package-global install gate's keys: the
// one-line answer to "is an install wedged?" that GDK-1502's under-load
// failures had to be inferred from a sibling test's 409.
func activeInstalls() []string {
	installGate.mu.Lock()
	defer installGate.mu.Unlock()
	out := make([]string, 0, len(installGate.active))
	for k := range installGate.active {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestIntegrationsPOSTMissingCLI(t *testing.T) {
	t.Setenv("GADAK_DESKTOP_CLI", filepath.Join(t.TempDir(), "no-such-gadak"))
	// GDK-1502: this test's old under-load failure was a 409 inherited from a
	// sibling's leaked install, not anything about a missing CLI. Fail on the
	// inheritance itself, naming the key that leaked.
	if held := activeInstalls(); len(held) != 0 {
		t.Fatalf("gate already holds %v before this test ran", held)
	}
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
