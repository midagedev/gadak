// Package skillinstall owns two questions about gadak's agent skill:
// *where does it go for this host*, and *is the copy already there gadak's own*.
//
// It sits outside package main because more than one caller needs the answers.
// `gadak skill install` and `gadak doctor` live in cmd/gadak; the desktop app's
// integrations list (internal/integrations) cannot import package main at all,
// and today it re-derives the Claude path by hand with a comment apologising
// for the duplication. One table, one classifier, one owner.
//
// The bytes never change per host. Codex loads gadak's SKILL.md verbatim — the
// investigation for GDK-1508 copied it in byte-for-byte and read it back out of
// `codex debug prompt-input` — so the "renderer" for every skill-capable host
// is the identity function and a destination path. Hosts that need a different
// *format* (Copilot, Windsurf, Cline, Kiro, Amp: an always-loaded rules file,
// not a skill directory) are deliberately not in this table.
package skillinstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/gadak/internal/clitool"
)

const (
	// SkillDir is the folder gadak occupies inside a host's skills root, and
	// SkillFile the file inside it. Every host in the table reads the same two
	// names, which is why a single `--dir PATH` means PATH/gadak/SKILL.md
	// regardless of who is going to load it.
	SkillDir  = "gadak"
	SkillFile = "SKILL.md"

	// skillsRootName is the directory each host scans for skill folders.
	skillsRootName = "skills"
)

// Env is the process environment the destination table reads: the home
// directory, the working directory, and the environment lookup.
//
// It is a parameter and not package state on purpose. Tests build an Env
// literal, so nothing in this package can ever resolve to the developer's real
// home — and in particular no test has to set CODEX_HOME to stay contained.
// That matters: orca's tripwire records that the Codex binary ignores the
// USERPROFILE sandbox on Windows, so CODEX_HOME is not a containment
// mechanism. Here it is read as a *user preference*, never trusted as a fence.
type Env struct {
	// Home is the user's home directory (os.UserHomeDir).
	Home string
	// Cwd is the working directory; only project scope reads it.
	Cwd string
	// Getenv looks up an environment variable. A nil Getenv reads nothing,
	// which is the right default for a hermetic test.
	Getenv func(key string) string

	// homeErr and cwdErr carry the lookup failures so the caller that actually
	// needs that directory reports the original cause — a --project install
	// must not fail because the home directory is unreadable, and the reverse.
	// Unexported: an Env built by a test has no such failure.
	homeErr error
	cwdErr  error
}

// OSEnv reads the real process environment. It never fails here: a missing
// home or working directory surfaces at the destination that needed it.
func OSEnv() Env {
	home, homeErr := os.UserHomeDir()
	cwd, cwdErr := os.Getwd()
	return Env{Home: home, Cwd: cwd, Getenv: os.Getenv, homeErr: homeErr, cwdErr: cwdErr}
}

func (e Env) getenv(key string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(key)
}

func (e Env) homeJoin(parts ...string) (string, error) {
	if e.homeErr != nil {
		return "", fmt.Errorf("home directory: %w", e.homeErr)
	}
	if e.Home == "" {
		return "", errors.New("home directory: not available")
	}
	return filepath.Join(append([]string{e.Home}, parts...)...), nil
}

// Client is one agent host that loads a skill from a directory of SKILL.md
// files. Everything host-specific is a path; the content is not.
type Client struct {
	// Name is the CLI token (`gadak skill install <name>`).
	Name string
	// Label is the host's own name, for messages and docs.
	Label string
	// CardLabel is the host's name as a card title — Label without the
	// parenthetical qualifiers that make a narrow card wrap (GDK-1534). Same
	// value as Label for every host whose label has no parenthetical; the
	// table test keeps the two honest against each other.
	CardLabel string
	// Universal marks the one row the desktop integrations list always offers,
	// whether or not the host looks installed: the cross-host .agents
	// convention is a directory any agentskills.io reader loads, so absence
	// of the host is not a reason to hide the install (GDK-1534). Exactly one
	// client may set this — the table test pins which.
	Universal bool

	// configDir returns the host's configuration root — the directory whose
	// existence means "this host is on this machine".
	configDir func(Env) (string, error)
	// projectRel is the skills root relative to the working directory, or nil
	// when this host has no skill-directory project scope.
	projectRel []string
	// noProjectWhy is the one-line refusal for a host without project scope.
	noProjectWhy string

	// homeDoc is how the user-scope directory is written in help and docs —
	// with the ~ and the environment variable left in, because that is what the
	// reader has to recognise on their own machine. It is a caption for
	// configDir, not a second source of truth: the table test walks every
	// client and asserts the two resolve to the same place.
	homeDoc string
}

// clients is the table, in the order gadak lists and scans hosts. Claude is
// first because it is the default client and the only one before GDK-1508.
//
// Sources: `$CODEX_HOME/skills`, `~/.agents/skills` and `<cwd>/.agents/skills`
// were measured working against codex-cli 0.147.0 with `codex debug
// prompt-input`. The remaining home roots are orca's discovery table
// (src/main/skills/skill-discovery-sources.ts), cross-checked against
// ponytail's adapter list. Anything uncertain is left out rather than guessed —
// a wrong path installs a skill nobody loads and reports success.
var clients = []Client{
	{
		Name:      "claude",
		homeDoc:   "~/.claude/skills/gadak/",
		Label:     "Claude Code",
		CardLabel: "Claude Code",
		configDir: func(e Env) (string, error) {
			return e.homeJoin(".claude")
		},
		// Claude Code's project scope predates this table and stays as it was.
		projectRel: []string{".claude", skillsRootName},
	},
	{
		Name:      "codex",
		homeDoc:   "$CODEX_HOME/skills/gadak/ (default ~/.codex/skills/gadak/)",
		Label:     "Codex CLI",
		CardLabel: "Codex CLI",
		configDir: func(e Env) (string, error) {
			// Codex's own installer writes under $CODEX_HOME, and the binary
			// reads it, even though the published docs describe only
			// .agents/skills. Honour the user's override first, then the
			// default home.
			if v := strings.TrimSpace(e.getenv("CODEX_HOME")); v != "" {
				return absUserPath(v)
			}
			return e.homeJoin(".codex")
		},
		// .agents/skills is the cross-host repo convention Codex walks up to
		// find; it is also what agentskills.io documents. Codex reads it with
		// or without a git repository.
		projectRel: []string{".agents", skillsRootName},
	},
	{
		Name:    "agents",
		homeDoc: "~/.agents/skills/gadak/",
		Label:   "agentskills.io hosts (.agents)",
		// The card title drops the parenthetical: the row already shows the
		// path it installs to, so the qualifier only wraps.
		CardLabel: "agentskills.io hosts",
		// No single host owns ~/.agents — every agentskills.io reader walks it
		// — so the integrations list offers this row even on a machine where
		// none of them looks installed (GDK-1534).
		Universal: true,
		configDir: func(e Env) (string, error) {
			return e.homeJoin(".agents")
		},
		projectRel: []string{".agents", skillsRootName},
	},
	{
		Name:      "cursor",
		homeDoc:   "~/.cursor/skills/gadak/",
		Label:     "Cursor",
		CardLabel: "Cursor",
		configDir: func(e Env) (string, error) {
			return e.homeJoin(".cursor")
		},
		noProjectWhy: "Cursor's project scope is a rules file (.cursor/rules/gadak.mdc), not a skill directory",
	},
	{
		Name:      "gemini",
		homeDoc:   "~/.gemini/skills/gadak/",
		Label:     "Gemini CLI",
		CardLabel: "Gemini CLI",
		configDir: func(e Env) (string, error) {
			return e.homeJoin(".gemini")
		},
		noProjectWhy: "Gemini's project scope is an extension manifest (gemini-extension.json), not a skill directory",
	},
	{
		Name:      "opencode",
		homeDoc:   "~/.config/opencode/skills/gadak/",
		Label:     "OpenCode",
		CardLabel: "OpenCode",
		configDir: func(e Env) (string, error) {
			return e.homeJoin(".config", "opencode")
		},
		noProjectWhy: "OpenCode's project scope is a plugin under .opencode/, not a skill directory",
	},
	{
		Name:      "grok",
		homeDoc:   "~/.grok/skills/gadak/",
		Label:     "grok CLI",
		CardLabel: "grok CLI",
		configDir: func(e Env) (string, error) {
			return e.homeJoin(".grok")
		},
		noProjectWhy: "grok has no measured project skill directory",
	},
}

// DefaultClient is the client `gadak skill install` uses when none is named.
// It was the only client before GDK-1508 and stays the default.
const DefaultClient = "claude"

// Clients returns the table in listing order.
func Clients() []Client {
	out := make([]Client, len(clients))
	copy(out, clients)
	return out
}

// Names returns the client tokens in listing order.
func Names() []string {
	out := make([]string, 0, len(clients))
	for _, c := range clients {
		out = append(out, c.Name)
	}
	return out
}

// Lookup finds a client by token, case-insensitively.
func Lookup(name string) (Client, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, c := range clients {
		if c.Name == want {
			return c, true
		}
	}
	return Client{}, false
}

// UnknownClientError is the refusal for a name outside the table. It lists the
// tokens that do work, because "unknown client" without the list is one more
// round trip for a user who is already guessing.
func UnknownClientError(name string) error {
	return fmt.Errorf("unknown client %q — supported: %s\n"+
		"hosts that load an always-on rules file instead of a skill (Copilot, Windsurf, Cline, Kiro, Amp) are not installable this way; use `gadak mcp install <client>` or copy skills/gadak/SKILL.md yourself\n"+
		"run \"gadak skill install --help\" for examples", name, strings.Join(Names(), ", "))
}

// ConfigDir is the host's configuration root. Its existence, not a binary on
// PATH, is the signal that the host is on this machine: several of these hosts
// ship as IDE extensions or apps with no command of their own.
func (c Client) ConfigDir(env Env) (string, error) {
	return c.configDir(env)
}

// HomeDest is the user-scope SKILL.md path for this host.
func (c Client) HomeDest(env Env) (string, error) {
	dir, err := c.ConfigDir(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, skillsRootName, SkillDir, SkillFile), nil
}

// HasProjectScope reports whether this host reads a skill directory from the
// working directory.
func (c Client) HasProjectScope() bool { return c.projectRel != nil }

// ProjectDest is the working-directory SKILL.md path, or the refusal for a
// host whose project scope is a different file format.
func (c Client) ProjectDest(env Env) (string, error) {
	if !c.HasProjectScope() {
		return "", c.ProjectRefusal()
	}
	if env.cwdErr != nil {
		return "", fmt.Errorf("current directory: %w", env.cwdErr)
	}
	if env.Cwd == "" {
		return "", errors.New("current directory: not available")
	}
	parts := append([]string{env.Cwd}, c.projectRel...)
	parts = append(parts, SkillDir, SkillFile)
	return filepath.Join(parts...), nil
}

// ProjectRelDir is the project skills root as the user typed it — the literal
// relative path, with no working directory in it. `gadak doctor` prints this
// rather than an absolute path so its report stays safe to paste in public.
func (c Client) ProjectRelDir() string {
	if !c.HasProjectScope() {
		return ""
	}
	return filepath.Join(append(append([]string{}, c.projectRel...), SkillDir, SkillFile)...)
}

// ProjectRefusal explains, in one line, why --project does nothing for this
// host. Naming the format is the point: the user is not being told "no", they
// are being told the file they want is a different kind of file.
func (c Client) ProjectRefusal() error {
	why := c.noProjectWhy
	if why == "" {
		why = "this host has no project skill directory"
	}
	return fmt.Errorf("--project is not supported for %s — %s\n"+
		"install it at the user scope (`gadak skill install %s`), or write the file yourself with --dir",
		c.Name, why, c.Name)
}

// HomeDoc is the user-scope directory as help and documentation write it.
func (c Client) HomeDoc() string { return c.homeDoc }

// ProjectDoc is the project-scope directory as documentation writes it, or ""
// for a host that has none.
func (c Client) ProjectDoc() string {
	if !c.HasProjectScope() {
		return ""
	}
	return filepath.Join(append(append([]string{}, c.projectRel...), SkillDir)...) + string(filepath.Separator)
}

// NoProjectWhy is the one-line reason this host has no project skill
// directory, or "" when it has one.
func (c Client) NoProjectWhy() string { return c.noProjectWhy }

// Dest is HomeDest or ProjectDest.
func (c Client) Dest(env Env, project bool) (string, error) {
	if project {
		return c.ProjectDest(env)
	}
	return c.HomeDest(env)
}

// Present reports whether this host looks installed: its configuration root
// exists and holds something the operating system did not write.
//
// The second half is orca's lesson, ported. A single Finder visit leaves a
// .DS_Store behind, and a directory that contains nothing else is not evidence
// of anything — reporting a host on that basis puts a row in `gadak doctor`
// that the user cannot act on. Note this is deliberately stricter than the
// plain existence check that gates *auto-install*: writing into a directory the
// user made is fine, listing a host we cannot justify is not.
func (c Client) Present(env Env) bool {
	dir, err := c.ConfigDir(env)
	if err != nil {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !IsOSMetadata(e.Name()) {
			return true
		}
	}
	return false
}

// IsOSMetadata reports whether name is a file the operating system authored
// rather than the user or gadak: Finder's .DS_Store, Windows Explorer's
// thumbs.db and desktop.ini, and the ._ AppleDouble sidecars a copy onto a
// non-HFS volume leaves behind.
//
// Why this exists: orca compares a whole skill directory against a manifest,
// and one Finder visit was enough to make an untouched copy read as
// "unrecognized" — the user was asked to --force over their own unmodified
// files. gadak's identity is a single file's hash, so no current judgment can
// be broken this way; this is the guard that keeps it true when a caller does
// start reading directory contents. Present() is the first such caller.
func IsOSMetadata(name string) bool {
	switch strings.ToLower(name) {
	case ".ds_store", "thumbs.db", "desktop.ini":
		return true
	}
	return strings.HasPrefix(name, "._")
}

// DirDest is the `--dir PATH` destination: PATH/gadak/SKILL.md. It overrides
// every host in the table, which is the honest seam for a test or for a host
// gadak has not measured.
func DirDest(dir string) (string, error) {
	abs, err := absUserPath(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, SkillDir, SkillFile), nil
}

// absUserPath expands a leading ~ and makes the path absolute.
func absUserPath(p string) (string, error) {
	expanded, err := clitool.ExpandHome(p)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}
