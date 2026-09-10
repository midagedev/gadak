package integrations

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/skillinstall"
)

// stubNoClaude makes List/listFor skip the MCP probe. Catalogue tests
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

// Catalogue tests that call List/listFor without stubNoClaude used to hit a
// real `claude mcp get gadak` (Linux Raycast row, 2s). Probe tests replace
// lookPath themselves; anyone else resolving "claude" is a missed stub.
func TestMain(m *testing.M) {
	orig := lookPath
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			panic("List/listFor resolved claude without stubNoClaude; TestMCPProbe* must install their own lookPath")
		}
		return orig(name)
	}
	os.Exit(m.Run())
}

// fakeSkillEnv points the skill rows at a throwaway home. It is a seam and
// not an environment variable on purpose: nothing in this package may resolve
// to the developer's real ~/.claude — and CODEX_HOME in particular is a user
// preference, never a fence (orca's tripwire: the Codex binary ignores the
// USERPROFILE sandbox on Windows).
func fakeSkillEnv(t *testing.T, home string) {
	t.Helper()
	prev := skillEnv
	skillEnv = func() skillinstall.Env {
		return skillinstall.Env{
			Home:   home,
			Cwd:    filepath.Join(home, "work"),
			Getenv: func(string) string { return "" },
		}
	}
	t.Cleanup(func() { skillEnv = prev })
}

// mkConfigDir makes a host's configuration directory look lived-in. An empty
// directory is not evidence (skillinstall.Present ignores OS droppings), so
// the marker file is the point.
func mkConfigDir(t *testing.T, home string, parts ...string) {
	t.Helper()
	dir := filepath.Join(append([]string{home}, parts...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ids(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestListOrderAndDetectFlip(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	gadakHome := filepath.Join(home, ".gadak")
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", gadakHome)
	fakeSkillEnv(t, home)
	mkConfigDir(t, home, ".claude")

	items := listFor("darwin")
	// cli, raycast, skill-claude, skill-agents (always), mcp-claude,
	// mcp-claude-desktop.
	want := []string{idCommandLineTool, idRaycast, "skill-claude", "skill-agents", idMCPClaude, idMCPClaudeDesktop}
	if got := ids(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("ids=%v want %v", got, want)
	}
	if items[0].Command != "gadak install-cli" {
		t.Fatalf("cli command=%q", items[0].Command)
	}
	raycast := itemByID(t, items, idRaycast)
	if raycast.Installed == nil || *raycast.Installed {
		t.Fatalf("raycast want false, got %v", raycast.Installed)
	}
	if raycast.Detail != "~/.gadak/raycast-extension" {
		t.Fatalf("raycast detail=%q", raycast.Detail)
	}
	skill := itemByID(t, items, "skill-claude")
	if skill.Installed == nil || *skill.Installed {
		t.Fatalf("skill want false, got %v", skill.Installed)
	}
	if skill.Status != skillinstall.StatusMissing {
		t.Fatalf("skill status=%q want missing", skill.Status)
	}
	if skill.Prerequisite != nil {
		t.Fatalf("skill prerequisite=%v want nil", skill.Prerequisite)
	}
	if want := clitool.TildeHome(filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md")); skill.Detail != want {
		t.Fatalf("skill detail=%q want %q", skill.Detail, want)
	}

	if err := os.MkdirAll(filepath.Join(gadakHome, "raycast-extension"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gadakHome, "raycast-extension", "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, home, ".claude", gadak.SkillMarkdown())

	items = listFor("darwin")
	raycast = itemByID(t, items, idRaycast)
	if raycast.Installed == nil || !*raycast.Installed {
		t.Fatalf("raycast after touch: %v", raycast.Installed)
	}
	skill = itemByID(t, items, "skill-claude")
	if skill.Installed == nil || !*skill.Installed {
		t.Fatalf("skill after touch: %v", skill.Installed)
	}
	if skill.Status != "current" {
		t.Fatalf("skill status=%q want current", skill.Status)
	}

	// package.json without node_modules is an interrupted install: still
	// installed (Update re-runs the verb) but the detail says so.
	if !strings.Contains(raycast.Detail, "node_modules missing") {
		t.Fatalf("raycast detail should flag missing node_modules, got %q", raycast.Detail)
	}
	if err := os.MkdirAll(filepath.Join(gadakHome, "raycast-extension", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	raycast = itemByID(t, listFor("darwin"), idRaycast)
	if strings.Contains(raycast.Detail, "incomplete") {
		t.Fatalf("raycast detail should be clean with node_modules present, got %q", raycast.Detail)
	}
}

// writeSkill puts content at <home>/<configDir>/skills/gadak/SKILL.md and
// returns the directory it wrote into.
func writeSkill(t *testing.T, home, configDir string, content []byte) string {
	t.Helper()
	dir := filepath.Join(home, configDir, "skills", "gadak")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A host gadak cannot find gets no row at all — the Raycast rule (GDK-1513).
// The one exception is .agents: the shared root every agentskills.io host
// reads is worth offering before any particular host is installed.
func TestSkillRowsFollowConfigDirs(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", filepath.Join(home, ".gadak"))
	fakeSkillEnv(t, home)
	mkConfigDir(t, home, ".codex")

	want := []string{idCommandLineTool, idRaycast, "skill-codex", "skill-agents", idMCPClaude, idMCPClaudeDesktop}
	if got := ids(listFor("darwin")); !reflect.DeepEqual(got, want) {
		t.Fatalf("only ~/.codex: ids=%v want %v", got, want)
	}
	wantWin := []string{idCommandLineTool, "skill-codex", "skill-agents", idMCPClaude, idMCPClaudeDesktop}
	if got := ids(listFor("windows")); !reflect.DeepEqual(got, wantWin) {
		t.Fatalf("windows: ids=%v want %v", got, wantWin)
	}

	// Every host present: the rows follow skillinstall's table order.
	for _, parts := range [][]string{{".claude"}, {".agents"}, {".cursor"}, {".gemini"}, {".config", "opencode"}, {".grok"}} {
		mkConfigDir(t, home, parts...)
	}
	wantAll := []string{idCommandLineTool, idRaycast}
	for _, c := range skillinstall.Clients() {
		wantAll = append(wantAll, skillRowID(c.Name))
	}
	wantAll = append(wantAll, idMCPClaude, idMCPClaudeDesktop)
	if got := ids(listFor("darwin")); !reflect.DeepEqual(got, wantAll) {
		t.Fatalf("all hosts: ids=%v want %v", got, wantAll)
	}
}

// A directory holding nothing but what the operating system wrote is not
// evidence that a host is installed (orca's .DS_Store lesson, ported into
// skillinstall.Present).
func TestSkillRowsIgnoreOSDroppings(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", filepath.Join(home, ".gadak"))
	fakeSkillEnv(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cursor", ".DS_Store"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, it := range listFor("darwin") {
		if it.ID == "skill-cursor" {
			t.Fatalf("a Finder visit must not offer a Cursor row: %+v", it)
		}
	}
}

// The row's status is the word `gadak doctor` uses, from the same classifier.
// The stale case is the one that used to lie: `Installed: fileExists(dest)`
// called a three-release-old file installed while doctor called it stale
// (GDK-1514).
func TestSkillRowStatusPerState(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", filepath.Join(home, ".gadak"))
	fakeSkillEnv(t, home)
	mkConfigDir(t, home, ".claude")

	row := func() Item { return itemByID(t, listFor("darwin"), "skill-claude") }

	if got := row(); got.Status != skillinstall.StatusMissing || got.Installed == nil || *got.Installed {
		t.Fatalf("nothing there: status=%q installed=%v want missing/false", got.Status, got.Installed)
	}

	writeSkill(t, home, ".claude", gadak.SkillMarkdown())
	if got := row(); got.Status != "current" || got.Installed == nil || !*got.Installed {
		t.Fatalf("embedded copy: status=%q installed=%v want current/true", got.Status, got.Installed)
	}

	// gadak's own copy from an earlier release: different bytes, and the
	// receipt beside them proves gadak wrote them.
	older := []byte("---\nname: gadak\ndescription: an older release\n---\n\nold body\n")
	dir := writeSkill(t, home, ".claude", older)
	if err := skillinstall.WriteReceipt(dir, older, "0.0.0-test"); err != nil {
		t.Fatal(err)
	}
	if got := row(); got.Status != skillinstall.StatusStale || got.Installed == nil || !*got.Installed {
		t.Fatalf("stale copy: status=%q installed=%v want stale/true — this is the state fileExists could not see", got.Status, got.Installed)
	}

	// The user's own file: gadak did not write it, and only --force replaces
	// it. Still installed — something is there — but not gadak's.
	if err := os.Remove(filepath.Join(dir, skillinstall.ReceiptName)); err != nil {
		t.Fatal(err)
	}
	if got := row(); got.Status != skillinstall.StatusConflict || got.Installed == nil || !*got.Installed {
		t.Fatalf("hand-edited copy: status=%q installed=%v want conflict/true", got.Status, got.Installed)
	}
}

// Non-skill rows carry no status word, and the field leaves the wire when it
// is empty so those rows are byte-identical to what they always were.
func TestNonSkillRowsHaveNoStatus(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", filepath.Join(home, ".gadak"))
	fakeSkillEnv(t, home)

	for _, it := range listFor("darwin") {
		isSkill := strings.HasPrefix(it.ID, skillIDPrefix)
		if isSkill == (it.Status == "") {
			t.Fatalf("row %q: status=%q (skill row=%v)", it.ID, it.Status, isSkill)
		}
	}
	b, err := json.Marshal(Item{ID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"status"`) {
		t.Fatalf("empty status must not reach the wire: %s", b)
	}
}

func TestInstallArgs(t *testing.T) {
	// The pre-GDK-1513 id still runs the default client: a window open across
	// the upgrade, or a stored id, posts "skill" and must not 404.
	args, ok := InstallArgs(idSkill)
	if !ok || strings.Join(args, " ") != "skill install claude" {
		t.Fatalf("legacy skill args=%v ok=%v", args, ok)
	}
	args, ok = InstallArgs(idCommandLineTool)
	if !ok || len(args) != 1 || args[0] != "install-cli" {
		t.Fatalf("cli args=%v ok=%v want [install-cli]", args, ok)
	}
	// Two Claude rows, two verbs: the Code row execs the claude CLI, the
	// Desktop row writes the config file itself.
	args, ok = InstallArgs(idMCPClaude)
	if !ok || strings.Join(args, " ") != "mcp install claude" {
		t.Fatalf("mcp-claude args=%v ok=%v", args, ok)
	}
	args, ok = InstallArgs(idMCPClaudeDesktop)
	if !ok || strings.Join(args, " ") != "mcp install claude-desktop" {
		t.Fatalf("mcp-claude-desktop args=%v ok=%v", args, ok)
	}
	if _, ok := InstallArgs("nope"); ok {
		t.Fatal("unknown id must be false")
	}
}

// TestInstallArgsForce — GDK-1535. A conflict skill row's Replace needs the
// one flag the armed confirm bought; the plain verbs stay byte-identical so a
// stray ?force=1 on the wire cannot turn any other install into an overwrite.
func TestInstallArgsForce(t *testing.T) {
	for _, id := range []string{idSkill, "skill-codex"} {
		args, ok := InstallArgsForce(id)
		if !ok {
			t.Fatalf("%s: InstallArgsForce ok=false", id)
		}
		want, _ := InstallArgs(id)
		if len(args) != len(want)+1 || args[len(args)-1] != "--force" {
			t.Fatalf("%s force args=%v, want exactly one --force after %v", id, args, want)
		}
		for i := range want {
			if args[i] != want[i] {
				t.Fatalf("%s force args=%v reshaped %v", id, args, want)
			}
		}
	}
	// Every other row ignores the flag: same argv as the plain verb.
	for _, id := range []string{idCommandLineTool, idMCPClaude, idMCPClaudeDesktop} {
		forced, ok1 := InstallArgsForce(id)
		plain, ok2 := InstallArgs(id)
		if !ok1 || !ok2 || strings.Join(forced, " ") != strings.Join(plain, " ") {
			t.Fatalf("%s: force=%v (%v), plain=%v (%v) — non-skill rows stay flagless", id, forced, ok1, plain, ok2)
		}
	}
	if _, ok := InstallArgsForce("nope"); ok {
		t.Fatal("unknown id must be false for the force variant too")
	}
}

// Two Claude rows, each truthful: mcp-claude is Claude Code (the claude CLI
// is the registrar), mcp-claude-desktop is Claude Desktop (the config file on
// disk). Before GDK-1633 one row carried both names and matched neither.
func TestMCPClaudeRowsTitlesAndCommands(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeSkillEnv(t, home)

	got := ids(listFor("darwin"))
	if got[len(got)-2] != idMCPClaude || got[len(got)-1] != idMCPClaudeDesktop {
		t.Fatalf("the desktop row must sit right after the code row: %v", got)
	}
	code := itemByID(t, listFor("darwin"), idMCPClaude)
	if code.Title != "Claude Code MCP" || code.Command != "gadak mcp install claude" {
		t.Fatalf("code row: title=%q command=%q", code.Title, code.Command)
	}
	// The missing-claude branch carries the same title — the card never goes
	// back to calling the CLI row "Claude Desktop".
	if code.Prerequisite == nil || code.Prerequisite.OK || code.Prerequisite.Message != "claude CLI is not on PATH" {
		t.Fatalf("code prerequisite: %+v", code.Prerequisite)
	}
	desktop := itemByID(t, listFor("darwin"), idMCPClaudeDesktop)
	if desktop.Title != "Claude Desktop MCP" || desktop.Command != "gadak mcp install claude-desktop" {
		t.Fatalf("desktop row: title=%q command=%q", desktop.Title, desktop.Command)
	}
}

// listFor(goos) promises a catalogue for that GOOS, and the Claude Desktop
// row is the one whose content is a per-OS path — so it is the row that can
// silently answer for the host instead. It did: the row resolved its path
// through runtime.GOOS while the test asked for darwin, which passed on a
// Mac and failed on Linux CI with a ~/.config/Claude path (2026-09-08).
// This asserts all three branches on any host, so a row that reaches for the
// process's own GOOS fails wherever it is run.
func TestMCPClaudeDesktopRowFollowsGOOS(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	appdata := filepath.Join(home, "AppData", "Roaming")
	t.Setenv("APPDATA", appdata)
	t.Setenv("XDG_CONFIG_HOME", "")
	fakeSkillEnv(t, home)

	for _, goos := range []string{"darwin", "linux", "windows"} {
		want, err := clitool.ClaudeDesktopConfigPathFor(goos, home, appdata, "")
		if err != nil {
			t.Fatalf("%s: want path: %v", goos, err)
		}
		item := itemByID(t, listFor(goos), idMCPClaudeDesktop)
		if item.Detail != clitool.TildeHome(want) {
			t.Errorf("listFor(%q) desktop row detail=%q want %q", goos, item.Detail, clitool.TildeHome(want))
		}
		if item.Prerequisite == nil || !strings.Contains(item.Prerequisite.Message, clitool.TildeHome(filepath.Dir(want))) {
			t.Errorf("listFor(%q) prerequisite must name %q: %+v", goos, clitool.TildeHome(filepath.Dir(want)), item.Prerequisite)
		}
	}
}

// The desktop row reads claude_desktop_config.json only. States: no app
// directory (prerequisite fails naming it), directory without config (not
// installed), config without gadak (false), config with gadak (true), and an
// unparsable config (unknown, detail says unreadable).
func TestMCPClaudeDesktopRowPerState(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeSkillEnv(t, home)
	row := func() Item { return itemByID(t, listFor("darwin"), idMCPClaudeDesktop) }

	cfgDir := filepath.Join(home, "Library", "Application Support", "Claude")
	cfg := filepath.Join(cfgDir, "claude_desktop_config.json")

	item := row()
	if item.Prerequisite == nil || item.Prerequisite.OK {
		t.Fatalf("no app dir: prerequisite=%+v", item.Prerequisite)
	}
	if !strings.Contains(item.Prerequisite.Message, "~/Library/Application Support/Claude") {
		// Naming the GOOS matters: when this row answered for the host
		// instead of the requested OS, the bare message read as a path
		// format problem rather than "listFor(darwin) returned a Linux
		// path", which is what it was.
		t.Fatalf("listFor(%q) prerequisite must name the missing dir: %q", "darwin", item.Prerequisite.Message)
	}
	if item.Installed == nil || *item.Installed {
		t.Fatalf("no app dir: installed=%v want false", item.Installed)
	}

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	item = row()
	if item.Prerequisite == nil || !item.Prerequisite.OK {
		t.Fatalf("app dir present: prerequisite=%+v", item.Prerequisite)
	}
	if item.Installed == nil || *item.Installed {
		t.Fatalf("no config: installed=%v want false", item.Installed)
	}
	if item.Detail != clitool.TildeHome(cfg) {
		t.Fatalf("detail=%q want %q", item.Detail, clitool.TildeHome(cfg))
	}

	if err := os.WriteFile(cfg, []byte(`{"mcpServers": {"other": {"command": "x"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if item = row(); item.Installed == nil || *item.Installed {
		t.Fatalf("other server only: installed=%v want false", item.Installed)
	}

	if err := os.WriteFile(cfg, []byte(`{"mcpServers": {"gadak": {"command": "gadak", "args": ["mcp"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if item = row(); item.Installed == nil || !*item.Installed {
		t.Fatalf("registered: installed=%v want true", item.Installed)
	}

	if err := os.WriteFile(cfg, []byte(`{"mcpServers": {`), 0o644); err != nil {
		t.Fatal(err)
	}
	if item = row(); item.Installed != nil {
		t.Fatalf("unparsable: installed=%v want nil", item.Installed)
	}
	if item.Detail != "unreadable: "+clitool.TildeHome(cfg) {
		t.Fatalf("detail=%q", item.Detail)
	}
}

// One argv per host, and never --project: the app has no notion of the
// working directory the user means (GDK-1513).
func TestInstallArgsPerSkillHost(t *testing.T) {
	for _, client := range skillinstall.Clients() {
		id := skillRowID(client.Name)
		args, ok := InstallArgs(id)
		if !ok {
			t.Fatalf("%s: not installable", id)
		}
		want := []string{"skill", "install", client.Name}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("%s: args=%v want %v", id, args, want)
		}
		for _, a := range args {
			if strings.HasPrefix(a, "--") {
				t.Fatalf("%s: argv must carry no flags, got %v", id, args)
			}
		}
		// Windows has every skill host Windows can have; the GOOS seam is
		// Raycast's alone.
		if _, ok := InstallArgsFor(id, "windows"); !ok {
			t.Fatalf("%s: must still be installable on windows", id)
		}
	}
	if _, ok := InstallArgs("skill-nosuchhost"); ok {
		t.Fatal("an unknown host behind the skill- prefix must not be installable")
	}
	if _, ok := InstallArgs(skillIDPrefix); ok {
		t.Fatal("a bare prefix must not be installable")
	}
}

func TestCommandLineToolDetectFlip(t *testing.T) {
	stubNoClaude(t)
	fakeSkillEnv(t, t.TempDir())
	oldExec := fileIsExec
	t.Cleanup(func() {
		fileIsExec = oldExec
	})

	// Empty PATH plus no fallbacks: the machine's /opt/homebrew/bin/gadak
	// must not leak into this row.
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	fileIsExec = func(string) bool { return false }

	item := itemByID(t, List(), idCommandLineTool)
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

	item = itemByID(t, List(), idCommandLineTool)
	if item.Installed == nil || !*item.Installed {
		t.Fatalf("PATH gadak: installed=%v want true", item.Installed)
	}
	if item.Detail != clitool.TildeHome(bin) {
		t.Fatalf("PATH gadak: detail=%q want %q", item.Detail, clitool.TildeHome(bin))
	}

	// Fallback: LookPath misses, a well-known path is executable.
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	fileIsExec = func(p string) bool { return p == "/usr/local/bin/gadak" }
	item = itemByID(t, List(), idCommandLineTool)
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
	if _, ok := clitool.ResolveNPM(look, func(string) bool { return false }); ok {
		t.Fatal("no candidate should miss")
	}
	got, ok := clitool.ResolveNPM(func(string) (string, error) { return "/tmp/path-npm", nil }, func(string) bool { return false })
	if !ok || got != "/tmp/path-npm" {
		t.Fatalf("PATH win: %q ok=%v", got, ok)
	}
	got, ok = clitool.ResolveNPM(look, func(p string) bool { return p == "/opt/homebrew/bin/npm" })
	if !ok || got != "/opt/homebrew/bin/npm" {
		t.Fatalf("brew fallback: %q ok=%v", got, ok)
	}
	got, ok = clitool.ResolveNPM(look, func(p string) bool { return p == "/usr/local/bin/npm" })
	if !ok || got != "/usr/local/bin/npm" {
		t.Fatalf("usr/local fallback: %q ok=%v", got, ok)
	}
}

func TestRaycastPrerequisiteListsTriedNPM(t *testing.T) {
	prevLook, prevExec := lookPath, fileIsExec
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	fileIsExec = func(string) bool { return false }
	t.Cleanup(func() {
		lookPath = prevLook
		fileIsExec = prevExec
	})
	item := raycastItem()
	if item.Prerequisite == nil || item.Prerequisite.OK {
		t.Fatalf("missing npm: prerequisite=%+v", item.Prerequisite)
	}
	msg := item.Prerequisite.Message
	detail := clitool.NPMNotFoundDetail()
	if !strings.Contains(msg, detail) {
		t.Errorf("raycast prerequisite must include %q; got %q", detail, msg)
	}
	for _, p := range clitool.NPMFallbackPaths {
		if !strings.Contains(msg, p) {
			t.Errorf("raycast prerequisite must name tried path %s; got %q", p, msg)
		}
	}
}

// fakeProbeRun installs one canned probeRun result (GDK-723): the answer
// tests assert classification, not process scheduling, so they swap the
// exec seam and the timeout contract stays with the one real-exec test.
func fakeProbeRun(t *testing.T, res probeRunResult) {
	t.Helper()
	prev := probeRun
	probeRun = func(string) probeRunResult { return res }
	t.Cleanup(func() { probeRun = prev })
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

	// The get ran and failed without claude's not-registered wording:
	// unknown, not a definitive false.
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/nonexistent/claude", nil
		}
		return old(name)
	}
	fakeProbeRun(t, probeRunResult{err: errors.New("exit status 1")})
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
	old := lookPath
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/nonexistent/claude", nil
		}
		return old(name)
	}
	t.Cleanup(func() { lookPath = old })
	fakeProbeRun(t, probeRunResult{
		out: `No MCP server named "gadak". Configured servers: x`,
		err: errors.New("exit status 1"),
	})

	item := mcpClaudeItem()
	if item.Installed == nil || *item.Installed {
		t.Fatalf("not-registered wording: installed=%v want false", item.Installed)
	}
	if !strings.Contains(item.Detail, "not registered") {
		t.Fatalf("detail=%q want not-registered wording", item.Detail)
	}
}

func TestMCPProbeTrueOnExitZero(t *testing.T) {
	old := lookPath
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/nonexistent/claude", nil
		}
		return old(name)
	}
	t.Cleanup(func() { lookPath = old })
	fakeProbeRun(t, probeRunResult{})

	item := mcpClaudeItem()
	if item.Installed == nil || !*item.Installed {
		t.Fatalf("exit 0: installed=%v want true", item.Installed)
	}
}

// TestMCPProbeAnswersSurviveStarvedBudget — GDK-723's standing contract: the
// probe's answers are classification, not scheduling. Before the probeRun
// seam these assertions ran `#!/bin/sh` stubs and needed a 30s budget to
// stay green under load (GDK-303); FAIL-first on the pre-seam source: a 1ms
// budget turned "exit 0 → true" into `installed=<nil> want true`
// (/tmp/gdk723-failfirst.txt). With the fake runner the budget starves to
// nothing and the answers do not move.
func TestMCPProbeAnswersSurviveStarvedBudget(t *testing.T) {
	prev := mcpProbeTimeout
	mcpProbeTimeout = time.Nanosecond
	t.Cleanup(func() { mcpProbeTimeout = prev })

	old := lookPath
	lookPath = func(string) (string, error) { return "/nonexistent/claude", nil }
	t.Cleanup(func() { lookPath = old })

	fakeProbeRun(t, probeRunResult{})
	if got := probeClaudeMCP("/nonexistent/claude"); got == nil || !*got {
		t.Fatalf("starved budget: exit-0 answer = %v, want true — the answer must not depend on the clock", got)
	}
	fakeProbeRun(t, probeRunResult{out: `No MCP server named "gadak"`, err: errors.New("exit status 1")})
	if got := probeClaudeMCP("/nonexistent/claude"); got == nil || *got {
		t.Fatalf("starved budget: not-registered answer = %v, want false", got)
	}
	fakeProbeRun(t, probeRunResult{err: errors.New("exit status 1")})
	if got := probeClaudeMCP("/nonexistent/claude"); got != nil {
		t.Fatalf("starved budget: bare failure = %v, want unknown", *got)
	}
}

// TestMCPProbeTimeoutIsUnknown is the one test that still execs (GDK-723):
// the timeout contract — kill, reap, unknown — only means something against
// a real process. The absent-binary branch rides along because it is the
// real implementation's other half and costs no spawn.
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
	if _, outcome := probeClaudeMCPOutcome(filepath.Join(dir, "does-not-exist")); outcome != probeNotStarted {
		t.Fatalf("absent binary: outcome=%s want not-started", outcome)
	}
}

// The two causes of "unknown" must be distinguishable. Without this, a starved
// machine and a broken CLI produce the same nil, and the answer-asserting tests
// above fail with a message about the wrong thing (GDK-303). All branches run
// through the fake runner: this test pins the classifier, and the real process
// behaviour belongs to the one exec test above (GDK-723).
func TestProbeOutcomeSeparatesTimeoutFromAnswer(t *testing.T) {
	fakeProbeRun(t, probeRunResult{timedOut: true})
	if got, outcome := probeClaudeMCPOutcome("claude"); got != nil || outcome != probeTimedOut {
		t.Fatalf("timed out: got=%v outcome=%s want nil/timed-out", got, outcome)
	}

	fakeProbeRun(t, probeRunResult{err: errors.New("exit status 1")})
	if got, outcome := probeClaudeMCPOutcome("claude"); got != nil || outcome != probeAnswered {
		t.Fatalf("bare failure: got=%v outcome=%s want nil/answered", got, outcome)
	}

	fakeProbeRun(t, probeRunResult{startErr: errors.New("no such file")})
	if got, outcome := probeClaudeMCPOutcome("claude"); got != nil || outcome != probeNotStarted {
		t.Fatalf("not started: got=%v outcome=%s want nil/not-started", got, outcome)
	}

	fakeProbeRun(t, probeRunResult{})
	if got, outcome := probeClaudeMCPOutcome("claude"); got == nil || !*got || outcome != probeAnswered {
		t.Fatalf("exit 0: got=%v outcome=%s want true/answered", got, outcome)
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
// Install button can run (GDK-244). listFor is the GOOS seam so this pins
// the Windows catalog on a Linux/macOS CI host.
func TestListForWindowsOmitsRaycast(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	fakeSkillEnv(t, home)
	mkConfigDir(t, home, ".claude")
	items := listFor("windows")
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
		if it.ID == idRaycast {
			t.Fatalf("windows catalog must not include raycast: %+v", it)
		}
	}
	want := []string{idCommandLineTool, "skill-claude", "skill-agents", idMCPClaude, idMCPClaudeDesktop}
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
	home := t.TempDir()
	fakeSkillEnv(t, home)
	mkConfigDir(t, home, ".claude")
	items := listFor("darwin")
	if len(items) != 6 {
		t.Fatalf("darwin len=%d want 6: %v", len(items), ids(items))
	}
	if items[1].ID != idRaycast {
		t.Fatalf("darwin order %v", ids(items))
	}
}

func TestInstallArgsForWindowsRejectsRaycast(t *testing.T) {
	if args, ok := InstallArgsFor(idRaycast, "windows"); ok {
		t.Fatalf("windows must not install raycast, args=%v", args)
	}
	if _, ok := InstallArgsFor(idSkill, "windows"); !ok {
		t.Fatal("skill must still be installable on windows")
	}
	if _, ok := InstallArgsFor(idCommandLineTool, "windows"); !ok {
		t.Fatal("command-line-tool must still be installable on windows")
	}
	if _, ok := InstallArgsFor(idRaycast, "darwin"); !ok {
		t.Fatal("darwin must still install raycast")
	}
}

// GDK-354 / os-audit F-7: Raycast is a macOS app, and `gadak raycast install`
// refuses every non-darwin GOOS — so the catalog must not offer a row whose
// Install button is guaranteed to fail. Same reasoning as GDK-244 on Windows.
func TestInstallArgsForLinuxRejectsRaycast(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	fakeSkillEnv(t, home)
	mkConfigDir(t, home, ".claude")
	if args, ok := InstallArgsFor(idRaycast, "linux"); ok {
		t.Fatalf("linux must not install raycast, args=%v", args)
	}
	for _, item := range listFor("linux") {
		if item.ID == idRaycast {
			t.Fatal("linux catalog must not list raycast")
		}
	}
	if _, ok := InstallArgsFor(idSkill, "linux"); !ok {
		t.Fatal("skill must still be installable on linux")
	}
}

func TestListMatchesListForThisGOOS(t *testing.T) {
	stubNoClaude(t)
	home := t.TempDir()
	fakeSkillEnv(t, home)
	mkConfigDir(t, home, ".claude")
	got := List()
	want := listFor(runtime.GOOS)
	if len(got) != len(want) {
		t.Fatalf("List len=%d listFor(%s) len=%d", len(got), runtime.GOOS, len(want))
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Fatalf("List[%d]=%q listFor=%q", i, got[i].ID, want[i].ID)
		}
	}
}

// TestCataloguePathDoesNotExecClaude is the class gate: List/listFor must
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
	fakeSkillEnv(t, t.TempDir())

	_ = List()
	_ = listFor("darwin")
	_ = listFor("windows")
	_ = listFor("linux")

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("catalogue List/listFor must not exec claude")
	}
}
