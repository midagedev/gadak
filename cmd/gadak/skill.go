package main

// gadak skill install [client] — install the embedded Claude Code skill so
// agents that prefer skills over MCP get schema/query knowledge without a
// server process. Source is go:embed'd (gadak.SkillMarkdown); brew installs
// work without a repo checkout.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/clitool"
)

func printSkillHelp() {
	fmt.Print(`gadak skill — install the Claude Code skill (schema + query patterns)

Usage:
  gadak skill install [client] [--project] [--dir <path>] [--print] [--force]

Clients:
  claude   Claude Code skill (default). Installs SKILL.md under skills/gadak/

Other agents (cursor, codex, …) are not supported by this command yet —
use gadak mcp install <client>, or copy skills/gadak/SKILL.md from the repo
yourself.

Options:
  --project   install into ./.claude/skills/gadak/ (current working directory)
  --dir PATH  install into PATH/gadak/SKILL.md (overrides default and --project)
  --print     print the install plan without writing
  --force     overwrite a SKILL.md gadak did not write (hand-edited or your own)

Upgrades are not conflicts: when the file already there is a copy gadak
installed earlier, it is replaced in place and the command prints "updated:".
Only a file gadak did not write needs --force.

Install locations (when --dir is omitted):
  default     ~/.claude/skills/gadak/SKILL.md
  --project   .claude/skills/gadak/SKILL.md under the current directory

Examples:
  gadak skill install
  gadak skill install claude
  gadak skill install --print
  gadak skill install --project
  gadak skill install --dir /tmp/skills-preview --print
  gadak skill install --force

See also: gadak mcp install, docs/AGENT_SETUP.md, docs/MCP.md
`)
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
	project := false
	printOnly := false
	force := false
	dirFlag := ""
	var positionals []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printSkillHelp()
			return nil
		case a == "--project":
			project = true
		case a == "--print":
			printOnly = true
		case a == "--force":
			force = true
		case a == "--dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--dir requires a path\nrun \"gadak skill install --help\" for examples")
			}
			i++
			dirFlag = args[i]
		case strings.HasPrefix(a, "--dir="):
			dirFlag = strings.TrimPrefix(a, "--dir=")
			if dirFlag == "" {
				return fmt.Errorf("--dir requires a path\nrun \"gadak skill install --help\" for examples")
			}
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s\nrun \"gadak skill install --help\" for examples", a)
		default:
			positionals = append(positionals, a)
		}
	}

	client := "claude"
	if len(positionals) > 1 {
		return usageError("skill install", "usage: gadak skill install [client] [--project] [--dir <path>] [--print] [--force]")
	}
	if len(positionals) == 1 {
		client = strings.ToLower(positionals[0])
	}
	if client != "claude" {
		return fmt.Errorf("unknown client %q — only Claude Code skills are supported (client: claude)\n"+
			"for other agents use `gadak mcp install <client>`, or copy skills/gadak/SKILL.md yourself\n"+
			"run \"gadak skill install --help\" for examples", client)
	}

	dest, err := resolveSkillDest(project, dirFlag)
	if err != nil {
		return err
	}
	return installSkill(os.Stdout, gadak.SkillMarkdown(), dest, force, printOnly)
}

// resolveSkillDest picks the SKILL.md path.
// --dir wins (dir/gadak/SKILL.md); else --project → cwd/.claude/skills/gadak/;
// else ~/.claude/skills/gadak/SKILL.md.
func resolveSkillDest(project bool, dirFlag string) (string, error) {
	if dirFlag != "" {
		expanded, err := clitool.ExpandHome(dirFlag)
		if err != nil {
			return "", err
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", err
		}
		return filepath.Join(abs, "gadak", "SKILL.md"), nil
	}
	if project {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("current directory: %w", err)
		}
		return filepath.Join(cwd, ".claude", "skills", "gadak", "SKILL.md"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md"), nil
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
	// install — that difference is the whole point of GDK-92.
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

// skillDestStatus classifies dest relative to content.
//
//	missing    nothing at dest
//	identical  byte-equal to content — nothing to do
//	stale      gadak wrote it and it has since fallen behind: either it still
//	           matches the receipt gadak left beside it, or its digest is one of
//	           the copies gadak shipped before receipts existed
//	conflict   anything else — someone else's file, or gadak's file after a hand
//	           edit. Only --force replaces it.
//
// Identity is the content hash, never mtime: `brew upgrade` rewrites timestamps
// and `git checkout` restores them, so neither says who wrote the bytes.
func skillDestStatus(dest string, content []byte) (status string, existing []byte, err error) {
	fi, err := os.Lstat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", nil, nil
		}
		return "", nil, fmt.Errorf("inspect %s: %w", clitool.TildeHome(dest), err)
	}
	if fi.IsDir() {
		return "", nil, fmt.Errorf("%s is a directory — remove it or choose another --dir", clitool.TildeHome(dest))
	}
	existing, err = os.ReadFile(dest)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", clitool.TildeHome(dest), err)
	}
	if bytes.Equal(existing, content) {
		return "identical", existing, nil
	}
	if skillIsOurs(filepath.Dir(dest), existing) {
		return "stale", existing, nil
	}
	return "conflict", existing, nil
}

// ---------------------------------------------------------------------------
// Provenance: telling gadak's own copy from the user's file (GDK-92)
//
// Before this, "differs from the embedded skill" was read as "the user edited
// it", so every `brew upgrade` turned the documented one-liner red. The two
// cases are told apart by content hash:
//
//   1. a receipt gadak leaves beside SKILL.md on every install. If the file
//      still hashes to what the receipt says gadak wrote, gadak wrote it and
//      nobody has touched it since. This is the mechanism going forward — it
//      needs no per-release maintenance.
//   2. legacySkillDigests, for the installs that predate the receipt.
//
// A file that matches neither is the user's, and the refusal stands.
// ---------------------------------------------------------------------------

// skillReceiptName sits next to SKILL.md. It is a disposable cache, not data:
// delete it and the worst that happens is the next upgrade asks for --force.
const skillReceiptName = ".gadak-skill.json"

// skillReceipt records what gadak last wrote at a destination. Only SHA256 is
// compared; the other fields are there so a human who opens the file can tell
// what put it there and when.
type skillReceipt struct {
	SHA256       string `json:"sha256"`
	GadakVersion string `json:"gadak_version"`
	InstalledAt  string `json:"installed_at"`
}

// legacySkillDigests are the SHA-256 digests of every skills/gadak/SKILL.md
// gadak shipped *before* installs started leaving a receipt.
//
// This set is FROZEN — it is the backfill for pre-receipt installs only, and it
// must not grow. Every release from this one on writes skillReceiptName, so its
// own body is recognised by the receipt rather than by a new entry here. (An
// append-only table that a release could forget to update is exactly the bug
// this round is closing, so there is nothing to append to.)
//
// Derived 2026-08-16 from `git log --follow -- skills/gadak/SKILL.md`: seven
// revisions, of which the newest is the current embed and is deliberately
// absent — that one classifies as "identical", not "stale". The two oldest
// lived at skills/scry/SKILL.md, before the rename to gadak.
var legacySkillDigests = map[string]string{
	"37c489c4475984c1a9c33852828640c4833dda7f20d939063af6f944ccd40565": "79a70f3",
	"a00da5247df29926d88d4948f1ba16e36ea1c9cda1eb8728a2a9cc2d2ff1b594": "3d7a65b",
	"be5be92dfc76faed5a330dd905895efcbc6a433fc782fa566c6c1653956e9a32": "c7628ef",
	"5a6ca6f702ade9f91fa740f80b1600fe781508c82b17dfbee242b5f506d9b3ab": "eed711e",
	"5a6d63ae45af97344c0b91052ef59abbc763de815e277aa6a12bd2d8981f06fd": "1096106",
	"1f7000999eaebdeade1995b97373a083a3cc9f02673a8798020f463e3b7d27d8": "f2b8d94",
}

// skillIsOurs reports whether gadak wrote these exact bytes.
func skillIsOurs(dir string, existing []byte) bool {
	digest := skillDigest(existing)
	if r, ok := readSkillReceipt(dir); ok && r.SHA256 == digest {
		return true
	}
	_, ok := legacySkillDigests[digest]
	return ok
}

func skillDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// readSkillReceipt returns the receipt in dir. A missing, unreadable, corrupt
// or digest-less receipt is simply "no receipt" — it degrades to the legacy
// digest table and, failing that, to the refusal.
func readSkillReceipt(dir string) (skillReceipt, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, skillReceiptName))
	if err != nil {
		return skillReceipt{}, false
	}
	var r skillReceipt
	if err := json.Unmarshal(raw, &r); err != nil {
		return skillReceipt{}, false
	}
	if r.SHA256 == "" {
		return skillReceipt{}, false
	}
	return r, true
}

func writeSkillReceipt(dir, digest string) error {
	raw, err := json.MarshalIndent(skillReceipt{
		SHA256:       digest,
		GadakVersion: version,
		InstalledAt:  time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, skillReceiptName), append(raw, '\n'), 0o644)
}

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

// skillFrontmatterName returns the `name:` value of a leading YAML frontmatter
// block, or "". It is used only to sharpen the refusal message, never as a
// licence to overwrite: a user who edits gadak's own skill keeps `name: gadak`
// in it, so trusting that line would delete exactly the edits the refusal
// exists to protect.
func skillFrontmatterName(content []byte) string {
	const fence = "---\n"
	s := string(content)
	if !strings.HasPrefix(s, fence) {
		return ""
	}
	body := s[len(fence):]
	end := strings.Index(body, "\n---")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(body[:end], "\n") {
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
