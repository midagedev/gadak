package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	scry "github.com/midagedev/scry"
)

func TestSkillInstallNewMatchesEmbedded(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "scry", "SKILL.md")
	content := scry.SkillMarkdown()
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
	dest := filepath.Join(root, "scry", "SKILL.md")
	content := scry.SkillMarkdown()
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
	dest := filepath.Join(root, "scry", "SKILL.md")
	content := scry.SkillMarkdown()
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

func TestSkillInstallPrintNoWrite(t *testing.T) {
	root := t.TempDir()
	// dest under a path that does not exist yet
	dest := filepath.Join(root, "skills-preview", "scry", "SKILL.md")
	content := scry.SkillMarkdown()

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
	if !strings.Contains(out, "embedded skills/scry/SKILL.md") {
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
	want := filepath.Join(root, ".claude", "skills", "scry", "SKILL.md")
	if dest != want {
		t.Fatalf("project dest = %q, want %q", dest, want)
	}

	var buf bytes.Buffer
	if err := installSkill(&buf, scry.SkillMarkdown(), dest, false, false); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, scry.SkillMarkdown()) {
		t.Error("project install bytes differ from embed")
	}
}

func TestResolveSkillDestDir(t *testing.T) {
	root := t.TempDir()
	got, err := resolveSkillDest(false, root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "scry", "SKILL.md")
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
	// Nothing under root/scry
	if _, err := os.Stat(filepath.Join(root, "scry")); !os.IsNotExist(err) {
		t.Error("--print must not create scry/ under --dir")
	}
}
