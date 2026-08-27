package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	gadak "github.com/midagedev/gadak"
)

// gadakTestOrigHome is the developer/CI home TestMain replaced. The auto-sync
// tests build the CLI with `go build`; go's caches live under the real home,
// so that build subprocess gets the original back.
var gadakTestOrigHome string

// TestMain isolates HOME for the whole cmd/gadak package. cmdInit and
// installCLI now write into ~/.claude when that directory exists; without
// this, a developer machine that already has Claude Code would have its
// real skill rewritten by unrelated tests. Individual tests that need a
// specific home still t.Setenv HOME (and USERPROFILE) themselves.
func TestMain(m *testing.M) {
	gadakTestOrigHome = os.Getenv("HOME")
	dir, err := os.MkdirTemp("", "gadak-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "test home: %v\n", err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", dir)
	_ = os.Setenv("USERPROFILE", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	removeGadakTestBin()
	os.Exit(code)
}

// isolateHome points os.UserHomeDir at a fresh temp directory with no
// ~/.claude, so auto-install is a skip.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// isolateHomeWithClaude is isolateHome plus the directory that means
// "Claude Code is on this machine".
func isolateHomeWithClaude(t *testing.T) string {
	t.Helper()
	home := isolateHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func skillDestUnder(home string) string {
	return filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md")
}

func TestSkillInstallNewMatchesEmbedded(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "gadak", "SKILL.md")
	content := gadak.SkillMarkdown()
	if len(content) == 0 {
		t.Fatal("embedded skill empty")
	}

	var buf bytes.Buffer
	if err := installSkill(&buf, content, dest, false, false); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v\nout:\n%s", err, buf.String())
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("installed bytes differ from embed (%d vs %d)", len(got), len(content))
	}
	out := buf.String()
	if !strings.Contains(out, "installed:") {
		t.Errorf("expected installed: line, got:\n%s", out)
	}
	if !strings.Contains(out, "next:") {
		t.Errorf("expected next: line, got:\n%s", out)
	}
	// Mode 0644 (umask may clear write bits for group/other; at least user-readable).
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o400 == 0 {
		t.Errorf("dest not readable: mode %o", fi.Mode().Perm())
	}
}

func TestSkillInstallAlreadyInstalled(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "gadak", "SKILL.md")
	content := gadak.SkillMarkdown()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := installSkill(&buf, content, dest, false, false); err != nil {
		t.Fatalf("re-install: %v", err)
	}
	after, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("expected no-op (same mtime); before=%v after=%v", before.ModTime(), after.ModTime())
	}
	if !strings.Contains(buf.String(), "already installed") {
		t.Errorf("expected already installed, got:\n%s", buf.String())
	}
}

func TestSkillInstallConflictAndForce(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "gadak", "SKILL.md")
	content := gadak.SkillMarkdown()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("user-edited skill body\n")
	if err := os.WriteFile(dest, old, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := installSkill(&buf, content, dest, false, false)
	if err == nil {
		t.Fatal("expected error when dest differs")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force: %v", err)
	}
	raw, _ := os.ReadFile(dest)
	if !bytes.Equal(raw, old) {
		t.Errorf("dest mutated without --force")
	}

	buf.Reset()
	if err := installSkill(&buf, content, dest, true, false); err != nil {
		t.Fatalf("--force: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("after force, bytes differ from embed")
	}
	if !strings.Contains(buf.String(), "installed:") {
		t.Errorf("force output:\n%s", buf.String())
	}
}

// TestSkillInstallUpgradesOurOwnCopy — GDK-92. After `brew upgrade` the file at
// the destination is the previous release's embedded skill, so it differs, so
// the documented one-liner used to fail until the user passed --force. An
// upgrade of gadak's own copy is not a conflict.
func TestSkillInstallUpgradesOurOwnCopy(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "gadak", "SKILL.md")

	// Stand in for the previous release: install its body through the same
	// writer, so the on-disk state is exactly what an older gadak left behind
	// (file plus whatever bookkeeping that release wrote next to it).
	prev := []byte("---\nname: gadak\ndescription: the previous release's skill\n---\n\n# older body\n")
	var buf bytes.Buffer
	if err := installSkill(&buf, prev, dest, false, false); err != nil {
		t.Fatalf("seed previous release: %v", err)
	}

	buf.Reset()
	if err := installSkill(&buf, gadak.SkillMarkdown(), dest, false, false); err != nil {
		t.Fatalf("upgrading gadak's own copy must not need --force: %v\nout:\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "updated:") {
		t.Errorf("expected an updated: line so the user sees what happened, got:\n%s", buf.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Errorf("after upgrade the file is not the embedded skill (%d vs %d bytes)", len(got), len(gadak.SkillMarkdown()))
	}
}

// TestSkillInstallUpgradesPreReceiptCopy — GDK-92. The receipt only exists from
// this release on; a user upgrading from an older gadak has a bare SKILL.md and
// nothing beside it. legacySkillDigests is what recognises that file, and this
// exercises that path specifically (the seed writes no receipt).
func TestSkillInstallUpgradesPreReceiptCopy(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "gadak", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	// Exactly what an older release left on disk: the file, no bookkeeping.
	shipped := []byte("---\nname: gadak\n---\n\n# a body some earlier gadak shipped\n")
	if err := os.WriteFile(dest, shipped, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := skillDigest(shipped)
	legacySkillDigests[digest] = "test-fixture"
	t.Cleanup(func() { delete(legacySkillDigests, digest) })

	var buf bytes.Buffer
	if err := installSkill(&buf, gadak.SkillMarkdown(), dest, false, false); err != nil {
		t.Fatalf("pre-receipt copy must upgrade without --force: %v", err)
	}
	if !strings.Contains(buf.String(), "updated:") {
		t.Errorf("expected updated:, got:\n%s", buf.String())
	}
	// And the upgrade leaves a receipt, so the next one needs no ledger entry.
	if r, ok := readSkillReceipt(filepath.Dir(dest)); !ok || r.SHA256 != skillDigest(gadak.SkillMarkdown()) {
		t.Errorf("install did not record a receipt for what it wrote: %+v ok=%v", r, ok)
	}
}

// TestLegacySkillDigestsAreFrozenAndWellFormed — GDK-92. The table is a
// backfill for pre-receipt installs, not a per-release chore. Two things must
// hold: the entries are real SHA-256 digests, and the *current* embed is not
// among them (it is the "identical" case, and an entry would make an unchanged
// skill report as an upgrade).
func TestLegacySkillDigestsAreFrozenAndWellFormed(t *testing.T) {
	if len(legacySkillDigests) == 0 {
		t.Fatal("no legacy digests: every pre-receipt install would demand --force")
	}
	for digest, provenance := range legacySkillDigests {
		if len(digest) != sha256.Size*2 {
			t.Errorf("%q is not a sha256 hex digest (%d chars)", digest, len(digest))
		}
		if _, err := hex.DecodeString(digest); err != nil {
			t.Errorf("%q is not hex: %v", digest, err)
		}
		if provenance == "" {
			t.Errorf("digest %s has no provenance note", digest)
		}
	}
	if _, found := legacySkillDigests[skillDigest(gadak.SkillMarkdown())]; found {
		t.Error("the current embedded skill is in the legacy table; an unchanged install would report as an update")
	}
}

// TestSkillFrontmatterName — GDK-92. Parsing feeds the refusal message only, so
// what matters is that it does not claim "gadak" for a file that never said so.
func TestSkillFrontmatterName(t *testing.T) {
	cases := map[string]string{
		"---\nname: gadak\n---\nbody\n":                 "gadak",
		"---\ndescription: x\nname:  gadak  \n---\nb\n": "gadak",
		"---\nname: my-notes\n---\nbody\n":              "my-notes",
		"no frontmatter at all\n":                       "",
		"---\nname: gadak\nunterminated frontmatter\n":  "",
		"body first\n---\nname: gadak\n---\n":           "",
		"---\ndescription: mentions name: gadak\n---\n": "",
	}
	for in, want := range cases {
		if got := skillFrontmatterName([]byte(in)); got != want {
			t.Errorf("skillFrontmatterName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSkillInstallRefusesUserAuthoredWithGadakFrontmatter — GDK-92. The refusal
// is the feature. `name: gadak` is not proof gadak wrote the file: a user who
// edits gadak's own skill keeps that line, so frontmatter alone must not
// license an overwrite.
func TestSkillInstallRefusesUserAuthoredWithGadakFrontmatter(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "gadak", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	// Hand-written, never installed by gadak: no bookkeeping alongside it.
	mine := []byte("---\nname: gadak\ndescription: my own house rules for this mirror\n---\n\nAlways ask me before writing to Jira.\n")
	if err := os.WriteFile(dest, mine, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := installSkill(&buf, gadak.SkillMarkdown(), dest, false, false)
	if err == nil {
		t.Fatal("expected a refusal for a file gadak did not write")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("refusal should point at --force: %v", err)
	}
	raw, _ := os.ReadFile(dest)
	if !bytes.Equal(raw, mine) {
		t.Error("a user-authored skill was overwritten without --force")
	}
}

// TestSkillInstallRefusesEditsMadeAfterOurInstall — GDK-92. gadak installed it,
// then the user edited it. That is their file now; only --force may replace it.
func TestSkillInstallRefusesEditsMadeAfterOurInstall(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "gadak", "SKILL.md")

	prev := []byte("---\nname: gadak\ndescription: the previous release's skill\n---\n\n# older body\n")
	var buf bytes.Buffer
	if err := installSkill(&buf, prev, dest, false, false); err != nil {
		t.Fatalf("seed previous release: %v", err)
	}
	edited := append(append([]byte{}, prev...), []byte("\nMy own note: never transition GDK-1.\n")...)
	if err := os.WriteFile(dest, edited, 0o644); err != nil {
		t.Fatal(err)
	}

	buf.Reset()
	if err := installSkill(&buf, gadak.SkillMarkdown(), dest, false, false); err == nil {
		t.Fatalf("expected a refusal after a hand edit, got:\n%s", buf.String())
	}
	raw, _ := os.ReadFile(dest)
	if !bytes.Equal(raw, edited) {
		t.Error("hand-edited skill was overwritten without --force")
	}
}

func TestSkillInstallPrintNoWrite(t *testing.T) {
	root := t.TempDir()
	// dest under a path that does not exist yet
	dest := filepath.Join(root, "skills-preview", "gadak", "SKILL.md")
	content := gadak.SkillMarkdown()

	var buf bytes.Buffer
	if err := installSkill(&buf, content, dest, false, true); err != nil {
		t.Fatalf("print: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(dest)); !os.IsNotExist(err) {
		t.Errorf("--print created parent dir: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("--print created dest file")
	}
	out := buf.String()
	if !strings.Contains(out, "source:") || !strings.Contains(out, "dest:") || !strings.Contains(out, "status:") {
		t.Errorf("print plan missing fields:\n%s", out)
	}
	if !strings.Contains(out, "missing") {
		t.Errorf("print should say missing:\n%s", out)
	}
	if !strings.Contains(out, "embedded skills/gadak/SKILL.md") {
		t.Errorf("print should name embedded source:\n%s", out)
	}

	// force + print still no write
	buf.Reset()
	if err := installSkill(&buf, content, dest, true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("--print --force must not create")
	}
}

func TestSkillInstallUnsupportedClient(t *testing.T) {
	err := cmdSkillInstall([]string{"cursor"})
	if err == nil {
		t.Fatal("expected error for cursor")
	}
	s := err.Error()
	if !strings.Contains(s, "cursor") {
		t.Errorf("error should name client: %v", err)
	}
	if !strings.Contains(s, "mcp install") && !strings.Contains(s, "SKILL.md") {
		t.Errorf("error should guide to mcp or copy: %v", err)
	}

	err = cmdSkillInstall([]string{"codex", "--print"})
	if err == nil {
		t.Fatal("expected error for codex")
	}
	if !strings.Contains(err.Error(), "codex") {
		t.Errorf("error should name codex: %v", err)
	}
}

func TestSkillInstallProjectCwd(t *testing.T) {
	root := t.TempDir()
	// Isolate cwd so --project never touches the real tree.
	t.Chdir(root)

	dest, err := resolveSkillDest(true, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".claude", "skills", "gadak", "SKILL.md")
	if dest != want {
		t.Fatalf("project dest = %q, want %q", dest, want)
	}

	var buf bytes.Buffer
	if err := installSkill(&buf, gadak.SkillMarkdown(), dest, false, false); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Error("project install bytes differ from embed")
	}
}

func TestResolveSkillDestDir(t *testing.T) {
	root := t.TempDir()
	got, err := resolveSkillDest(false, root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "gadak", "SKILL.md")
	if got != want {
		t.Errorf("--dir dest = %q, want %q", got, want)
	}
	// --dir wins over --project
	got, err = resolveSkillDest(true, root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("--dir should override --project: got %q", got)
	}
}

func TestCmdSkillBareHelp(t *testing.T) {
	// Bare skill / help must not write files.
	if err := cmdSkill(nil); err != nil {
		t.Fatal(err)
	}
	if err := cmdSkill([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
}

func TestCmdSkillInstallPrintViaCLI(t *testing.T) {
	root := t.TempDir()
	// Capture stdout by routing through installSkill is enough for unit tests;
	// this path exercises flag parsing + resolve with --dir --print.
	// cmdSkillInstall writes to os.Stdout; redirect temporarily.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	err = cmdSkillInstall([]string{"--print", "--dir", root})
	_ = w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("cmdSkillInstall: %v", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	out := buf.String()
	if !strings.Contains(out, "source:") || !strings.Contains(out, "status:") {
		t.Errorf("unexpected --print output:\n%s", out)
	}
	// Nothing under root/gadak
	if _, err := os.Stat(filepath.Join(root, "gadak")); !os.IsNotExist(err) {
		t.Error("--print must not create gadak/ under --dir")
	}
}

func TestAutoInstallSkillSkippedWithoutClaudeDir(t *testing.T) {
	home := isolateHome(t)
	var buf bytes.Buffer
	if got := autoInstallSkill(&buf); got != "skipped" {
		t.Fatalf("status = %q, want skipped", got)
	}
	if buf.Len() != 0 {
		t.Fatalf("skipped must be silent, got %q", buf.String())
	}
	if _, err := os.Stat(skillDestUnder(home)); !os.IsNotExist(err) {
		t.Fatalf("must not create SKILL.md: %v", err)
	}
}

func TestAutoInstallSkillInstallsMissing(t *testing.T) {
	home := isolateHomeWithClaude(t)
	var buf bytes.Buffer
	if got := autoInstallSkill(&buf); got != "installed" {
		t.Fatalf("status = %q, want installed; buf=%q", got, buf.String())
	}
	got, err := os.ReadFile(skillDestUnder(home))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatalf("bytes differ from embed")
	}
}

func TestAutoInstallSkillIdenticalIsInstalled(t *testing.T) {
	home := isolateHomeWithClaude(t)
	dest := skillDestUnder(home)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, gadak.SkillMarkdown(), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if got := autoInstallSkill(&buf); got != "installed" {
		t.Fatalf("status = %q, want installed", got)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatal("identical path mutated the file")
	}
}

func TestAutoInstallSkillUpdatesStaleCopy(t *testing.T) {
	home := isolateHomeWithClaude(t)
	dest := skillDestUnder(home)
	prev := []byte("---\nname: gadak\ndescription: previous\n---\n\n# older\n")
	if err := installSkill(io.Discard, prev, dest, false, false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var buf bytes.Buffer
	if got := autoInstallSkill(&buf); got != "installed" {
		t.Fatalf("status = %q, want installed; buf=%q", got, buf.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatal("stale copy was not updated")
	}
}

func TestAutoInstallSkillConflictDoesNotOverwrite(t *testing.T) {
	home := isolateHomeWithClaude(t)
	dest := skillDestUnder(home)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("user-authored skill body\n")
	if err := os.WriteFile(dest, old, 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if got := autoInstallSkill(&buf); got != "skipped" {
		t.Fatalf("status = %q, want skipped; buf=%q", got, buf.String())
	}
	raw, _ := os.ReadFile(dest)
	if !bytes.Equal(raw, old) {
		t.Fatal("conflict overwrote the user file")
	}
	if !strings.Contains(buf.String(), "gadak skill install --force") {
		t.Fatalf("conflict hint missing --force command:\n%s", buf.String())
	}
}

func TestAutoInstallSkillFailedWhenDestIsDirectory(t *testing.T) {
	home := isolateHomeWithClaude(t)
	dest := skillDestUnder(home)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if got := autoInstallSkill(&buf); got != "failed" {
		t.Fatalf("status = %q, want failed; buf=%q", got, buf.String())
	}
	if !strings.Contains(buf.String(), "warning:") {
		t.Fatalf("failed must warn on the writer:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Daily auto-sync — GDK-996: the installed copy follows the binary.
//
// Of the layers that receive a tag, the agent contract was the only one with
// no refresh: a skill installed by v0.16 kept teaching v0.16 verbs against a
// v0.18 binary until the user re-ran `gadak skill install`. The hook lives on
// main()'s dispatch path, so these tests drive the real binary.
// ---------------------------------------------------------------------------

var (
	gadakTestBinOnce sync.Once
	gadakTestBin     string
	gadakTestBinErr  error
)

// buildGadakTestBin builds the CLI once per test run. The auto-sync hook is
// wired in main(), which no in-process call reaches — only the built binary
// proves the wiring. go's caches sit under the real home, so the build gets
// the home TestMain replaced (Go's exec keeps the last duplicate env entry).
func buildGadakTestBin(t *testing.T) string {
	t.Helper()
	gadakTestBinOnce.Do(func() {
		_, thisFile, _, _ := runtime.Caller(0)
		dir, err := os.MkdirTemp("", "gadak-skill-autosync-bin-*")
		if err != nil {
			gadakTestBinErr = err
			return
		}
		bin := filepath.Join(dir, "gadak")
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = filepath.Dir(thisFile)
		build.Env = append(os.Environ(),
			"HOME="+gadakTestOrigHome,
			"USERPROFILE="+gadakTestOrigHome,
		)
		if out, err := build.CombinedOutput(); err != nil {
			gadakTestBinErr = fmt.Errorf("go build: %w\n%s", err, out)
			return
		}
		gadakTestBin = bin
	})
	if gadakTestBinErr != nil {
		t.Fatalf("cannot build the CLI for the auto-sync tests: %v", gadakTestBinErr)
	}
	return gadakTestBin
}

func removeGadakTestBin() {
	if gadakTestBin != "" {
		_ = os.RemoveAll(filepath.Dir(gadakTestBin))
	}
}

// runGadakTestBin runs one subcommand the way a user would and returns
// stdout, stderr and the process error. Env is inherited, so the caller's
// t.Setenv HOME / GADAK_HOME reach the child.
func runGadakTestBin(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(buildGadakTestBin(t), args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}

// autoSyncSeedStaleCopy plants what an older gadak left behind: the previous
// release's body plus the receipt that proves gadak wrote it — exactly the
// on-disk state after `brew upgrade`, before this feature.
func autoSyncSeedStaleCopy(t *testing.T, dest string) {
	t.Helper()
	prev := []byte("---\nname: gadak\ndescription: the previous release's skill\n---\n\n# older body\n")
	if err := installSkill(io.Discard, prev, dest, false, false); err != nil {
		t.Fatalf("seed previous release copy: %v", err)
	}
}

// readAutoSyncStampForTest reads the once-a-day stamp as an outsider would
// (raw JSON off disk), so the assertions lean on the file contract, not on
// the implementation's helpers.
func readAutoSyncStampForTest(t *testing.T, gadakHome string) (lastCheck string, exists bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(gadakHome, "skill-autosync.json"))
	if err != nil {
		return "", false
	}
	var s struct {
		LastCheck string `json:"last_check"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("rate-limit stamp is not JSON: %v\n%s", err, raw)
	}
	return s.LastCheck, true
}

// stampNamesToday reports whether an RFC3339 stamp falls on today's UTC date.
func stampNamesToday(t *testing.T, stamp string) bool {
	t.Helper()
	at, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("stamp %q is not RFC3339: %v", stamp, err)
	}
	y1, m1, d1 := at.UTC().Date()
	y2, m2, d2 := time.Now().UTC().Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// TestSkillAutoSyncUpdatesStaleCopyOnAnyCommand — GDK-996. The success gate
// the issue names: a stale-but-ours copy plus ONE arbitrary subcommand must
// end with the embedded current content and an advanced rate-limit stamp.
func TestSkillAutoSyncUpdatesStaleCopyOnAnyCommand(t *testing.T) {
	home := isolateHomeWithClaude(t)
	gadakHome := t.TempDir()
	t.Setenv("GADAK_HOME", gadakHome)
	dest := skillDestUnder(home)
	autoSyncSeedStaleCopy(t, dest)

	stdout, stderr, err := runGadakTestBin(t, "version")
	if err != nil {
		t.Fatalf("gadak version: %v\nstderr:\n%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got == "" {
		t.Fatalf("version printed nothing; stderr:\n%s", stderr)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatalf("one arbitrary subcommand left the copy stale (%d bytes; the embed is %d)", len(got), len(gadak.SkillMarkdown()))
	}
	if !strings.Contains(stderr, "skill: updated") {
		t.Errorf("the update must say so on stderr, got:\n%s", stderr)
	}
	lastCheck, ok := readAutoSyncStampForTest(t, gadakHome)
	if !ok {
		t.Fatal("no rate-limit stamp after a sync: the next command would redo the work")
	}
	if !stampNamesToday(t, lastCheck) {
		t.Fatalf("rate-limit stamp did not advance: last_check = %q", lastCheck)
	}
	// The receipt must describe the new bytes, so the next classifier still
	// sees gadak's own copy rather than a foreign file.
	if r, rok := readSkillReceipt(filepath.Dir(dest)); !rok || r.SHA256 != skillDigest(gadak.SkillMarkdown()) {
		t.Errorf("auto-sync left no receipt for the new bytes: %+v ok=%v", r, rok)
	}
}

// TestSkillAutoSyncNeverTouchesForeignCopy — GDK-996. "Foreign" is the
// installer's `conflict`: a copy whose hash matches neither the receipt nor a
// shipped digest is the user's edit. It must survive any subcommand unchanged
// — and unlike init's auto-install, without even a stderr hint, because this
// hook runs on every command and a daily nag is noise doctor already answers.
func TestSkillAutoSyncNeverTouchesForeignCopy(t *testing.T) {
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	dest := skillDestUnder(home)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := []byte("---\nname: gadak\ndescription: my own house rules for this mirror\n---\n\nAlways ask me before writing to Jira.\n")
	if err := os.WriteFile(dest, mine, 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runGadakTestBin(t, "version")
	if err != nil {
		t.Fatalf("gadak version: %v\nstderr:\n%s", err, stderr)
	}
	raw, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(raw, mine) {
		t.Fatal("auto-sync overwrote a copy gadak did not write")
	}
	if strings.Contains(stderr, "skill:") {
		t.Errorf("a foreign copy must not produce stderr output:\n%s", stderr)
	}
}

// TestSkillAutoSyncSwallowsUnwritableSkillDir — GDK-996. The hook must never
// turn into a subcommand failure: with the skill directory read-only the
// command still exits 0 (the sync retries tomorrow, by the stamp).
func TestSkillAutoSyncSwallowsUnwritableSkillDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod read-only is not the Windows failure mode")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permission bits are not enforced")
	}
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	dest := skillDestUnder(home)
	autoSyncSeedStaleCopy(t, dest)
	if err := os.Chmod(filepath.Dir(dest), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(dest), 0o755) })

	stdout, stderr, err := runGadakTestBin(t, "version")
	if err != nil {
		t.Fatalf("a read-only skill dir must not fail the subcommand: %v\nstderr:\n%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got == "" {
		t.Fatalf("version printed nothing; stderr:\n%s", stderr)
	}
	got, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if want := []byte("---\nname: gadak\ndescription: the previous release's skill\n---\n\n# older body\n"); !bytes.Equal(got, want) {
		t.Fatal("unexpected content with a read-only dir: the write should not have landed")
	}
}

// TestSkillAutoSyncRateLimitIsOncePerDay — GDK-996. Proven content-wise, not
// by mtime: a stale copy planted *behind* today's stamp survives a second
// command, so the rate limit short-circuits before any skill I/O. A stamp
// from yesterday does not rate-limit today.
func TestSkillAutoSyncRateLimitIsOncePerDay(t *testing.T) {
	home := isolateHomeWithClaude(t)
	gadakHome := t.TempDir()
	t.Setenv("GADAK_HOME", gadakHome)
	dest := skillDestUnder(home)
	prev := []byte("---\nname: gadak\ndescription: the previous release's skill\n---\n\n# older body\n")
	autoSyncSeedStaleCopy(t, dest)

	var buf bytes.Buffer
	maybeAutoSyncSkill(&buf, "version")
	if !strings.Contains(buf.String(), "skill: updated") {
		t.Fatalf("first check of the day must sync; got:\n%s", buf.String())
	}

	// Stale again, same day: the stamp must hold the second check off.
	if err := installSkill(io.Discard, prev, dest, false, false); err != nil {
		t.Fatalf("re-plant stale copy: %v", err)
	}
	buf.Reset()
	maybeAutoSyncSkill(&buf, "version")
	if buf.Len() != 0 {
		t.Fatalf("second check the same day must be silent, got:\n%s", buf.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, prev) {
		t.Fatal("the rate limit did not hold: a second same-day check rewrote the copy")
	}

	// Yesterday's stamp does not rate-limit today: the check runs again.
	p, err := skillAutoSyncStampPath()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillAutoSyncStamp(p, time.Now().UTC().Add(-24*time.Hour))
	buf.Reset()
	maybeAutoSyncSkill(&buf, "version")
	if !strings.Contains(buf.String(), "skill: updated") {
		t.Fatalf("a yesterday stamp must let today's check run; got:\n%s", buf.String())
	}
	got, err = os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatal("the day-boundary check did not bring the copy current")
	}
}

// TestSkillAutoSyncSkippedForSkillAndMCP — GDK-996. `skill` owns the copy
// explicitly (its install verb is the writer); `mcp` is the stdio JSON-RPC
// server. For neither may the hook run — and it must not even record a
// check, so those commands never consume the day's refresh.
func TestSkillAutoSyncSkippedForSkillAndMCP(t *testing.T) {
	home := isolateHomeWithClaude(t)
	gadakHome := t.TempDir()
	t.Setenv("GADAK_HOME", gadakHome)
	dest := skillDestUnder(home)
	prev := []byte("---\nname: gadak\ndescription: the previous release's skill\n---\n\n# older body\n")

	for _, cmd := range []string{"skill", "mcp"} {
		autoSyncSeedStaleCopy(t, dest)
		var buf bytes.Buffer
		maybeAutoSyncSkill(&buf, cmd)
		if buf.Len() != 0 {
			t.Errorf("%s: excluded commands must be silent, got:\n%s", cmd, buf.String())
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, prev) {
			t.Errorf("%s: excluded command rewrote the copy", cmd)
		}
		if _, err := os.Stat(filepath.Join(gadakHome, skillAutoSyncStampName)); !os.IsNotExist(err) {
			t.Errorf("%s: excluded command wrote the rate-limit stamp: %v", cmd, err)
		}
	}
}

// TestSkillAutoSyncDoesNotInstallMissing — GDK-996. `missing` means the user
// has no skill installed; creating one uninvited is init / `skill install`'s
// job (autoInstallSkill, which gates on ~/.claude existing). The daily hook
// only ever refreshes what gadak already wrote.
func TestSkillAutoSyncDoesNotInstallMissing(t *testing.T) {
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	var buf bytes.Buffer
	maybeAutoSyncSkill(&buf, "version")
	if buf.Len() != 0 {
		t.Fatalf("missing must be silent, got:\n%s", buf.String())
	}
	if _, err := os.Stat(skillDestUnder(home)); !os.IsNotExist(err) {
		t.Fatalf("auto-sync created a skill uninvited: %v", err)
	}
}
