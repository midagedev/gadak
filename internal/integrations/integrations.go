// Package integrations is the desktop Settings catalog: which local
// agent/host tools gadak can install, how to detect them, and which
// gadak CLI argv installs each one. Detection is files and local
// processes only — no network, no mirror.
package integrations

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/skillinstall"
)

// IDs are part of the GET/POST contract. Order of List is fixed:
// command-line-tool, raycast (darwin only), one row per skill host,
// mcp-claude, mcp-claude-desktop.
const (
	idCommandLineTool  = "command-line-tool"
	idRaycast          = "raycast"
	idMCPClaude        = "mcp-claude"
	idMCPClaudeDesktop = "mcp-claude-desktop"

	// idSkill is the single skill row gadak listed before GDK-1513. List no
	// longer emits it — there is a row per host now, `skill-<client>` — but the
	// install route still answers to it, and must keep doing so: a desktop
	// window left open across an upgrade, or a UI that stored the id, posts
	// the id it was handed. It runs the default client's install, which is
	// exactly what the old row ran.
	idSkill = "skill"

	// skillIDPrefix + a skillinstall client name is a skill row's id.
	skillIDPrefix = "skill-"

	// universalSkillClient is the one host offered whether or not it is
	// already on the machine: ~/.agents is the shared root every
	// agentskills.io host reads, so it is worth installing before any
	// particular host is, and stays worth it afterwards. orca reaches the
	// same conclusion from the other side — its per-agent table always
	// includes the universal key so an unmapped agent still gets the skill.
	universalSkillClient = "agents"
)

// skillRowID is the catalog id for one skill host.
func skillRowID(client string) string { return skillIDPrefix + client }

// skillEnv is the process environment the destination table reads. It is the
// installer's own (skillinstall.OSEnv, os.UserHomeDir), so Settings and
// `gadak skill install` resolve the same home — a $HOME override the installer
// ignores must not make Settings call an installed skill missing (GDK-352).
//
// A var so tests build a hermetic Env rather than reaching for the real home.
var skillEnv = skillinstall.OSEnv

// skillContent is the embedded SKILL.md. Same accessor `gadak doctor` reads
// (cmd/gadak/doctor.go collectSkillStatus) — the app and doctor must classify
// the same bytes or they will disagree about the same file.
var skillContent = gadak.SkillMarkdown

// mcpProbeTimeout is the production probe budget. Tests may assign a
// shorter value and restore it with t.Cleanup.
var mcpProbeTimeout = 3 * time.Second

// lookPath is exec.LookPath; tests inject a stub.
var lookPath = exec.LookPath

// fileIsExec reports a path exists and has an execute bit. Tests inject a stub.
var fileIsExec = isExecutable

// gadakFallbackPaths: LookPath("gadak") then these, first existing+executable wins.
var gadakFallbackPaths = []string{
	"/opt/homebrew/bin/gadak",
	"/usr/local/bin/gadak",
}

// Prerequisite is a local dependency the install verb needs.
// Skill has none (JSON null).
type Prerequisite struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Item is one row of GET /desktop/integrations.
type Item struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Installed *bool  `json:"installed"`
	// Status is the skill verdict for a skill row — one of "current",
	// "stale", "missing", "conflict" — and "" for every other row. It is the
	// same word `gadak doctor` prints, from the same classifier
	// (skillinstall.DestStatus), so the two can no longer disagree about one
	// file (GDK-1514). Installed is derived from it and never the other way
	// round: a stale copy is installed *and* out of date, which one boolean
	// cannot say.
	Status       string        `json:"status,omitempty"`
	Detail       string        `json:"detail"`
	Command      string        `json:"command"`
	Prerequisite *Prerequisite `json:"prerequisite"`
}

// List returns the catalog rows for this host's GOOS, in contract order:
// command-line-tool, raycast (darwin only), the skill rows, mcp-claude,
// mcp-claude-desktop. Non-macOS hosts omit raycast — Raycast does not exist
// there, and a row whose Install button can run would lie (GDK-244, GDK-354).
func List() []Item {
	return listFor(runtime.GOOS)
}

// listFor is List with an explicit GOOS so the Windows catalog can be
// pinned on any host (same shape as clitool.ResolveFor).
func listFor(goos string) []Item {
	items := []Item{commandLineToolItem()}
	if raycastOffered(goos) {
		items = append(items, raycastItem())
	}
	items = append(items, skillItems()...)
	return append(items, mcpClaudeItem(), mcpClaudeDesktopItem())
}

// raycastOffered is the single owner of "does this OS get a Raycast row".
// The install endpoint uses the same predicate via InstallArgsFor. Raycast
// is a macOS app, and `gadak raycast install` refuses every non-darwin GOOS
// (GDK-354 / os-audit F-7) — a row it offers must be one it can install.
func raycastOffered(goos string) bool {
	return goos == "darwin"
}

// InstallArgs is the argv tail for the bundled gadak CLI. ok is false for an
// unknown id, and for raycast on Windows.
func InstallArgs(id string) ([]string, bool) {
	return InstallArgsFor(id, runtime.GOOS)
}

// InstallArgsFor is InstallArgs with an explicit GOOS.
func InstallArgsFor(id, goos string) ([]string, bool) {
	switch id {
	case idCommandLineTool:
		return []string{"install-cli"}, true
	case idRaycast:
		if !raycastOffered(goos) {
			return nil, false
		}
		return []string{"raycast", "install"}, true
	case idSkill:
		// The pre-GDK-1513 id, kept working: it ran the default client and
		// still does.
		return skillInstallArgs(skillinstall.DefaultClient), true
	case idMCPClaude:
		return []string{"mcp", "install", "claude"}, true
	case idMCPClaudeDesktop:
		return []string{"mcp", "install", "claude-desktop"}, true
	default:
		if name, found := strings.CutPrefix(id, skillIDPrefix); found {
			if client, known := skillinstall.Lookup(name); known {
				return skillInstallArgs(client.Name), true
			}
		}
		return nil, false
	}
}

// skillInstallArgs is the argv for one host, and the only place the app
// composes a skill install. Never --project: the app has no notion of the
// working directory the user means. The integrations tab is a settings
// surface; a project install belongs where the project is, which is a
// terminal (GDK-1513).
func skillInstallArgs(client string) []string {
	return []string{"skill", "install", client}
}

func commandLineToolItem() Item {
	path, ok := resolveGadak(lookPath, fileIsExec)
	if !ok {
		return Item{
			ID:           idCommandLineTool,
			Title:        "Command line tool",
			Installed:    boolPtr(false),
			Detail:       "not on PATH",
			Command:      "gadak install-cli",
			Prerequisite: nil,
		}
	}
	return Item{
		ID:           idCommandLineTool,
		Title:        "Command line tool",
		Installed:    boolPtr(true),
		Detail:       clitool.TildeHome(path),
		Command:      "gadak install-cli",
		Prerequisite: nil,
	}
}

func raycastItem() Item {
	dir := raycastExtDir()
	prereq := &Prerequisite{}
	if _, ok := clitool.ResolveNPM(lookPath, fileIsExec); ok {
		prereq.OK = true
	} else {
		prereq.Message = "npm is required (not found on " + clitool.NPMNotFoundDetail() + ")"
	}
	installed := fileExists(filepath.Join(dir, "package.json"))
	detail := clitool.TildeHome(dir)
	// An interrupted install can leave the manifest without node_modules;
	// the row still counts as installed (Update re-runs the verb), but the
	// detail must not pretend the deploy is whole.
	if installed && !dirExists(filepath.Join(dir, "node_modules")) {
		detail += " (incomplete — node_modules missing, run install again)"
	}
	return Item{
		ID:           idRaycast,
		Title:        "Raycast extension",
		Installed:    boolPtr(installed),
		Detail:       detail,
		Command:      "gadak raycast install",
		Prerequisite: prereq,
	}
}

// skillItems is one row per agent host that loads gadak's skill, in
// skillinstall's table order (GDK-1513).
//
// A host is offered only when its configuration directory is on this machine.
// That is the Raycast rule applied seven times: a row whose Install button
// writes a SKILL.md into a directory nothing reads is a button that lies. The
// test is the directory and not a binary on PATH, because several of these
// hosts ship as IDE extensions or as apps with no command of their own —
// skillinstall.Present is the single owner of that signal, and it ignores the
// .DS_Store a single Finder visit leaves behind.
//
// The .agents row is the exception and is always offered; see
// universalSkillClient.
func skillItems() []Item {
	env := skillEnv()
	content := skillContent()
	items := make([]Item, 0, len(skillinstall.Clients()))
	for _, client := range skillinstall.Clients() {
		if client.Name != universalSkillClient && !client.Present(env) {
			continue
		}
		items = append(items, skillItem(client, env, content))
	}
	return items
}

// skillItem is one host's row: where its copy goes, and what is there now.
func skillItem(client skillinstall.Client, env skillinstall.Env, content []byte) Item {
	item := Item{
		ID:      skillRowID(client.Name),
		Title:   client.Label + " skill",
		Command: "gadak " + strings.Join(skillInstallArgs(client.Name), " "),
	}
	dest, err := client.HomeDest(env)
	if err != nil {
		// No resolvable home: nothing to inspect, so nothing to promise.
		// Installed stays null — the pill reads "unknown" and points at the
		// command, which reports the same failure in its own words. The
		// detail is the documented location, ~ and all, because that is what
		// the user has to recognise on their own machine.
		item.Detail = client.HomeDoc()
		return item
	}
	item.Detail = clitool.TildeHome(dest)
	status, _, err := skillinstall.DestStatus(dest, content)
	if err != nil {
		// A directory where the file should be, or an unreadable one. Same
		// answer as above: unknown, and let the command say why.
		return item
	}
	item.Status = skillStatusWord(status)
	// Every word except "missing" means a copy is there. "Installed" alone
	// would call a three-release-old file current, which is the disagreement
	// with `gadak doctor` this row exists to end (GDK-1514).
	item.Installed = boolPtr(status != skillinstall.StatusMissing)
	return item
}

// skillStatusWord renames the installer's "identical" to "current", the word
// a report reads better with; the other three are already right.
//
// It is a copy of cmd/gadak/doctor.go's skillStatusWord because package main
// cannot be imported. The words themselves are contract and live in
// skillinstall; a StatusWord there would leave one owner (noted for the lead).
func skillStatusWord(installStatus string) string {
	if installStatus == skillinstall.StatusIdentical {
		return "current"
	}
	return installStatus
}

// mcpClaudeItem is Claude Code's MCP row. Everything in it is Claude Code:
// `gadak mcp install claude` execs the claude CLI, which writes Claude Code's
// own config — never Claude Desktop's. The shell-less host is the row below.
func mcpClaudeItem() Item {
	prereq := &Prerequisite{}
	path, err := lookPath("claude")
	if err != nil || path == "" {
		prereq.Message = "claude CLI is not on PATH"
		return Item{
			ID:           idMCPClaude,
			Title:        "Claude Code MCP",
			Installed:    nil,
			Detail:       "claude CLI not found",
			Command:      "gadak mcp install claude",
			Prerequisite: prereq,
		}
	}
	prereq.OK = true
	installed := probeClaudeMCP(path)
	detail := "unknown (claude mcp get gadak failed)"
	if installed != nil {
		if *installed {
			detail = "registered via claude mcp get gadak"
		} else {
			detail = "not registered (claude mcp get gadak)"
		}
	}
	return Item{
		ID:           idMCPClaude,
		Title:        "Claude Code MCP",
		Installed:    installed,
		Detail:       detail,
		Command:      "gadak mcp install claude",
		Prerequisite: prereq,
	}
}

// mcpClaudeDesktopItem is Claude Desktop's row — a different app and a
// different config file from Claude Code. Claude Desktop has no shell and no
// claude binary, so `gadak mcp install claude-desktop` merges the entry into
// claude_desktop_config.json itself. The row only reads that file; the
// install verb is the writer, exactly as for every other row.
func mcpClaudeDesktopItem() Item {
	item := Item{
		ID:      idMCPClaudeDesktop,
		Title:   "Claude Desktop MCP",
		Command: "gadak mcp install claude-desktop",
	}
	path, err := clitool.ClaudeDesktopConfigPath()
	if err != nil {
		// No resolvable home/APPDATA: nothing to inspect. Unknown, and the
		// command reports the same failure in its own words.
		item.Installed = nil
		item.Detail = "unknown (" + err.Error() + ")"
		item.Prerequisite = &Prerequisite{OK: false, Message: err.Error()}
		return item
	}
	prereq := &Prerequisite{}
	if dirExists(filepath.Dir(path)) {
		// The app has run at least once — the config directory exists.
		prereq.OK = true
	} else {
		prereq.Message = "Claude Desktop is not installed (no " + clitool.TildeHome(filepath.Dir(path)) + ")"
	}
	item.Prerequisite = prereq
	item.Detail = clitool.TildeHome(path)
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			// The app has run (its directory exists) but no config yet.
			item.Installed = boolPtr(false)
		} else {
			// Unreadable for another reason (permissions, a directory where
			// the file should be): the same unknown as an unparsable file.
			item.Installed = nil
			item.Detail = "unreadable: " + clitool.TildeHome(path)
		}
		return item
	}
	item.Installed = claudeDesktopInstalled(raw)
	if item.Installed == nil {
		// Unparsable body: the same unknown as an unreadable file.
		item.Detail = "unreadable: " + clitool.TildeHome(path)
	}
	return item
}

// claudeDesktopInstalled reads a claude_desktop_config.json body: true when it
// parses as an object naming a gadak server, false when absent from the map
// (including an mcpServers that is not an object), and unknown (nil) when the
// body is not parsable. A file the row does not own never errors the row.
func claudeDesktopInstalled(raw []byte) *bool {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return nil
	}
	serversRaw, ok := top["mcpServers"]
	if !ok {
		return boolPtr(false)
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &servers); err != nil || servers == nil {
		return boolPtr(false)
	}
	_, ok = servers["gadak"]
	return boolPtr(ok)
}

// raycastExtDir is clitool.RaycastExtDir (same path the CLI install writes).
// List cannot return an error, so a DirFor failure falls back to a display
// path — the install verb still fails closed if the home cannot be resolved.
func raycastExtDir() string {
	dir, err := clitool.RaycastExtDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return filepath.Join(config.DirName, clitool.RaycastExtDirName)
		}
		return filepath.Join(home, config.DirName, clitool.RaycastExtDirName)
	}
	return dir
}

// resolveGadak is LookPath("gadak"), then gadakFallbackPaths.
func resolveGadak(look func(string) (string, error), present func(string) bool) (string, bool) {
	return clitool.LookPathThen("gadak", gadakFallbackPaths, look, present)
}

// mcpNotRegisteredMarker is claude CLI's wording for a definitive negative
// (measured 2026-08-17: exit 1 + this text, ~1.4s). If the wording ever
// changes, the probe degrades to unknown — never to a false positive.
const mcpNotRegisteredMarker = "No MCP server named"

// probeOutcome says *why* a probe ended, which the *bool cannot: unknown has
// two causes and they are not interchangeable. A wrong answer is a product
// bug; running out of time is the machine being busy. Tests that assert which
// answer the CLI gives must be able to tell the difference, or a loaded
// machine turns into a red build about the wrong thing (GDK-303).
type probeOutcome int

const (
	probeAnswered   probeOutcome = iota // the process exited and we read it
	probeNotStarted                     // exec failed
	probeTimedOut                       // mcpProbeTimeout fired first
)

func (o probeOutcome) String() string {
	switch o {
	case probeAnswered:
		return "answered"
	case probeNotStarted:
		return "not-started"
	case probeTimedOut:
		return "timed-out"
	}
	return "unknown"
}

// probeClaudeMCP runs `claude mcp get gadak` with a short timeout.
// Exit 0 is installed=true; exit non-zero with the not-registered wording is
// a definitive installed=false; any other failure or timeout is unknown (nil).
// Mirrors cmd/gadak/mcp_install.go mcpInstallClaude's execLookPath("claude") rule
// for finding the binary; the get probe itself is desktop-status only.
func probeClaudeMCP(claudePath string) *bool {
	installed, _ := probeClaudeMCPOutcome(claudePath)
	return installed
}

// probeClaudeMCPOutcome is probeClaudeMCP plus the reason. Production reads
// only the answer; the outcome exists so a test can assert "the CLI said no"
// rather than "the CLI said no, or we gave up waiting".
//
// The timer starts after Start so a starved test goroutine cannot expire the
// budget before the process exists (CommandContext+WithTimeout before Start
// returned unknown for an `exit 0` stub under go test ./... load).
func probeClaudeMCPOutcome(claudePath string) (*bool, probeOutcome) {
	cmd := exec.Command(claudePath, "mcp", "get", "gadak")
	var out limitedBuf
	cmd.Stdout = &out
	cmd.Stderr = &out
	setProbeProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		return nil, probeNotStarted
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(mcpProbeTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			if strings.Contains(out.String(), mcpNotRegisteredMarker) {
				return boolPtr(false), probeAnswered
			}
			return nil, probeAnswered
		}
		return boolPtr(true), probeAnswered
	case <-timer.C:
		killProbe(cmd)
		// Bound the reap: if Kill did not take, do not pin GET behind a child.
		reap := time.NewTimer(200 * time.Millisecond)
		defer reap.Stop()
		select {
		case <-done:
		case <-reap.C:
		}
		return nil, probeTimedOut
	}
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	// Windows file modes are not POSIX execute bits; a regular file is enough.
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}

func boolPtr(v bool) *bool { return &v }

// limitedBuf keeps the first few KB of probe output for marker matching.
// Mutex-guarded: stdout and stderr share it, and after a timeout kill the
// probe returns while a straggling pipe copier may still write.
type limitedBuf struct {
	mu sync.Mutex
	b  []byte
}

func (l *limitedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	if len(l.b) < 8192 {
		room := min(8192-len(l.b), len(p))
		l.b = append(l.b, p[:room]...)
	}
	l.mu.Unlock()
	return len(p), nil
}

func (l *limitedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.b)
}
