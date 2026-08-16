package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gadak "github.com/midagedev/gadak"
)

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
