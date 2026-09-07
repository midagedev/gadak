package main

// gadak skill install [client] — install the embedded agent skill so agents
// that prefer skills over MCP get schema/query knowledge without a server
// process. Source is go:embed'd (gadak.SkillMarkdown); brew installs work
// without a repo checkout.
//
// The bytes are the same for every host: Codex, Cursor, Gemini, OpenCode, grok
// and the .agents convention all read a SKILL.md directory the way Claude Code
// does, so the only thing a client selects is a path. internal/skillinstall
// owns that table.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/skillinstall"
)

// skillClientLines renders the client table for --help. It is generated from
// internal/skillinstall rather than typed out, so a new host cannot be
// installable and undocumented at the same time.
func skillClientLines() string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, c := range skillinstall.Clients() {
		suffix := ""
		if c.Name == skillinstall.DefaultClient {
			suffix = " (default)"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s%s\n", c.Name, c.Label, c.HomeDoc(), suffix)
	}
	_ = tw.Flush()
	return b.String()
}

func printSkillHelp() {
	fmt.Printf(`gadak skill — install the gadak agent skill (schema + query patterns)

Usage:
  gadak skill install [client] [--project] [--dir <path>] [--print] [--force]

Clients — the installed file is byte-identical for all of them; only the
path differs:
%s
Hosts that load an always-on rules file instead of a skill directory
(Copilot, Windsurf, Cline, Kiro, Amp) are not installable this way: use
gadak mcp install <client>, or paste the block from docs/AGENT_SETUP.md.

Options:
  --project   install under the current directory instead of the home
              directory: .claude/skills/gadak/ for claude, .agents/skills/gadak/
              for codex and agents. The other hosts read a rules file rather
              than a project skill directory and refuse --project.
  --dir PATH  install into PATH/gadak/SKILL.md (overrides default and --project)
  --print     print the install plan without writing
  --force     overwrite a SKILL.md gadak did not write (hand-edited or your own)

Upgrades are not conflicts: when the file already there is a copy gadak
installed earlier, it is replaced in place and the command prints "updated:".
Only a file gadak did not write needs --force.

Confirming it loaded: Claude Code lists it under /skills; Codex prints it
inside <skills_instructions> in "codex debug prompt-input".

Examples:
  gadak skill install
  gadak skill install codex
  gadak skill install codex --project
  gadak skill install --print
  gadak skill install --dir /tmp/skills-preview --print
  gadak skill install --force

See also: gadak mcp install, gadak doctor, docs/AGENT_SETUP.md, docs/MCP.md
`, skillClientLines())
}

func cmdSkill(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		printSkillHelp()
		return nil
	}
	if args[0] == "install" {
		return cmdSkillInstall(args[1:])
	}
	return usageError("skill", "usage: gadak skill install [client] [--project] [--dir <path>] [--print] [--force]")
}

func cmdSkillInstall(args []string) error {
	fs := newFlagSet("skill install")
	project := fs.Bool("project", false, "install under the current directory instead of the home directory")
	dirFlag := fs.String("dir", "", "install into PATH/gadak/SKILL.md (overrides default and --project)")
	printOnly := fs.Bool("print", false, "print the install plan without writing")
	force := fs.Bool("force", false, "overwrite when the existing file differs from the embedded skill")
	if wantsHelp(args) {
		printSkillHelp()
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}

	name := skillinstall.DefaultClient
	if len(pos) > 1 {
		return usageError("skill install", "usage: gadak skill install [client] [--project] [--dir <path>] [--print] [--force]")
	}
	if len(pos) == 1 {
		name = strings.ToLower(strings.TrimSpace(pos[0]))
	}
	client, ok := skillinstall.Lookup(name)
	if !ok {
		return skillinstall.UnknownClientError(name)
	}

	dest, err := resolveSkillDestFor(client, *project, *dirFlag)
	if err != nil {
		return err
	}
	if *printOnly {
		// Which host this plan is for, before the paths — with seven clients
		// the destination alone no longer says it.
		fmt.Printf("client:  %s (%s)\n", client.Name, client.Label)
	}
	return installSkill(os.Stdout, gadak.SkillMarkdown(), dest, *force, *printOnly)
}

// resolveSkillDestFor picks the SKILL.md path for one host.
// --dir wins (dir/gadak/SKILL.md); else --project → the host's project skills
// root under the working directory; else the host's user-scope root.
func resolveSkillDestFor(client skillinstall.Client, project bool, dirFlag string) (string, error) {
	if dirFlag != "" {
		return skillinstall.DirDest(dirFlag)
	}
	return client.Dest(skillinstall.OSEnv(), project)
}

// resolveSkillDest is the Claude Code destination — the default client, and
// what auto-install, the daily sync and install-cli all mean when they say
// "the skill".
func resolveSkillDest(project bool, dirFlag string) (string, error) {
	client, ok := skillinstall.Lookup(skillinstall.DefaultClient)
	if !ok {
		return "", fmt.Errorf("internal: no %q client in the skill table", skillinstall.DefaultClient)
	}
	return resolveSkillDestFor(client, project, dirFlag)
}

// installSkill writes content to dest (or plans with printOnly).
//
//	identical  nothing to do (exit 0)
//	stale      a copy gadak wrote, now behind → overwrite, print "updated:"
//	conflict   a file gadak did not write → refuse unless force
//	missing    → install
func installSkill(w io.Writer, content []byte, dest string, force, printOnly bool) error {
	if len(content) == 0 {
		return fmt.Errorf("embedded skill is empty — this binary was built without skills/gadak/SKILL.md")
	}

	status, existing, err := skillDestStatus(dest, content)
	if err != nil {
		return err
	}

	if printOnly {
		fmt.Fprintf(w, "source:  embedded skills/gadak/SKILL.md\n")
		fmt.Fprintf(w, "dest:    %s\n", clitool.TildeHome(dest))
		switch status {
		case "missing":
			fmt.Fprintf(w, "status:  missing (would install)\n")
		case "identical":
			fmt.Fprintf(w, "status:  already installed (identical)\n")
		case "stale":
			fmt.Fprintf(w, "status:  stale (gadak installed it; would update)\n")
		case "conflict":
			fmt.Fprintf(w, "status:  differs (use --force to overwrite)\n")
		}
		return nil
	}

	// verb is what the user is told happened. An upgrade of our own copy is an
	// "updated:", so the log of a brew upgrade reads differently from a first
	// install (GDK-92).
	verb := "installed"
	switch status {
	case "identical":
		fmt.Fprintf(w, "already installed: %s\n", clitool.TildeHome(dest))
		fmt.Fprintf(w, "next: restart the agent or open a new session so it picks up the skill\n")
		return nil
	case "stale":
		verb = "updated"
	case "conflict":
		if !force {
			return errSkillConflict(dest, existing)
		}
	case "missing":
		// install below
	default:
		return fmt.Errorf("internal: unknown skill dest status %q", status)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", clitool.TildeHome(dir), err)
	}
	// Write via temp + rename so a partial write never leaves a corrupt skill.
	tmp, err := os.CreateTemp(dir, "SKILL.md.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", clitool.TildeHome(dir), err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we fail before rename.
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", clitool.TildeHome(dest), err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod %s: %w", clitool.TildeHome(tmpName), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install %s: %w", clitool.TildeHome(dest), err)
	}
	// The receipt is what makes the *next* upgrade a no-question overwrite.
	// Failing to write it costs nothing today, so it never fails the install.
	if err := writeSkillReceipt(dir, skillDigest(content)); err != nil {
		fmt.Fprintf(w, "note: could not record the install receipt in %s — the next upgrade may ask for --force\n",
			clitool.TildeHome(dir))
	}

	fmt.Fprintf(w, "%s: %s\n", verb, clitool.TildeHome(dest))
	fmt.Fprintf(w, "next: restart the agent or open a new session so it picks up the skill\n")
	return nil
}

// The provenance classifier, the receipt and the legacy digest table moved to
// internal/skillinstall (GDK-1508) so `gadak doctor`, the installer and the
// desktop app's integrations list can all reach one owner. These aliases keep
// the CLI's own call sites reading the way they did.
type skillReceipt = skillinstall.Receipt

var legacySkillDigests = skillinstall.LegacyDigests

func skillDestStatus(dest string, content []byte) (string, []byte, error) {
	return skillinstall.DestStatus(dest, content)
}

func skillDigest(content []byte) string { return skillinstall.Digest(content) }

func readSkillReceipt(dir string) (skillReceipt, bool) { return skillinstall.ReadReceipt(dir) }

func writeSkillReceipt(dir, digest string) error {
	return skillinstall.WriteReceipt(dir, digest, version)
}

func skillFrontmatterName(content []byte) string { return skillinstall.FrontmatterName(content) }

// skillConflictError is the --force refusal. The message is unchanged from
// the previous errors.New form so `gadak skill install` output stays the same;
// the type lets auto-install detect a conflict without matching the copy.
type skillConflictError struct {
	msg string
}

func (e *skillConflictError) Error() string { return e.msg }

// errSkillConflict refuses a file gadak did not write. When the file carries
// `name: gadak` the message says so, because that is the confusing case: the
// frontmatter says "gadak" but the bytes are not any skill gadak shipped, which
// means someone edited it — and their edit is what --force would destroy.
func errSkillConflict(dest string, existing []byte) error {
	msg := fmt.Sprintf("%s exists and differs from the embedded skill — re-run with --force to overwrite (your edits will be lost)",
		clitool.TildeHome(dest))
	if skillFrontmatterName(existing) == "gadak" {
		msg += "\nit declares `name: gadak` but its contents match no skill gadak shipped, so it is treated as your edit"
	}
	return &skillConflictError{msg: msg}
}

// claudeDirExists reports whether ~/.claude is a directory. That is the
// signal that Claude Code is on this machine; auto-install does not create
// it.
func claudeDirExists() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	st, err := os.Stat(filepath.Join(home, ".claude"))
	return err == nil && st.IsDir()
}

// autoInstallSkill writes the embedded Claude Code skill into the user-level
// dest when ~/.claude already exists. force is always false: a file gadak
// did not write is left in place. The return is one of "installed",
// "skipped", "failed"; it never becomes the caller's exit status.
//
// w receives warnings (conflict --force hint, I/O failures). Callers pass
// os.Stderr so --json stdout stays a single object.
func autoInstallSkill(w io.Writer) string {
	if w == nil {
		w = io.Discard
	}
	if !claudeDirExists() {
		return "skipped"
	}
	dest, err := resolveSkillDest(false, "")
	if err != nil {
		fmt.Fprintf(w, "warning: skill auto-install: %v\n", err)
		return "failed"
	}
	err = installSkill(io.Discard, gadak.SkillMarkdown(), dest, false, false)
	if err == nil {
		return "installed"
	}
	var conflict *skillConflictError
	if errors.As(err, &conflict) {
		fmt.Fprintf(w, "skill: %s exists and differs from the embedded skill — run gadak skill install --force to overwrite\n",
			clitool.TildeHome(dest))
		return "skipped"
	}
	fmt.Fprintf(w, "warning: skill auto-install failed: %v\n", err)
	return "failed"
}

// printSkillAutoResult is the one human line init prints after a successful
// save. JSON callers skip this and put the same token in the document.
func printSkillAutoResult(status string) {
	if status == "installed" {
		dest, err := resolveSkillDest(false, "")
		if err != nil {
			fmt.Printf("skill: installed\n")
			return
		}
		fmt.Printf("skill: installed %s\n", clitool.TildeHome(dest))
		return
	}
	fmt.Printf("skill: %s\n", status)
}

// ---------------------------------------------------------------------------
// Daily auto-sync: the installed copy follows the binary (GDK-996)
//
// Every layer that receives a tag refreshes itself except the agent contract:
// a skill installed by v0.16 kept teaching v0.16 verbs against a v0.18 binary
// until the user re-ran `gadak skill install`. main() now gives every
// subcommand one refresh chance per day — same move as fzf serving its shell
// integration from the binary (`eval "$(fzf --bash)"`), closing the skew
// between the artifact and the binary that owns it.
//
// The boundaries are the installer's classifier, unchanged and reused:
// `conflict` (a copy gadak did not write — the issue's "foreign") and
// `missing` (no skill installed) are never touched here. Creating a skill
// uninvited is init / `skill install`'s job. A fourth boundary is the binary
// rather than the file: a dev build never syncs (GDK-1531, below).
// ---------------------------------------------------------------------------

// skillAutoSyncStampName is the once-a-day rate-limit stamp, JSON like the
// install receipt. It lives at the gadak home root (config.DirFor("")), not
// under a profile directory: the skill is user-scoped, so the rate limit must
// not reset per workspace.
const skillAutoSyncStampName = "skill-autosync.json"

// skillAutoSyncStamp records when the daily check last ran, so the common
// case — already checked today — costs one small-file read and no skill I/O.
type skillAutoSyncStamp struct {
	LastCheck string `json:"last_check"` // RFC3339, UTC
}

// skillAutoSyncSkip names the commands the hook never runs for:
//
//	skill  its subcommands own the copy explicitly; the install verb is the
//	       one that must write it
//	mcp    the stdio JSON-RPC server — stdout is protocol and stderr sits
//	       right beside it in the host's log, so no surprise startup lines
var skillAutoSyncSkip = map[string]bool{
	"skill": true,
	"mcp":   true,
}

func skillAutoSyncStampPath() (string, error) {
	dir, err := config.DirFor("")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, skillAutoSyncStampName), nil
}

// lastSkillAutoCheck returns the stamp's last_check, or "" for a missing,
// unreadable or corrupt stamp — which simply means "check now".
func lastSkillAutoCheck() string {
	p, err := skillAutoSyncStampPath()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var s skillAutoSyncStamp
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s.LastCheck
}

func writeSkillAutoSyncStamp(path string, now time.Time) {
	raw, err := json.MarshalIndent(skillAutoSyncStamp{LastCheck: now.Format(time.RFC3339)}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(raw, '\n'), 0o600)
}

// skillAutoSyncCheckedToday reports whether stamp (RFC3339) falls on the same
// UTC day as now. An unparsable stamp never rate-limits. A stamp in the
// future (clock set back) also fails the date compare, so the check re-runs
// and rewrites it — the stamp self-heals.
func skillAutoSyncCheckedToday(stamp string, now time.Time) bool {
	at, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return false
	}
	y1, m1, d1 := at.UTC().Date()
	y2, m2, d2 := now.UTC().Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// maybeAutoSyncSkill is the once-a-day hook main() puts on the dispatch path.
// It never returns an error and never prints unless it actually updated the
// copy: any failure is swallowed (a read-only gadak home is a supported
// state, applog.Install says so too), so the subcommand's own exit status is
// unaffected either way.
func maybeAutoSyncSkill(w io.Writer, cmd string) {
	if w == nil {
		w = io.Discard
	}
	if skillAutoSyncSkip[cmd] {
		return
	}
	now := time.Now().UTC()
	if skillAutoSyncCheckedToday(lastSkillAutoCheck(), now) {
		return
	}
	stampPath, err := skillAutoSyncStampPath()
	if err != nil {
		return
	}
	// Mark the day before touching the skill: a write that then fails
	// retries tomorrow, not on every command.
	writeSkillAutoSyncStamp(stampPath, now)

	dest, err := resolveSkillDest(false, "")
	if err != nil {
		return
	}
	content := gadak.SkillMarkdown()
	status, _, err := skillDestStatus(dest, content)
	if err != nil || status != "stale" {
		return
	}
	// GDK-1531: a binary built from a checkout must not push its working-tree
	// SKILL.md into a real agent home. Measured 2026-09-07: a round running
	// `go run ./cmd/gadak …` replaced the developer's installed skill with an
	// uncommitted draft. The rate-limit stamp did not stop it, and could not:
	// the stamp lives under GADAK_HOME while the destination lives under HOME,
	// so any probe that isolates one and not the other hands the hook a fresh
	// day. The version is what actually separates "gadak shipped this" from
	// "someone is editing this right now", so that is what the hook asks.
	//
	// The line prints here rather than at the top of the function so it costs
	// a day's stamp and appears only when a sync would really have happened —
	// once a day at most, and never for a machine with no skill installed.
	if skillinstall.IsDevBuild(version) {
		fmt.Fprintf(w, "skill: dev build — not syncing %s (run gadak skill install to do it on purpose)\n",
			clitool.TildeHome(filepath.Dir(dest)))
		return
	}
	// installSkill reuses the same classifier, the same atomic write and the
	// same receipt, so what lands is byte-identical to `gadak skill install`
	// output and the next classifier still recognises gadak's own copy.
	if err := installSkill(io.Discard, content, dest, false, false); err != nil {
		return
	}
	fmt.Fprintf(w, "skill: updated %s (restart the agent to load it)\n", clitool.TildeHome(dest))
}
