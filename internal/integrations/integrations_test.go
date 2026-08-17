package integrations

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListOrderAndDetectFlip(t *testing.T) {
	home := t.TempDir()
	gadakHome := filepath.Join(home, ".gadak")
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", gadakHome)

	items := List()
	if len(items) != 3 {
		t.Fatalf("len=%d want 3", len(items))
	}
	if items[0].ID != IDRaycast || items[1].ID != IDSkill || items[2].ID != IDMCPClaude {
		t.Fatalf("order %q %q %q", items[0].ID, items[1].ID, items[2].ID)
	}
	if items[0].Installed == nil || *items[0].Installed {
		t.Fatalf("raycast want false, got %v", items[0].Installed)
	}
	if items[1].Installed == nil || *items[1].Installed {
		t.Fatalf("skill want false, got %v", items[1].Installed)
	}
	if items[1].Prerequisite != nil {
		t.Fatalf("skill prerequisite=%v want nil", items[1].Prerequisite)
	}
	if items[0].Detail != "~/.gadak/raycast-extension" {
		t.Fatalf("raycast detail=%q", items[0].Detail)
	}
	if items[1].Detail != "~/.claude/skills/gadak/SKILL.md" {
		t.Fatalf("skill detail=%q", items[1].Detail)
	}

	if err := os.MkdirAll(filepath.Join(gadakHome, "raycast-extension"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gadakHome, "raycast-extension", "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("# gadak\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	items = List()
	if items[0].Installed == nil || !*items[0].Installed {
		t.Fatalf("raycast after touch: %v", items[0].Installed)
	}
	if items[1].Installed == nil || !*items[1].Installed {
		t.Fatalf("skill after touch: %v", items[1].Installed)
	}

	// package.json without node_modules is an interrupted install: still
	// installed (Update re-runs the verb) but the detail says so.
	if !strings.Contains(items[0].Detail, "node_modules missing") {
		t.Fatalf("raycast detail should flag missing node_modules, got %q", items[0].Detail)
	}
	if err := os.MkdirAll(filepath.Join(gadakHome, "raycast-extension", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	items = List()
	if strings.Contains(items[0].Detail, "incomplete") {
		t.Fatalf("raycast detail should be clean with node_modules present, got %q", items[0].Detail)
	}
}

func TestInstallArgs(t *testing.T) {
	args, ok := InstallArgs(IDSkill)
	if !ok || strings.Join(args, " ") != "skill install claude" {
		t.Fatalf("skill args=%v ok=%v", args, ok)
	}
	if _, ok := InstallArgs("nope"); ok {
		t.Fatal("unknown id must be false")
	}
}

func TestItemJSONKeepsNulls(t *testing.T) {
	b, err := json.Marshal(Item{ID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"installed":null`) {
		t.Fatalf("installed omitted: %s", s)
	}
	if !strings.Contains(s, `"prerequisite":null`) {
		t.Fatalf("prerequisite omitted: %s", s)
	}
}

func TestResolveNPMOrder(t *testing.T) {
	look := func(string) (string, error) { return "", os.ErrNotExist }
	if _, ok := resolveNPM(look, func(string) bool { return false }); ok {
		t.Fatal("no candidate should miss")
	}
	got, ok := resolveNPM(func(string) (string, error) { return "/tmp/path-npm", nil }, func(string) bool { return false })
	if !ok || got != "/tmp/path-npm" {
		t.Fatalf("PATH win: %q ok=%v", got, ok)
	}
	got, ok = resolveNPM(look, func(p string) bool { return p == "/opt/homebrew/bin/npm" })
	if !ok || got != "/opt/homebrew/bin/npm" {
		t.Fatalf("brew fallback: %q ok=%v", got, ok)
	}
	got, ok = resolveNPM(look, func(p string) bool { return p == "/usr/local/bin/npm" })
	if !ok || got != "/usr/local/bin/npm" {
		t.Fatalf("usr/local fallback: %q ok=%v", got, ok)
	}
}

func TestMCPProbeUnknownWhenMissingOrFail(t *testing.T) {
	old := lookPath
	t.Cleanup(func() { lookPath = old })

	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	item := mcpClaudeItem()
	if item.Installed != nil {
		t.Fatalf("missing claude: installed=%v want null", item.Installed)
	}
	if item.Prerequisite == nil || item.Prerequisite.OK {
		t.Fatalf("missing claude: prerequisite=%+v", item.Prerequisite)
	}

	dir := t.TempDir()
	fail := filepath.Join(dir, "claude")
	if err := os.WriteFile(fail, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return fail, nil
		}
		return old(name)
	}
	item = mcpClaudeItem()
	if item.Installed != nil {
		t.Fatalf("failed get: installed=%v want null", item.Installed)
	}
	if item.Prerequisite == nil || !item.Prerequisite.OK {
		t.Fatalf("failed get: prerequisite=%+v", item.Prerequisite)
	}
}

func TestMCPProbeDefinitiveNotRegistered(t *testing.T) {
	// claude's real wording for "not registered" (measured 2026-08-17):
	// exit 1 + "No MCP server named ...". That is a definitive false, not
	// an unknown — the UI should offer Install, not shrug.
	dir := t.TempDir()
	neg := filepath.Join(dir, "claude")
	script := "#!/bin/sh\necho 'No MCP server named \"gadak\". Configured servers: x' \nexit 1\n"
	if err := os.WriteFile(neg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := lookPath
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return neg, nil
		}
		return old(name)
	}
	t.Cleanup(func() { lookPath = old })

	item := mcpClaudeItem()
	if item.Installed == nil || *item.Installed {
		t.Fatalf("not-registered wording: installed=%v want false", item.Installed)
	}
	if !strings.Contains(item.Detail, "not registered") {
		t.Fatalf("detail=%q want not-registered wording", item.Detail)
	}
}

func TestMCPProbeTrueOnExitZero(t *testing.T) {
	dir := t.TempDir()
	okBin := filepath.Join(dir, "claude")
	if err := os.WriteFile(okBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := lookPath
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return okBin, nil
		}
		return old(name)
	}
	t.Cleanup(func() { lookPath = old })

	item := mcpClaudeItem()
	if item.Installed == nil || !*item.Installed {
		t.Fatalf("exit 0: installed=%v want true", item.Installed)
	}
}

func TestMCPProbeTimeoutIsUnknown(t *testing.T) {
	dir := t.TempDir()
	slow := filepath.Join(dir, "claude")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 10\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	got := probeClaudeMCP(slow)
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("probe hung %s", elapsed)
	}
	if got != nil {
		t.Fatalf("timeout should be unknown, got %v", *got)
	}
}

func TestLookPathDefaultIsExecLookPath(t *testing.T) {
	// Sanity: we did not leave lookPath nil.
	if lookPath == nil {
		t.Fatal("lookPath is nil")
	}
	_, _ = exec.LookPath("true")
}
