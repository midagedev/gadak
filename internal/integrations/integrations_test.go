package integrations

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/clitool"
)

// stubNoClaude makes List/ListFor skip the MCP probe. Catalogue tests
// whose subject is not the probe must call this; TestMCPProbe* install
// their own lookPath. Matches the existing lookPath + t.Cleanup idiom.
func stubNoClaude(t *testing.T) {
	t.Helper()
	prev := lookPath
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return "", os.ErrNotExist
		}
		return prev(name)
	}
	t.Cleanup(func() { lookPath = prev })
}

func TestListOrderAndDetectFlip(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	gadakHome := filepath.Join(home, ".gadak")
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", gadakHome)

	items := List()
	if len(items) != 4 {
		t.Fatalf("len=%d want 4", len(items))
	}
	if items[0].ID != IDCommandLineTool || items[1].ID != IDRaycast || items[2].ID != IDSkill || items[3].ID != IDMCPClaude {
		t.Fatalf("order %q %q %q %q", items[0].ID, items[1].ID, items[2].ID, items[3].ID)
	}
	if items[0].Command != "gadak install-cli" {
		t.Fatalf("cli command=%q", items[0].Command)
	}
	if items[1].Installed == nil || *items[1].Installed {
		t.Fatalf("raycast want false, got %v", items[1].Installed)
	}
	if items[2].Installed == nil || *items[2].Installed {
		t.Fatalf("skill want false, got %v", items[2].Installed)
	}
	if items[2].Prerequisite != nil {
		t.Fatalf("skill prerequisite=%v want nil", items[2].Prerequisite)
	}
	if items[1].Detail != "~/.gadak/raycast-extension" {
		t.Fatalf("raycast detail=%q", items[1].Detail)
	}
	if items[2].Detail != "~/.claude/skills/gadak/SKILL.md" {
		t.Fatalf("skill detail=%q", items[2].Detail)
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
	if items[1].Installed == nil || !*items[1].Installed {
		t.Fatalf("raycast after touch: %v", items[1].Installed)
	}
	if items[2].Installed == nil || !*items[2].Installed {
		t.Fatalf("skill after touch: %v", items[2].Installed)
	}

	// package.json without node_modules is an interrupted install: still
	// installed (Update re-runs the verb) but the detail says so.
	if !strings.Contains(items[1].Detail, "node_modules missing") {
		t.Fatalf("raycast detail should flag missing node_modules, got %q", items[1].Detail)
	}
	if err := os.MkdirAll(filepath.Join(gadakHome, "raycast-extension", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	items = List()
	if strings.Contains(items[1].Detail, "incomplete") {
		t.Fatalf("raycast detail should be clean with node_modules present, got %q", items[1].Detail)
	}
}

func TestInstallArgs(t *testing.T) {
	args, ok := InstallArgs(IDSkill)
	if !ok || strings.Join(args, " ") != "skill install claude" {
		t.Fatalf("skill args=%v ok=%v", args, ok)
	}
	args, ok = InstallArgs(IDCommandLineTool)
	if !ok || len(args) != 1 || args[0] != "install-cli" {
		t.Fatalf("cli args=%v ok=%v want [install-cli]", args, ok)
	}
	if _, ok := InstallArgs("nope"); ok {
		t.Fatal("unknown id must be false")
	}
}

func TestCommandLineToolDetectFlip(t *testing.T) {
	stubNoClaude(t)
	oldExec := fileIsExec
	t.Cleanup(func() {
		fileIsExec = oldExec
	})

	// Empty PATH plus no fallbacks: the machine's /opt/homebrew/bin/gadak
	// must not leak into this row.
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	fileIsExec = func(string) bool { return false }

	item := itemByID(t, List(), IDCommandLineTool)
	if item.Installed == nil || *item.Installed {
		t.Fatalf("empty PATH: installed=%v want false", item.Installed)
	}
	if item.Detail != "not on PATH" {
		t.Fatalf("empty PATH: detail=%q want not on PATH", item.Detail)
	}
	if item.Title != "Command line tool" || item.Command != "gadak install-cli" {
		t.Fatalf("title=%q command=%q", item.Title, item.Command)
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "gadak")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	item = itemByID(t, List(), IDCommandLineTool)
	if item.Installed == nil || !*item.Installed {
		t.Fatalf("PATH gadak: installed=%v want true", item.Installed)
	}
	if item.Detail != clitool.TildeHome(bin) {
		t.Fatalf("PATH gadak: detail=%q want %q", item.Detail, clitool.TildeHome(bin))
	}

	// Fallback: LookPath misses, a well-known path is executable.
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	fileIsExec = func(p string) bool { return p == "/usr/local/bin/gadak" }
	item = itemByID(t, List(), IDCommandLineTool)
	if item.Installed == nil || !*item.Installed {
		t.Fatalf("fallback: installed=%v want true", item.Installed)
	}
	if item.Detail != "/usr/local/bin/gadak" {
		t.Fatalf("fallback: detail=%q", item.Detail)
	}
}

func itemByID(t *testing.T, items []Item, id string) Item {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no item %q in %d rows", id, len(items))
	return Item{}
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
	prev := mcpProbeTimeout
	mcpProbeTimeout = 50 * time.Millisecond // short value is owned by this test
	t.Cleanup(func() { mcpProbeTimeout = prev })

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

// Raycast does not exist on Windows. The catalog must not offer a row whose
// Install button can run (GDK-244). ListFor is the GOOS seam so this pins
// the Windows catalog on a Linux/macOS CI host.
func TestListForWindowsOmitsRaycast(t *testing.T) {
	stubNoClaude(t)
	items := ListFor("windows")
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
		if it.ID == IDRaycast {
			t.Fatalf("windows catalog must not include raycast: %+v", it)
		}
	}
	want := []string{IDCommandLineTool, IDSkill, IDMCPClaude}
	if len(ids) != len(want) {
		t.Fatalf("windows ids=%v want %v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("windows ids=%v want %v", ids, want)
		}
	}
}

func TestListForDarwinKeepsRaycast(t *testing.T) {
	stubNoClaude(t)
	items := ListFor("darwin")
	if len(items) != 4 {
		t.Fatalf("darwin len=%d want 4", len(items))
	}
	if items[1].ID != IDRaycast {
		t.Fatalf("darwin order %v", []string{items[0].ID, items[1].ID, items[2].ID, items[3].ID})
	}
}

func TestInstallArgsForWindowsRejectsRaycast(t *testing.T) {
	if args, ok := InstallArgsFor(IDRaycast, "windows"); ok {
		t.Fatalf("windows must not install raycast, args=%v", args)
	}
	if _, ok := InstallArgsFor(IDSkill, "windows"); !ok {
		t.Fatal("skill must still be installable on windows")
	}
	if _, ok := InstallArgsFor(IDCommandLineTool, "windows"); !ok {
		t.Fatal("command-line-tool must still be installable on windows")
	}
	if _, ok := InstallArgsFor(IDRaycast, "darwin"); !ok {
		t.Fatal("darwin must still install raycast")
	}
}

func TestListMatchesListForThisGOOS(t *testing.T) {
	stubNoClaude(t)
	got := List()
	want := ListFor(runtime.GOOS)
	if len(got) != len(want) {
		t.Fatalf("List len=%d ListFor(%s) len=%d", len(got), runtime.GOOS, len(want))
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Fatalf("List[%d]=%q ListFor=%q", i, got[i].ID, want[i].ID)
		}
	}
}

// TestCataloguePathDoesNotExecClaude is the class gate: List/ListFor must
// not spawn a subprocess. A poison claude on PATH is exec'd if lookPath
// still resolves (the pre-fix catalogue tests) or if production stops
// honoring the lookPath seam and shells out.
func TestCataloguePathDoesNotExecClaude(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "execed")
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf ran >'" + marker + "'\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	stubNoClaude(t)

	_ = List()
	_ = ListFor("darwin")
	_ = ListFor("windows")

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("catalogue List/ListFor must not exec claude")
	}
}
