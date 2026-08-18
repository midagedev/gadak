// Package integrations is the desktop Settings catalog: which local
// agent/host tools gadak can install, how to detect them, and which
// gadak CLI argv installs each one. Detection is files and local
// processes only — no network, no mirror.
package integrations

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
)

// IDs are part of the GET/POST contract. Order of List is fixed:
// command-line-tool, raycast (omitted on Windows), skill, mcp-claude.
const (
	IDCommandLineTool = "command-line-tool"
	IDRaycast         = "raycast"
	IDSkill           = "skill"
	IDMCPClaude       = "mcp-claude"
)

// mcpProbeTimeout is the production probe budget. Tests may assign a
// shorter value and restore it with t.Cleanup.
var mcpProbeTimeout = 3 * time.Second

// lookPath is exec.LookPath; tests inject a stub.
var lookPath = exec.LookPath

// fileIsExec reports a path exists and has an execute bit. Tests inject a stub.
var fileIsExec = isExecutable

// npmFallbackPaths mirrors cmd/gadak/raycast.go npmFallbackPaths:
// LookPath("npm") then these, first existing+executable wins.
var npmFallbackPaths = []string{
	"/opt/homebrew/bin/npm",
	"/usr/local/bin/npm",
}

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
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Installed    *bool         `json:"installed"`
	Detail       string        `json:"detail"`
	Command      string        `json:"command"`
	Prerequisite *Prerequisite `json:"prerequisite"`
}

// List returns the catalog rows for this host's GOOS, in contract order:
// command-line-tool, raycast (darwin/linux only), skill, mcp-claude.
// Windows omits raycast — Raycast does not exist there, and a row whose
// Install button can run would lie (GDK-244).
func List() []Item {
	return ListFor(runtime.GOOS)
}

// ListFor is List with an explicit GOOS so the Windows catalog can be
// pinned on any host (same shape as clitool.ResolveFor).
func ListFor(goos string) []Item {
	items := []Item{commandLineToolItem()}
	if raycastOffered(goos) {
		items = append(items, raycastItem())
	}
	items = append(items, skillItem(), mcpClaudeItem())
	return items
}

// raycastOffered is the single owner of "does this OS get a Raycast row".
// The install endpoint uses the same predicate via InstallArgsFor.
func raycastOffered(goos string) bool {
	return goos != "windows"
}

// InstallArgs is the argv tail for the bundled gadak CLI. ok is false for an
// unknown id, and for raycast on Windows.
func InstallArgs(id string) ([]string, bool) {
	return InstallArgsFor(id, runtime.GOOS)
}

// InstallArgsFor is InstallArgs with an explicit GOOS.
func InstallArgsFor(id, goos string) ([]string, bool) {
	switch id {
	case IDCommandLineTool:
		return []string{"install-cli"}, true
	case IDRaycast:
		if !raycastOffered(goos) {
			return nil, false
		}
		return []string{"raycast", "install"}, true
	case IDSkill:
		return []string{"skill", "install", "claude"}, true
	case IDMCPClaude:
		return []string{"mcp", "install", "claude"}, true
	default:
		return nil, false
	}
}

func commandLineToolItem() Item {
	path, ok := resolveGadak(lookPath, fileIsExec)
	if !ok {
		return Item{
			ID:           IDCommandLineTool,
			Title:        "Command line tool",
			Installed:    boolPtr(false),
			Detail:       "not on PATH",
			Command:      "gadak install-cli",
			Prerequisite: nil,
		}
	}
	return Item{
		ID:           IDCommandLineTool,
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
	if _, ok := resolveNPM(lookPath, fileIsExec); ok {
		prereq.OK = true
	} else {
		prereq.Message = "npm is required (not found on PATH)"
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
		ID:           IDRaycast,
		Title:        "Raycast extension",
		Installed:    boolPtr(installed),
		Detail:       detail,
		Command:      "gadak raycast install",
		Prerequisite: prereq,
	}
}

func skillItem() Item {
	dest := skillPath()
	return Item{
		ID:           IDSkill,
		Title:        "Claude Code skill",
		Installed:    boolPtr(fileExists(dest)),
		Detail:       clitool.TildeHome(dest),
		Command:      "gadak skill install claude",
		Prerequisite: nil,
	}
}

func mcpClaudeItem() Item {
	prereq := &Prerequisite{}
	path, err := lookPath("claude")
	if err != nil || path == "" {
		prereq.Message = "claude CLI is not on PATH"
		return Item{
			ID:           IDMCPClaude,
			Title:        "Claude Desktop MCP",
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
		ID:           IDMCPClaude,
		Title:        "Claude Desktop MCP",
		Installed:    installed,
		Detail:       detail,
		Command:      "gadak mcp install claude",
		Prerequisite: prereq,
	}
}

// raycastExtDir mirrors cmd/gadak/raycast.go raycastExtDir:
// $GADAK_HOME/raycast-extension, default ~/.gadak/raycast-extension.
func raycastExtDir() string {
	base, err := config.DirFor("")
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return filepath.Join(".gadak", "raycast-extension")
		}
		return filepath.Join(home, config.DirName, "raycast-extension")
	}
	return filepath.Join(base, "raycast-extension")
}

// skillPath mirrors cmd/gadak/skill.go resolveSkillDest default:
// ~/.claude/skills/gadak/SKILL.md
func skillPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return filepath.Join(".claude", "skills", "gadak", "SKILL.md")
		}
	}
	return filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md")
}

// resolveNPM mirrors cmd/gadak/raycast.go resolveNPM: PATH, then npmFallbackPaths.
func resolveNPM(look func(string) (string, error), present func(string) bool) (string, bool) {
	return resolveNamed("npm", npmFallbackPaths, look, present)
}

// resolveGadak is LookPath("gadak"), then gadakFallbackPaths.
func resolveGadak(look func(string) (string, error), present func(string) bool) (string, bool) {
	return resolveNamed("gadak", gadakFallbackPaths, look, present)
}

func resolveNamed(name string, fallbacks []string, look func(string) (string, error), present func(string) bool) (string, bool) {
	if look != nil {
		if p, err := look(name); err == nil && p != "" {
			return p, true
		}
	}
	if present == nil {
		present = isExecutable
	}
	for _, c := range fallbacks {
		if present(c) {
			return c, true
		}
	}
	return "", false
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
